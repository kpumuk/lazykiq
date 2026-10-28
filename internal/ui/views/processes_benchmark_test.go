package views

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type processFetchBenchmarkHook struct {
	commands  atomic.Int64
	workReads atomic.Int64
}

func (h *processFetchBenchmarkHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *processFetchBenchmarkHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.record(cmd)
		return next(ctx, cmd)
	}
}

func (h *processFetchBenchmarkHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			h.record(cmd)
		}
		return next(ctx, cmds)
	}
}

func (h *processFetchBenchmarkHook) record(cmd redis.Cmder) {
	h.commands.Add(1)
	if cmd.Name() == "hgetall" {
		h.workReads.Add(1)
	}
}

// Each benchmark owns a Redis server with TCP and persistence disabled.
func processBenchmarkRedis(b *testing.B) *sidekiq.Client {
	b.Helper()
	server, err := exec.LookPath("redis-server")
	if err != nil {
		b.Skip("redis-server is required for process fetch benchmarks")
	}
	dir, err := os.MkdirTemp("", "lq-process-")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			b.Errorf("remove Redis fixture: %v", err)
		}
	})
	socket := filepath.Join(dir, "r.sock")
	cmd := exec.CommandContext(b.Context(), server, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--loglevel", "warning")
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	client, err := sidekiq.NewClient("unix://" + socket)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = client.Close() })
	deadline := time.Now().Add(3 * time.Second)
	for client.Redis().Ping(b.Context()).Err() != nil {
		if time.Now().After(deadline) {
			b.Fatalf("Redis did not start: %s", log.String())
		}
		time.Sleep(time.Millisecond)
	}
	return client
}

// BenchmarkProcessesFetch measures process-row fetching across active-work sizes.
func BenchmarkProcessesFetch(b *testing.B) {
	for _, workload := range []struct {
		processes int
		jobs      int
		argBytes  int
	}{
		{1, 0, 0},
		{1, 100, 0},
		{1, 100, 1024},
		{1, 100, 65536},
		{10, 10, 65536},
		{1, 1000, 65536},
	} {
		b.Run(fmt.Sprintf("processes_%d_jobs_%d_arg_bytes_%d", workload.processes, workload.jobs, workload.argBytes), func(b *testing.B) {
			client := processBenchmarkRedis(b)
			ctx := b.Context()
			for p := range workload.processes {
				identity := fmt.Sprintf("audit-%04d:1:nonce", p)
				if err := client.Redis().SAdd(ctx, "processes", identity).Err(); err != nil {
					b.Fatal(err)
				}
				if err := client.Redis().HSet(ctx, identity, "info", `{"hostname":"audit","pid":1,"concurrency":100}`, "busy", workload.jobs).Err(); err != nil {
					b.Fatal(err)
				}
				for j := range workload.jobs {
					job := fmt.Sprintf(`{"jid":"job-%d","class":"WideJob","args":["%s"]}`, j, strings.Repeat("x", workload.argBytes))
					work, err := json.Marshal(map[string]any{"queue": "default", "payload": job, "run_at": 1780000000})
					if err != nil {
						b.Fatal(err)
					}
					if err := client.Redis().HSet(ctx, identity+":work", strconv.Itoa(j), string(work)).Err(); err != nil {
						b.Fatal(err)
					}
				}
			}
			view := NewProcessesList(client)
			defer view.CancelRequests()
			cmd := view.fetchDataCmd()
			hook := &processFetchBenchmarkHook{}
			client.AddHook(hook)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				msg := cmd()
				if result, ok := msg.(processesListDataMsg); !ok || len(result.processes) != workload.processes {
					b.Fatalf("fetch result: %T %+v", msg, msg)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(hook.commands.Load())/float64(b.N), "commands/op")
			b.ReportMetric(float64(hook.workReads.Load())/float64(b.N), "work-hgetall/op")
		})
	}
}
