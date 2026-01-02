// Package sidekiq provides a Redis-backed Sidekiq client and data models.
package sidekiq

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/logging"
)

func init() {
	// Disable all Redis logging globally using the built-in VoidLogger
	redis.SetLogger(&logging.VoidLogger{})
}

// Stats holds Sidekiq statistics.
type Stats struct {
	Processed int64
	Failed    int64
	Busy      int64
	Enqueued  int64
	Retries   int64
	Scheduled int64
	Dead      int64
}

// Client is a Sidekiq API client.
type Client struct {
	redis           *redis.Client
	displayRedisURL string
}

// NewClient creates a new Sidekiq client configured from a Redis URL.
func NewClient(redisURL string) (*Client, error) {
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	// Disable connection pool logging by disabling retries entirely.
	opts.MaxRetries = -1               // Disable retries completely
	opts.DialTimeout = 2 * time.Second // Short timeout to fail fast
	opts.ReadTimeout = 2 * time.Second
	opts.WriteTimeout = 2 * time.Second
	opts.PoolSize = 1 // Minimal pool size

	rdb := redis.NewClient(opts)

	return &Client{
		redis:           rdb,
		displayRedisURL: sanitizeRedisURL(redisURL),
	}, nil
}

// DisplayRedisURL returns a sanitized URL safe for display.
func (c *Client) DisplayRedisURL() string {
	return c.displayRedisURL
}

func sanitizeRedisURL(redisURL string) string {
	if redisURL == "" {
		return ""
	}
	parsed, err := url.Parse(redisURL)
	if err != nil {
		return redisURL
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		if username == "" {
			parsed.User = nil
		} else {
			parsed.User = url.User(username)
		}
	}
	return parsed.String()
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.redis.Close()
}

// Redis returns the underlying Redis client for benchmarking and testing.
func (c *Client) Redis() *redis.Client {
	return c.redis
}

// GetStats fetches current Sidekiq statistics from Redis.
// Uses pipelining to minimize roundtrips, following Sidekiq's fetch_stats_fast!/fetch_stats_slow! pattern.
func (c *Client) GetStats(ctx context.Context) (Stats, error) {
	stats := Stats{}

	// Pipeline 1: Fast stats that don't require iteration
	pipe1Results, err := c.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Get(ctx, "stat:processed")
		pipe.Get(ctx, "stat:failed")
		pipe.ZCard(ctx, "retry")
		pipe.ZCard(ctx, "schedule")
		pipe.ZCard(ctx, "dead")
		pipe.SMembers(ctx, "processes")
		pipe.SMembers(ctx, "queues")
		return nil
	})
	if err != nil {
		return stats, err
	}

	// Parse fast stats
	if cmd, ok := pipe1Results[0].(*redis.StringCmd); ok {
		if val, err := cmd.Result(); err == nil {
			stats.Processed, _ = strconv.ParseInt(val, 10, 64)
		}
	}
	if cmd, ok := pipe1Results[1].(*redis.StringCmd); ok {
		if val, err := cmd.Result(); err == nil {
			stats.Failed, _ = strconv.ParseInt(val, 10, 64)
		}
	}
	if cmd, ok := pipe1Results[2].(*redis.IntCmd); ok {
		if val, err := cmd.Result(); err == nil {
			stats.Retries = val
		}
	}
	if cmd, ok := pipe1Results[3].(*redis.IntCmd); ok {
		if val, err := cmd.Result(); err == nil {
			stats.Scheduled = val
		}
	}
	if cmd, ok := pipe1Results[4].(*redis.IntCmd); ok {
		if val, err := cmd.Result(); err == nil {
			stats.Dead = val
		}
	}

	var processes []string
	if cmd, ok := pipe1Results[5].(*redis.StringSliceCmd); ok {
		if val, err := cmd.Result(); err == nil {
			processes = val
		}
	}

	var queues []string
	if cmd, ok := pipe1Results[6].(*redis.StringSliceCmd); ok {
		if val, err := cmd.Result(); err == nil {
			queues = val
		}
	}

	// Pipeline 2: Slow stats that require aggregation
	if len(processes) == 0 && len(queues) == 0 {
		return stats, nil
	}

	pipe2Results, err := c.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, processKey := range processes {
			pipe.HGet(ctx, processKey, "busy")
		}
		for _, queue := range queues {
			pipe.LLen(ctx, "queue:"+queue)
		}
		return nil
	})
	if err != nil {
		return stats, err
	}

	// Sum up busy counts
	var busy int64
	for i := range processes {
		if cmd, ok := pipe2Results[i].(*redis.StringCmd); ok {
			if val, err := cmd.Result(); err == nil {
				count, _ := strconv.ParseInt(val, 10, 64)
				busy += count
			}
		}
	}
	stats.Busy = busy

	// Sum up queue sizes
	var enqueued int64
	offset := len(processes)
	for i := range queues {
		if cmd, ok := pipe2Results[offset+i].(*redis.IntCmd); ok {
			if val, err := cmd.Result(); err == nil {
				enqueued += val
			}
		}
	}
	stats.Enqueued = enqueued

	return stats, nil
}
