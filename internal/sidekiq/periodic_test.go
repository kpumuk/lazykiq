package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func seedPeriodic(t *testing.T, client *Client, id, class string) PeriodicJob {
	t.Helper()
	client.capabilities = Capabilities{periodicJobs: true}
	job := PeriodicJob{LID: id, Version: "v1", Class: class, Schedule: "*/5 * * * *", Options: `{"queue":"low","args":[9007199254740993],"retry":3}`}
	ctx := t.Context()
	pipe := client.redis.Pipeline()
	pipe.Set(ctx, "periodic-version", job.Version, 0)
	pipe.SAdd(ctx, "loops-"+job.Version, id)
	pipe.HSet(ctx, "loop-"+id, "class", class, "schedule", job.Schedule, "options", job.Options)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestPeriodicReadAndHistory(t *testing.T) {
	_, client := setupTestRedis(t)
	job := seedPeriodic(t, client, "b", "CleanupJob")
	seedPeriodic(t, client, "a", "AnalyticsJob")
	ctx := t.Context()
	client.periodicTimezone = "America/Toronto"
	client.redis.SAdd(ctx, "loops-v1", "expired")
	client.redis.HSet(ctx, "loop-b", "paused", "1", "tz_name", "Asia/Tokyo")
	for i := range 30 {
		client.redis.ZAdd(ctx, "loop-history-b", redis.Z{Score: float64(1000 + i), Member: strings.Repeat("x", i+1)})
	}
	jobs, err := client.GetPeriodicJobs(ctx)
	if err != nil || len(jobs) != 2 || jobs[0].Class != "AnalyticsJob" || jobs[1].LID != job.LID {
		t.Fatalf("list = %+v, %v", jobs, err)
	}
	got, history, err := client.GetPeriodicJob(ctx, job.LID)
	if err != nil || got == nil || !got.Paused || got.EffectiveTimezone() != "Asia/Tokyo" || got.Queue() != "low" {
		t.Fatalf("detail = %+v, %v", got, err)
	}
	if len(history) != 25 || history[0].EnqueuedAt.Unix() != 1029 || history[24].EnqueuedAt.Unix() != 1005 {
		t.Fatalf("history = %+v", history)
	}
	got, history, err = client.GetPeriodicJob(ctx, "expired")
	if err != nil || got != nil || history != nil {
		t.Fatalf("expired = %+v, %+v, %v", got, history, err)
	}
	client.redis.Del(ctx, "periodic-version")
	jobs, err = client.GetPeriodicJobs(ctx)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("empty = %+v, %v", jobs, err)
	}
}

type periodicSnapshotHook struct{ change func() }

func (periodicSnapshotHook) DialHook(next redis.DialHook) redis.DialHook          { return next }
func (periodicSnapshotHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (h periodicSnapshotHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, commands []redis.Cmder) error {
		for _, command := range commands {
			if args := command.Args(); command.Name() == "get" && len(args) == 2 && args[1] == "periodic-version" {
				h.change()
				break
			}
		}
		return next(ctx, commands)
	}
}

func TestPeriodicReadRetriesConfigurationChanges(t *testing.T) {
	for name, continuous := range map[string]bool{"one change": false, "continuous changes": true} {
		t.Run(name, func(t *testing.T) {
			mr, client := setupTestRedis(t)
			seedPeriodic(t, client, "old", "OldJob")
			_, _ = mr.SetAdd("loops-v2", "new")
			mr.HSet("loop-new", "class", "NewJob", "schedule", "* * * * *", "options", `{}`)
			calls := 0
			client.redis.AddHook(periodicSnapshotHook{change: func() {
				calls++
				if calls == 1 {
					_ = mr.Set("periodic-version", "v2")
				} else if continuous {
					_ = mr.Set("periodic-version", strings.Repeat("v", calls))
				}
			}})
			jobs, err := client.GetPeriodicJobs(t.Context())
			if continuous {
				if !errors.Is(err, ErrPeriodicChanged) || calls != 3 {
					t.Fatalf("unstable snapshot = %+v, %v, calls=%d", jobs, err, calls)
				}
			} else if err != nil || len(jobs) != 1 || jobs[0].Class != "NewJob" || calls != 2 {
				t.Fatalf("retry = %+v, %v, calls=%d", jobs, err, calls)
			}
		})
	}
}

func TestPeriodicFeatureGuards(t *testing.T) {
	for _, feature := range []string{"", "pro"} {
		t.Run(feature, func(t *testing.T) {
			_, client := setupTestRedis(t)
			job := seedPeriodic(t, client, "id", "Job")
			client.capabilities = Capabilities{queuePause: feature == "pro"}
			ctx := t.Context()
			_, listErr := client.GetPeriodicJobs(ctx)
			_, _, detailErr := client.GetPeriodicJob(ctx, job.LID)
			pauseErr := client.SetPeriodicJobPaused(ctx, job, true)
			_, enqueueErr := client.EnqueuePeriodicJob(ctx, job)
			for _, err := range []error{listErr, detailErr, pauseErr, enqueueErr} {
				if !errors.Is(err, ErrPeriodicUnsupported) {
					t.Fatalf("guard error = %v", err)
				}
			}
			if client.redis.HExists(ctx, "loop-id", "paused").Val() || client.redis.Exists(ctx, "queues").Val() != 0 {
				t.Fatal("disabled feature wrote state")
			}
		})
	}
}

func TestPeriodicPauseAndEnqueue(t *testing.T) {
	for name, version := range map[string]Version{"Sidekiq 7": Version7, "Sidekiq 8": Version8} {
		t.Run(name, func(t *testing.T) {
			_, client := setupTestRedis(t)
			job := seedPeriodic(t, client, "id", "CleanupJob")
			ctx := t.Context()
			client.redis.SAdd(ctx, "processes", "worker")
			workerVersion := "7.3.9"
			if version == Version8 {
				workerVersion = "8.1.0"
			}
			client.redis.HSet(ctx, "worker", "info", `{"version":"`+workerVersion+`"}`)
			client.version = version
			client.versionDetected = true
			if err := client.SetPeriodicJobPaused(ctx, job, true); err != nil {
				t.Fatal(err)
			}
			before := time.Now()
			jid, err := client.EnqueuePeriodicJob(ctx, job)
			if err != nil || len(jid) != 24 {
				t.Fatalf("enqueue = %s, %v", jid, err)
			}
			var payload map[string]any
			if err := safeParseJSON([]byte(client.redis.LIndex(ctx, "queue:low", 0).Val()), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["class"] != job.Class || payload["jid"] != jid || payload["queue"] != "low" || payload["args"].([]any)[0].(json.Number).String() != "9007199254740993" {
				t.Fatalf("payload = %+v", payload)
			}
			timestamp, _ := payload["enqueued_at"].(json.Number).Float64()
			if version == Version8 {
				timestamp /= 1000
			}
			if timestamp < float64(before.Unix()) || timestamp > float64(time.Now().Unix()+1) {
				t.Fatalf("timestamp = %f", timestamp)
			}
			if client.redis.ZCard(ctx, "loop-history-id").Val() != 0 || client.redis.HGet(ctx, "loop-id", "paused").Val() != "1" || !client.redis.SIsMember(ctx, "queues", "low").Val() {
				t.Fatal("manual enqueue changed history or pause state, or missed queue registration")
			}
			if err := client.SetPeriodicJobPaused(ctx, job, false); err != nil || client.redis.HExists(ctx, "loop-id", "paused").Val() {
				t.Fatalf("resume = %v", err)
			}
		})
	}
}

func TestPeriodicMutationsRejectChangedConfiguration(t *testing.T) {
	for name, change := range map[string]func(context.Context, *Client){
		"version": func(ctx context.Context, c *Client) { c.redis.Set(ctx, "periodic-version", "v2", 0) },
		"removed": func(ctx context.Context, c *Client) { c.redis.SRem(ctx, "loops-v1", "id") },
		"expired": func(ctx context.Context, c *Client) { c.redis.Del(ctx, "loop-id") },
		"options": func(ctx context.Context, c *Client) { c.redis.HSet(ctx, "loop-id", "options", `{}`) },
		"zone":    func(ctx context.Context, c *Client) { c.redis.HSet(ctx, "loop-id", "tz_name", "UTC") },
	} {
		t.Run(name, func(t *testing.T) {
			_, client := setupTestRedis(t)
			job := seedPeriodic(t, client, "id", "Job")
			ctx := t.Context()
			change(ctx, client)
			if err := client.SetPeriodicJobPaused(ctx, job, true); !errors.Is(err, ErrPeriodicChanged) {
				t.Fatalf("pause = %v", err)
			}
			if _, err := client.EnqueuePeriodicJob(ctx, job); !errors.Is(err, ErrPeriodicChanged) {
				t.Fatalf("enqueue = %v", err)
			}
			if client.redis.Exists(ctx, "queues", "queue:low").Val() != 0 {
				t.Fatal("stale mutation enqueued work")
			}
		})
	}
}

func TestPeriodicEnqueueInvalidDataDoesNotWrite(t *testing.T) {
	for name, options := range map[string]string{
		"null": `null`, "array": `[]`, "broken": `{`, "args": `{"args":42}`, "queue": `{"queue":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, client := setupTestRedis(t)
			job := seedPeriodic(t, client, "id", "Job")
			job.Options = options
			client.redis.HSet(t.Context(), "loop-id", "options", options)
			if _, err := client.EnqueuePeriodicJob(t.Context(), job); err == nil {
				t.Fatal("invalid options accepted")
			}
			if client.redis.Exists(t.Context(), "queues", "queue:low", "queue:default").Val() != 0 {
				t.Fatal("invalid options wrote state")
			}
		})
	}
	for _, key := range []string{"queue:low", "queues"} {
		t.Run(key, func(t *testing.T) {
			_, client := setupTestRedis(t)
			job := seedPeriodic(t, client, "id", "Job")
			client.redis.Set(t.Context(), key, "wrong type", 0)
			before := client.redis.Keys(t.Context(), "*").Val()
			if _, err := client.EnqueuePeriodicJob(t.Context(), job); err == nil {
				t.Fatal("wrong Redis type accepted")
			}
			if after := client.redis.Keys(t.Context(), "*").Val(); !reflect.DeepEqual(before, after) {
				t.Fatalf("partial writes: %v -> %v", before, after)
			}
		})
	}
}
