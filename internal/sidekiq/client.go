// Package sidekiq provides a Redis-backed Sidekiq client and data models.
package sidekiq

import (
	"context"
	"errors"
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

// GetStats fetches current Sidekiq statistics from Redis.
func (c *Client) GetStats(ctx context.Context) (Stats, error) {
	stats := Stats{}

	// Get processed count
	processed, err := c.redis.Get(ctx, "stat:processed").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return stats, err
	}
	if err == nil {
		stats.Processed, _ = strconv.ParseInt(processed, 10, 64)
	}

	// Get failed count
	failed, err := c.redis.Get(ctx, "stat:failed").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return stats, err
	}
	if err == nil {
		stats.Failed, _ = strconv.ParseInt(failed, 10, 64)
	}

	// Get busy workers count by summing from all process hashes
	processes, err := c.redis.SMembers(ctx, "processes").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return stats, err
	}
	var busy int64
	for _, processKey := range processes {
		// Get the "busy" field directly from the process hash
		busyStr, err := c.redis.HGet(ctx, processKey, "busy").Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			continue
		}
		if err == nil {
			busyCount, _ := strconv.ParseInt(busyStr, 10, 64)
			busy += busyCount
		}
	}
	stats.Busy = busy

	// Get enqueued count by summing all queue sizes
	queues, err := c.redis.SMembers(ctx, "queues").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return stats, err
	}
	var enqueued int64
	for _, queue := range queues {
		size, err := c.redis.LLen(ctx, "queue:"+queue).Result()
		if err == nil {
			enqueued += size
		}
	}
	stats.Enqueued = enqueued

	// Get retries count
	retries, err := c.redis.ZCard(ctx, "retry").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return stats, err
	}
	stats.Retries = retries

	// Get scheduled count
	scheduled, err := c.redis.ZCard(ctx, "schedule").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return stats, err
	}
	stats.Scheduled = scheduled

	// Get dead count
	dead, err := c.redis.ZCard(ctx, "dead").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return stats, err
	}
	stats.Dead = dead

	return stats, nil
}
