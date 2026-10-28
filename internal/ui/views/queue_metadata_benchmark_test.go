package views

import (
	"bytes"
	"context"
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
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
)

type queueBenchmarkHook struct {
	commands   atomic.Int64
	roundTrips atomic.Int64
}

func (h *queueBenchmarkHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *queueBenchmarkHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.commands.Add(1)
		h.roundTrips.Add(1)
		return next(ctx, cmd)
	}
}

func (h *queueBenchmarkHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.commands.Add(int64(len(cmds)))
		h.roundTrips.Add(1)
		return next(ctx, cmds)
	}
}

func (h *queueBenchmarkHook) reset() {
	h.commands.Store(0)
	h.roundTrips.Store(0)
}

func (h *queueBenchmarkHook) report(b *testing.B) {
	b.ReportMetric(float64(h.commands.Load())/float64(b.N), "commands/op")
	b.ReportMetric(float64(h.roundTrips.Load())/float64(b.N), "roundtrips/op")
}

// Each benchmark owns a Redis subprocess with TCP and persistence disabled.
func queueBenchmarkRedis(b *testing.B) *sidekiq.Client {
	b.Helper()
	server, err := exec.LookPath("redis-server")
	if err != nil {
		b.Skip("redis-server is required for queue benchmarks")
	}
	// Short paths keep Unix sockets within the platform's address length limit.
	dir, err := os.MkdirTemp("", "lazykiq-bench-")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "redis.sock")
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

func BenchmarkQueueMetadata(b *testing.B) {
	client := queueBenchmarkRedis(b)
	ctx := b.Context()
	names := make([]any, 1000)
	for i := range names {
		names[i] = fmt.Sprintf("queue-%04d", i)
	}
	if err := client.Redis().SAdd(ctx, "queues", names...).Err(); err != nil {
		b.Fatal(err)
	}
	hook := &queueBenchmarkHook{}
	client.AddHook(hook)

	b.Run("list", func(b *testing.B) {
		cmd := NewQueuesList(client).fetchDataCmd()
		hook.reset()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			data, ok := cmd().(queuesListDataMsg)
			if !ok || len(data.queues) != len(names) {
				b.Fatal("queue list fetch failed")
			}
		}
		b.StopTimer()
		hook.report(b)
	})
	b.Run("details", func(b *testing.B) {
		view := NewQueueDetails(client)
		hook.reset()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			result, err := view.fetchWindow(ctx, 0, 75, lazytable.CursorStart)
			if err != nil || len(result.Payload.(queueDetailsPayload).queues) != len(names) {
				b.Fatalf("queue details fetch failed: %v", err)
			}
		}
		b.StopTimer()
		hook.report(b)
	})
	b.Run("paging", func(b *testing.B) {
		view := NewQueueDetails(client)
		initial, err := view.fetchWindow(ctx, 0, 75, lazytable.CursorStart)
		if err != nil {
			b.Fatal(err)
		}
		view.Update(lazytable.DataMsg{RequestID: view.lazy.RequestID(), Result: initial})
		hook.reset()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			result, err := view.fetchWindow(ctx, 75, 75, lazytable.CursorStart)
			if err != nil || result.Total != 0 {
				b.Fatalf("adjacent queue page failed: %v", err)
			}
		}
		b.StopTimer()
		hook.report(b)
	})
}

func BenchmarkQueueLatency(b *testing.B) {
	for _, width := range []int{32, 1024, 1 << 20} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			client := queueBenchmarkRedis(b)
			payload := `{"jid":"job","class":"TestJob","args":["` + strings.Repeat("x", width) + `"],"enqueued_at":1780000000000}`
			if err := client.Redis().RPush(b.Context(), "queue:default", payload).Err(); err != nil {
				b.Fatal(err)
			}
			queue := client.NewQueue("default")
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				latency, err := queue.Latency(b.Context())
				if err != nil || latency <= 0 {
					b.Fatalf("queue latency failed: %v (%v)", latency, err)
				}
			}
		})
	}
}
