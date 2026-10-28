package sidekiq

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Adapted from the retained PERF-016 audit harness. All data lives in a new
// disposable Redis process; no existing service is contacted.
func BenchmarkMetricsTopJobsFanout(b *testing.B) {
	for _, minutes := range []int{60, 480} {
		for _, filter := range []string{"", "auditworker", "missing"} {
			b.Run(fmt.Sprintf("classes=1000/minutes=%d/filter=%q", minutes, filter), func(b *testing.B) {
				client := metricsBenchmarkRedis(b)
				ctx := context.Background()
				values := make(map[string]any, 3000)
				for i := range 1000 {
					name := fmt.Sprintf("AuditWorker%04d", i)
					values[name+"|p"] = 100
					values[name+"|f"] = 1
					values[name+"|ms"] = 200
				}
				now := time.Now().UTC()
				// Include adjacent buckets so a minute rollover during the run
				// does not change the number of populated buckets requested.
				for minute := -1; minute <= minutes; minute++ {
					key := metricsRollupKeySidekiq8(now.Add(-time.Duration(minute)*time.Minute), MetricsGranularityMinutely)
					if err := client.redis.HSet(ctx, key, values).Err(); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					got, err := client.GetMetricsTopJobs(ctx, MetricsPeriod{Minutes: minutes}, filter)
					if err != nil {
						b.Fatal(err)
					}
					wantJobs := 1000
					if filter == "missing" {
						wantJobs = 0
					}
					if len(got.Jobs) != wantJobs {
						b.Fatalf("got %d classes, want %d", len(got.Jobs), wantJobs)
					}
					if wantJobs > 0 && got.Jobs["AuditWorker0000"].Processed != int64(minutes*100) {
						b.Fatal("incorrect processed total")
					}
				}
			})
		}
	}
}

func metricsBenchmarkRedis(b *testing.B) *Client {
	b.Helper()
	server, err := exec.LookPath("redis-server")
	if err != nil {
		b.Skip("redis-server unavailable")
	}
	// Keep the socket path short; b.TempDir includes the full benchmark name.
	dir, err := os.MkdirTemp("", "lq-metrics-")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			b.Errorf("remove Redis fixture: %v", err)
		}
	})
	socket := filepath.Join(dir, "r.sock")
	process := exec.CommandContext(b.Context(), server, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--loglevel", "warning")
	var log bytes.Buffer
	process.Stdout, process.Stderr = &log, &log
	if err := process.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })
	rdb := redis.NewClient(&redis.Options{
		Network: "unix", Addr: socket, MaxRetries: -1, PoolSize: 4,
		MaxActiveConns: 4, PipelinePoolSize: -1,
		ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second,
		ContextTimeoutEnabled: true,
	})
	b.Cleanup(func() { _ = rdb.Close() })
	deadline := time.Now().Add(3 * time.Second)
	for rdb.Ping(context.Background()).Err() != nil {
		if time.Now().After(deadline) {
			b.Fatalf("private Redis not ready: %s", log.String())
		}
		time.Sleep(time.Millisecond)
	}
	return &Client{redis: rdb}
}
