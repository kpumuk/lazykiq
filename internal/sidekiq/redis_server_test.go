package sidekiq

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// benchmarkPrivateSortedRedis starts a private redis-server for tests that
// need commands miniredis lacks. It skips locally without redis-server and
// fails in CI, where the workflow installs it.
func benchmarkPrivateSortedRedis(tb testing.TB) *Client {
	tb.Helper()
	server, err := exec.LookPath("redis-server")
	if err != nil {
		if os.Getenv("CI") != "" {
			tb.Fatalf("redis-server is required in CI: %v", err)
		}
		tb.Skip("real redis-server unavailable")
	}
	dir, err := os.MkdirTemp("", "lqp-")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "r.sock")
	cmd := exec.CommandContext(tb.Context(), server, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--loglevel", "warning")
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		tb.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1, PoolSize: 4, MaxActiveConns: 4, PipelinePoolSize: -1, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, ContextTimeoutEnabled: true})
	tb.Cleanup(func() { _ = rdb.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	deadline := time.Now().Add(3 * time.Second)
	for rdb.Ping(context.Background()).Err() != nil {
		if time.Now().After(deadline) {
			tb.Fatalf("redis not ready: %s", log.String())
		}
		time.Sleep(time.Millisecond)
	}
	return &Client{redis: rdb, version: Version8, versionDetected: true}
}
