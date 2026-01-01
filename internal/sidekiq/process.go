package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Process represents a Sidekiq worker process.
type Process struct {
	client       *Client
	Identity     string         // hostname:pid:nonce (e.g., "be4860dbdb68:14:96908d62200c")
	Hostname     string         // Parsed from identity (e.g., "be4860dbdb68")
	PID          int            // Parsed from identity (e.g., 14)
	Tag          string         // From info.tag (e.g., "myapp")
	Concurrency  int            // From info.concurrency
	Busy         int            // From busy field (converted to int)
	Beat         time.Time      // From beat field (heartbeat timestamp)
	Quiet        bool           // From quiet field
	Queues       []string       // From info.queues
	QueueWeights map[string]int // From info.weights or weighted queues
	RSS          int64          // From rss field in KB, convert to bytes (*1024)
	RTTUS        int64          // From rtt_us field (microseconds)
	StartedAt    time.Time      // From info.started_at (timestamp)
}

// Job represents an active Sidekiq job (currently running).
type Job struct {
	*JobRecord             // embedded job data from payload
	ProcessIdentity string // process identity running this job
	ThreadID        string // Base-36 encoded TID
	RunAt           time.Time
}

type workData struct {
	Queue   string  `json:"queue"`
	Payload string  `json:"payload"`
	RunAt   float64 `json:"run_at"`
}

// BusyData holds process and job information.
type BusyData struct {
	Processes []Process
	Jobs      []Job
}

type processInfo struct {
	Hostname    string          `json:"hostname"`
	StartedAt   float64         `json:"started_at"`
	PID         int             `json:"pid"`
	Tag         string          `json:"tag"`
	Concurrency int             `json:"concurrency"`
	Queues      []string        `json:"queues"`
	Weights     json.RawMessage `json:"weights"`
	Labels      []string        `json:"labels"`
	Identity    string          `json:"identity"`
	Version     string          `json:"version"`
	Embedded    bool            `json:"embedded"`
}

// NewProcess creates a new Process instance for the given identity.
func (c *Client) NewProcess(identity string) *Process {
	return &Process{
		client:   c,
		Identity: identity,
	}
}

// GetProcesses fetches all process identities from Redis, sorted alphabetically.
func (c *Client) GetProcesses(ctx context.Context) ([]*Process, error) {
	identities, err := c.redis.SMembers(ctx, "processes").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	sort.Strings(identities)

	processes := make([]*Process, len(identities))
	for i, identity := range identities {
		processes[i] = c.NewProcess(identity)
	}

	return processes, nil
}

// GetBusyData fetches detailed process and active job information from Redis.
func (c *Client) GetBusyData(ctx context.Context) (BusyData, error) {
	var data BusyData

	processes, err := c.GetProcesses(ctx)
	if err != nil {
		return data, err
	}

	data.Processes = make([]Process, 0, len(processes))
	for _, process := range processes {
		if err := process.Refresh(ctx); err != nil {
			continue
		}
		data.Processes = append(data.Processes, *process)

		jobs, err := process.GetJobs(ctx, "")
		if err != nil {
			continue
		}
		data.Jobs = append(data.Jobs, jobs...)
	}

	return data, nil
}

// Refresh updates process data from Redis.
func (p *Process) Refresh(ctx context.Context) error {
	if p.client == nil {
		return errors.New("process client is nil")
	}

	fields, err := p.client.redis.HMGet(ctx, p.Identity, "info", "busy", "beat", "quiet", "rss", "rtt_us").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	p.Hostname = ""
	p.PID = 0
	p.Tag = ""
	p.Concurrency = 0
	p.Queues = nil
	p.QueueWeights = nil
	p.StartedAt = time.Time{}

	p.Busy = 0
	p.Beat = time.Time{}
	p.Quiet = false
	p.RSS = 0
	p.RTTUS = 0

	parseProcessInfo(fieldAt(fields, 0), p)
	if p.Hostname == "" || p.PID == 0 {
		parts := strings.Split(p.Identity, ":")
		if len(parts) >= 2 {
			p.Hostname = parts[0]
			if pid, err := strconv.Atoi(parts[1]); err == nil {
				p.PID = pid
			}
		}
	}

	if busyCount, ok := parseOptionalInt64(fieldAt(fields, 1)); ok {
		p.Busy = int(busyCount)
	}

	if beat, ok := parseOptionalFloat64(fieldAt(fields, 2)); ok {
		p.Beat = timeFromSeconds(beat)
	}

	if quiet, ok := parseOptionalBool(fieldAt(fields, 3)); ok {
		p.Quiet = quiet
	}

	if rss, ok := parseOptionalInt64(fieldAt(fields, 4)); ok {
		p.RSS = rss * 1024
	}

	if rtt, ok := parseOptionalInt64(fieldAt(fields, 5)); ok {
		p.RTTUS = rtt
	}

	return nil
}

// GetJobs fetches active jobs for the process.
// If filter is non-empty, only jobs whose raw work payload contains the substring are returned.
func (p *Process) GetJobs(ctx context.Context, filter string) ([]Job, error) {
	if p.client == nil {
		return nil, errors.New("process client is nil")
	}

	work, err := p.client.redis.HGetAll(ctx, p.Identity+":work").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	jobs := make([]Job, 0, len(work))
	for tid, workJSON := range work {
		if filter != "" && !strings.Contains(workJSON, filter) {
			continue
		}

		var work workData
		if err := json.Unmarshal([]byte(workJSON), &work); err != nil {
			continue
		}

		job := Job{
			ProcessIdentity: p.Identity,
			ThreadID:        tid,
		}

		if work.RunAt > 0 {
			job.RunAt = timeFromSeconds(work.RunAt)
		}

		if work.Payload != "" {
			job.JobRecord = NewJobRecord(work.Payload, work.Queue)
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

func parseProcessInfo(field any, process *Process) {
	infoStr, ok := field.(string)
	if !ok || infoStr == "" {
		return
	}

	var info processInfo
	if err := json.Unmarshal([]byte(infoStr), &info); err != nil {
		return
	}

	process.Hostname = info.Hostname
	process.PID = info.PID
	if process.Identity == "" && info.Identity != "" {
		process.Identity = info.Identity
	}

	process.Concurrency = info.Concurrency
	process.Queues, process.QueueWeights = parseProcessQueues(info.Queues, info.Weights)
	process.Tag = info.Tag
	process.StartedAt = timeFromSeconds(info.StartedAt)
}

func parseProcessQueues(queues []string, weights json.RawMessage) ([]string, map[string]int) {
	parsedQueues, queueWeights := parseQueuesField(queues)
	weightMap := parseQueueWeightsRaw(weights)
	if len(queueWeights) == 0 && len(weightMap) == 0 {
		return parsedQueues, nil
	}

	merged := make(map[string]int)
	if len(queueWeights) > 0 {
		maps.Copy(merged, queueWeights)
	}
	if len(weightMap) > 0 {
		maps.Copy(merged, weightMap)
	}
	return parsedQueues, merged
}

func parseQueuesField(queues []string) ([]string, map[string]int) {
	if len(queues) == 0 {
		return nil, nil
	}

	result := make([]string, 0, len(queues))
	var weights map[string]int
	for _, q := range queues {
		name, weight, hasWeight := parseWeightedQueue(q)
		result = append(result, name)
		if hasWeight {
			if weights == nil {
				weights = make(map[string]int)
			}
			weights[name] = weight
		}
	}
	return result, weights
}

func parseWeightedQueue(queue string) (string, int, bool) {
	parts := strings.SplitN(queue, ",", 2)
	if len(parts) != 2 {
		return queue, 0, false
	}
	weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return queue, 0, false
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return queue, 0, false
	}
	return name, weight, true
}

func parseQueueWeights(weights []map[string]int) map[string]int {
	if len(weights) == 0 {
		return nil
	}
	merged := make(map[string]int)
	for _, entry := range weights {
		maps.Copy(merged, entry)
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func parseQueueWeightsRaw(raw json.RawMessage) map[string]int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var weights []map[string]int
	if err := json.Unmarshal(raw, &weights); err == nil && len(weights) > 0 {
		return parseQueueWeights(weights)
	}

	// Sidekiq 7.0.0 legacy format stored weights as a single hash.
	var legacy map[string]int
	if err := json.Unmarshal(raw, &legacy); err == nil && len(legacy) > 0 {
		return parseQueueWeightsLegacy(legacy)
	}

	return nil
}

// parseQueueWeightsLegacy handles Sidekiq 7.0.0 weights format (single hash).
func parseQueueWeightsLegacy(weights map[string]int) map[string]int {
	if len(weights) == 0 {
		return nil
	}
	legacy := make(map[string]int, len(weights))
	maps.Copy(legacy, weights)
	return legacy
}

func timeFromSeconds(seconds float64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(seconds*float64(time.Second)))
}

func fieldAt(fields []any, idx int) any {
	if idx >= 0 && idx < len(fields) {
		return fields[idx]
	}
	return nil
}

func parseOptionalInt64(field any) (int64, bool) {
	switch value := field.(type) {
	case string:
		if value == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case float64:
		return int64(value), true
	case int64:
		return value, true
	}
	return 0, false
}

func parseOptionalFloat64(field any) (float64, bool) {
	switch value := field.(type) {
	case string:
		if value == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case float64:
		return value, true
	case int64:
		return float64(value), true
	}
	return 0, false
}

func parseOptionalBool(field any) (bool, bool) {
	switch value := field.(type) {
	case string:
		if value == "" {
			return false, false
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, false
		}
		return parsed, true
	case bool:
		return value, true
	}
	return false, false
}
