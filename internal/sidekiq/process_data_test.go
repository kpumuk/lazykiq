package sidekiq

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type processDataHook struct {
	commands []string
	after    func(string)
}

func (h *processDataHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *processDataHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.commands = append(h.commands, cmd.Name())
		err := next(ctx, cmd)
		if h.after != nil {
			h.after(cmd.Name())
		}
		return err
	}
}

func (h *processDataHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			h.commands = append(h.commands, cmd.Name())
		}
		err := next(ctx, cmds)
		if h.after != nil {
			h.after(cmds[0].Name())
		}
		return err
	}
}

func TestGetProcessesDataWithoutWork(t *testing.T) {
	for name, tc := range map[string]struct {
		jobs      int
		width     int
		wrongType bool
	}{
		"no work":         {},
		"ordinary work":   {jobs: 100, width: 32},
		"wide work":       {jobs: 100, width: 65536},
		"unreadable work": {wrongType: true},
	} {
		t.Run(name, func(t *testing.T) {
			mr, client := setupTestRedis(t)
			for _, identity := range []string{"z:4:nonce", "c:3:nonce", "b:2:nonce", "a:1:nonce", "gone:5:nonce", "invalid"} {
				_, _ = mr.SetAdd("processes", identity)
			}
			info := `{"hostname":"worker","pid":42,"tag":"app","version":"8.1.0","concurrency":8,"started_at":1700000000.5,"capsules":{"default":{"concurrency":8,"mode":"weighted","weights":{"critical":5,"default":1}}}}`
			for _, identity := range []string{"z:4:nonce", "c:3:nonce", "b:2:nonce", "a:1:nonce"} {
				mr.HSet(identity, "info", info, "busy", "3", "beat", "1780000000.25", "rss", "2048", "rtt_us", "123")
			}
			mr.HSet("invalid", "info", `{}`)
			mr.HSet("b:2:nonce", "quiet", "true")
			_, _ = mr.Lpush("a:1:nonce-signals", "TSTP")
			_, _ = mr.Lpush("b:2:nonce-signals", "TSTP")
			_, _ = mr.Lpush("c:3:nonce-signals", "TSTP")
			_, _ = mr.Lpush("c:3:nonce-signals", "TERM")
			mr.HSet("c:3:nonce", "quiet", "true")
			for i := range tc.jobs {
				payload := fmt.Sprintf(`{"jid":"job-%d","class":"WideJob","args":["%s"]}`, i, strings.Repeat("x", tc.width))
				work := mustMarshalJSON(t, workData{Queue: "critical", Payload: payload, RunAt: 1780000000.25})
				mr.HSet("a:1:nonce:work", strconv.Itoa(i), string(work))
			}
			if tc.wrongType {
				_ = mr.Set("a:1:nonce:work", "not a hash")
			}
			if _, err := client.redis.Pipelined(t.Context(), func(pipe redis.Pipeliner) error {
				pipe.Ping(t.Context())
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			hook := &processDataHook{}
			client.AddHook(hook)
			processes, err := client.GetProcessesData(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			// Redis may initialize separate connections for commands and pipelines.
			commands := slices.DeleteFunc(slices.Clone(hook.commands), func(name string) bool {
				return name == "hello" || name == "client"
			})
			if slices.Contains(commands, "hgetall") || len(commands) != 13 {
				t.Fatalf("metadata fetch commands: %v", hook.commands)
			}
			if len(processes) != 4 {
				t.Fatalf("processes = %+v", processes)
			}
			wantStatus := []string{ProcessStatusPausing, ProcessStatusQuiet, ProcessStatusStopping, ProcessStatusRunning}
			wantIDs := []string{"a:1:nonce", "b:2:nonce", "c:3:nonce", "z:4:nonce"}
			for i, p := range processes {
				if p.Identity != wantIDs[i] || p.Status != wantStatus[i] || p.Hostname != "worker" || p.PID != 42 || p.Tag != "app" || p.Version != "8.1.0" || p.Concurrency != 8 || p.Busy != 3 || p.RSS != 2097152 || p.RTTUS != 123 {
					t.Fatalf("process metadata: %+v", p)
				}
				if p.StartedAt.Sub(time.Unix(1700000000, 500000000)).Abs() > time.Microsecond || p.Beat.Sub(time.Unix(1780000000, 250000000)).Abs() > time.Microsecond {
					t.Fatalf("process timestamps: %+v", p)
				}
				want := Capsule{Concurrency: 8, Mode: "weighted", Weights: map[string]int{"critical": 5, "default": 1}}
				if !reflect.DeepEqual(p.Capsules, map[string]Capsule{"default": want}) {
					t.Fatalf("process capsules: %+v", p.Capsules)
				}
			}
			if tc.wrongType {
				return
			}
			hook.commands = nil
			busy, err := client.GetBusyData(t.Context(), "")
			if err != nil || !reflect.DeepEqual(processes, busy.Processes) || len(busy.Jobs) != tc.jobs {
				t.Fatalf("Busy parity: processes=%+v jobs=%d error=%v", busy.Processes, len(busy.Jobs), err)
			}
			if !slices.Contains(hook.commands, "hgetall") {
				t.Fatal("Busy did not retrieve work")
			}
			wantArgs := []any{strings.Repeat("x", tc.width)}
			for _, job := range busy.Jobs {
				if job.JID() != "job-"+job.ThreadID || job.ProcessIdentity != "a:1:nonce" || job.Queue() != "critical" || !reflect.DeepEqual(job.Args(), wantArgs) || job.RunAt.Sub(time.Unix(1780000000, 250000000)).Abs() > time.Microsecond {
					t.Fatalf("Busy job changed: %+v", job)
				}
			}
		})
	}
}

func TestGetProcessesDataEmpty(t *testing.T) {
	_, client := setupTestRedis(t)
	processes, err := client.GetProcessesData(t.Context())
	if err != nil || len(processes) != 0 {
		t.Fatalf("empty processes: %+v, %v", processes, err)
	}
}

func TestGetProcessesDataCancellation(t *testing.T) {
	for _, phase := range []string{"before", "smembers", "hmget", "lrange"} {
		t.Run(phase, func(t *testing.T) {
			mr, client := setupTestRedis(t)
			_, _ = mr.SetAdd("processes", "worker:1:nonce")
			mr.HSet("worker:1:nonce", "info", `{"hostname":"worker","pid":1}`)
			if err := client.redis.Ping(t.Context()).Err(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			hook := &processDataHook{after: func(command string) {
				if command == phase {
					cancel()
				}
			}}
			client.AddHook(hook)
			if phase == "before" {
				cancel()
			}
			processes, err := client.GetProcessesData(ctx)
			if !errors.Is(err, context.Canceled) || processes != nil {
				t.Fatalf("canceled processes: %+v, %v", processes, err)
			}
		})
	}
}

func TestGetProcessesDataErrors(t *testing.T) {
	for _, phase := range []string{"processes", "worker:1:nonce", "worker:1:nonce-signals", "connection"} {
		t.Run(phase, func(t *testing.T) {
			mr, client := setupTestRedis(t)
			_, _ = mr.SetAdd("processes", "worker:1:nonce")
			mr.HSet("worker:1:nonce", "info", `{"hostname":"worker","pid":1}`)
			if phase == "connection" {
				_ = client.Close()
			} else {
				mr.Del(phase)
				if err := mr.Set(phase, "wrong type"); err != nil {
					t.Fatal(err)
				}
			}
			processes, err := client.GetProcessesData(t.Context())
			if err == nil || processes != nil {
				t.Fatalf("failed metadata returned a snapshot: %+v, %v", processes, err)
			}
		})
	}
}
