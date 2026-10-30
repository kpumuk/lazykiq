package sidekiq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrPeriodicUnsupported means Enterprise periodic jobs were not enabled.
var ErrPeriodicUnsupported = errors.New("periodic jobs require --enable=ent")

// ErrPeriodicChanged means the selected definition is no longer current.
var ErrPeriodicChanged = errors.New("periodic job configuration changed; refresh and try again")

// PeriodicJob is an Enterprise recurring-job definition, separate from scheduled jobs.
type PeriodicJob struct {
	LID             string
	Version         string
	Class           string
	Schedule        string
	Timezone        string
	DefaultTimezone string
	Options         string
	Paused          bool
}

// PeriodicExecution records an enqueue event, not the outcome of job execution.
type PeriodicExecution struct {
	JID        string
	EnqueuedAt time.Time
}

// WithPeriodicTimezone sets the server timezone for definitions without tz_name.
func WithPeriodicTimezone(name string) ClientOption {
	return func(c *Client) { c.periodicTimezone = name }
}

// GetPeriodicJobs reads a consistent snapshot of the current configuration.
func (c *Client) GetPeriodicJobs(ctx context.Context) ([]PeriodicJob, error) {
	if !c.capabilities.SupportsPeriodicJobs() {
		return nil, ErrPeriodicUnsupported
	}
	for range 3 {
		version, err := c.redis.Get(ctx, "periodic-version").Result()
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		ids, err := c.redis.SMembers(ctx, "loops-"+version).Result()
		if err != nil {
			return nil, err
		}
		pipe := c.redis.Pipeline()
		commands := make([]*redis.MapStringStringCmd, len(ids))
		for i, id := range ids {
			commands[i] = pipe.HGetAll(ctx, "loop-"+id)
		}
		currentVersion := pipe.Get(ctx, "periodic-version")
		_, err = pipe.Exec(ctx)
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		if currentVersion.Val() != version {
			continue
		}
		jobs := make([]PeriodicJob, 0, len(ids))
		for i, command := range commands {
			fields := command.Val()
			if len(fields) == 0 {
				continue
			}
			jobs = append(jobs, PeriodicJob{
				LID: ids[i], Version: version, Class: fields["class"], Schedule: fields["schedule"],
				Timezone: fields["tz_name"], DefaultTimezone: c.periodicTimezone,
				Options: fields["options"], Paused: fields["paused"] == "1",
			})
		}
		slices.SortFunc(jobs, func(a, b PeriodicJob) int {
			if cmp := strings.Compare(a.Class, b.Class); cmp != 0 {
				return cmp
			}
			if cmp := strings.Compare(a.Schedule, b.Schedule); cmp != 0 {
				return cmp
			}
			return strings.Compare(a.LID, b.LID)
		})
		return jobs, nil
	}
	return nil, ErrPeriodicChanged
}

// GetPeriodicJob returns a current definition and its latest 25 enqueue events.
// A removed or expired definition is returned as nil.
func (c *Client) GetPeriodicJob(ctx context.Context, lid string) (*PeriodicJob, []PeriodicExecution, error) {
	jobs, err := c.GetPeriodicJobs(ctx)
	if err != nil {
		return nil, nil, err
	}
	index := slices.IndexFunc(jobs, func(job PeriodicJob) bool { return job.LID == lid })
	if index < 0 {
		return nil, nil, nil
	}
	job := jobs[index]
	scores, err := c.redis.ZRevRangeWithScores(ctx, "loop-history-"+lid, 0, 24).Result()
	if err != nil {
		return nil, nil, err
	}
	history := make([]PeriodicExecution, len(scores))
	for i, score := range scores {
		history[i] = PeriodicExecution{JID: fmt.Sprint(score.Member), EnqueuedAt: time.Unix(int64(score.Score), 0)}
	}
	return &job, history, nil
}

// Queue returns the registered queue, or Sidekiq's default when unspecified.
func (p PeriodicJob) Queue() string {
	var options struct{ Queue string }
	if json.Unmarshal([]byte(p.Options), &options) == nil && options.Queue != "" {
		return options.Queue
	}
	return "default"
}

// EffectiveTimezone returns the named zone or the configured server default.
func (p PeriodicJob) EffectiveTimezone() string {
	if p.Timezone != "" {
		return p.Timezone
	}
	if p.DefaultTimezone != "" {
		return p.DefaultTimezone
	}
	return "UTC"
}

const periodicMatchScript = `
if redis.call("GET", KEYS[1]) ~= ARGV[1] then return 0 end
if redis.call("SISMEMBER", KEYS[2], ARGV[2]) == 0 then return 0 end
if redis.call("HGET", KEYS[3], "class") ~= ARGV[3] or
   redis.call("HGET", KEYS[3], "schedule") ~= ARGV[4] or
   redis.call("HGET", KEYS[3], "options") ~= ARGV[5] or
   (redis.call("HGET", KEYS[3], "tz_name") or "") ~= ARGV[6] then return 0 end
`

var periodicPauseScript = redis.NewScript(periodicMatchScript + `
if ARGV[7] == "1" then
  redis.call("HSET", KEYS[3], "paused", "1")
else
  redis.call("HDEL", KEYS[3], "paused")
end
return 1
`)

// SetPeriodicJobPaused changes future recurring enqueues without affecting queued work.
func (c *Client) SetPeriodicJobPaused(ctx context.Context, job PeriodicJob, paused bool) error {
	if !c.capabilities.SupportsPeriodicJobs() {
		return ErrPeriodicUnsupported
	}
	value := "0"
	if paused {
		value = "1"
	}
	args := append(periodicMatchArgs(job), value)
	changed, err := periodicPauseScript.Run(ctx, c.redis, periodicKeys(job), args...).Int()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrPeriodicChanged
	}
	return nil
}

// Manual enqueues match Enterprise's enqueue action: they do not append history.
var periodicEnqueueScript = redis.NewScript(periodicMatchScript + `
local qtype = redis.call("TYPE", KEYS[4]).ok
local stype = redis.call("TYPE", KEYS[5]).ok
if qtype ~= "none" and qtype ~= "list" then return redis.error_reply("queue is not a list") end
if stype ~= "none" and stype ~= "set" then return redis.error_reply("queues is not a set") end
redis.call("SADD", KEYS[5], ARGV[7])
redis.call("LPUSH", KEYS[4], ARGV[8])
return 1
`)

// EnqueuePeriodicJob immediately submits one instance, including while paused.
func (c *Client) EnqueuePeriodicJob(ctx context.Context, job PeriodicJob) (string, error) {
	if !c.capabilities.SupportsPeriodicJobs() {
		return "", ErrPeriodicUnsupported
	}
	var payload map[string]any
	if err := safeParseJSON([]byte(job.Options), &payload); err != nil || payload == nil {
		return "", errors.New("periodic job options must be a JSON object")
	}
	if strings.TrimSpace(job.Class) == "" {
		return "", errors.New("periodic job class is empty")
	}
	if payload["args"] == nil {
		payload["args"] = []any{}
	}
	if _, ok := payload["args"].([]any); !ok {
		return "", errors.New("periodic job args must be an array")
	}
	queue := "default"
	if raw, exists := payload["queue"]; exists {
		name, ok := raw.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return "", errors.New("periodic job queue must be a nonempty string")
		}
		queue = name
	}
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	jid := hex.EncodeToString(id[:])
	payload["class"], payload["queue"], payload["jid"] = job.Class, queue, jid
	if _, ok := payload["retry"]; !ok {
		payload["retry"] = true
	}
	timestamp := nowTimestamp(detectTimestampFormat(nil, c.DetectVersion(ctx)))
	payload["created_at"], payload["enqueued_at"] = timestamp, timestamp
	delete(payload, "at")
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	keys := append(periodicKeys(job), "queue:"+queue, "queues")
	args := append(periodicMatchArgs(job), queue, string(encoded))
	changed, err := periodicEnqueueScript.Run(ctx, c.redis, keys, args...).Int()
	if err != nil {
		return "", err
	}
	if changed == 0 {
		return "", ErrPeriodicChanged
	}
	return jid, nil
}

func periodicKeys(job PeriodicJob) []string {
	return []string{"periodic-version", "loops-" + job.Version, "loop-" + job.LID}
}

func periodicMatchArgs(job PeriodicJob) []any {
	return []any{job.Version, job.LID, job.Class, job.Schedule, job.Options, job.Timezone}
}
