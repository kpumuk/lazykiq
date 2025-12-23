package sidekiq

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/logging"
)

func init() {
	// Disable all Redis logging globally using the built-in VoidLogger
	redis.SetLogger(&logging.VoidLogger{})
}

// Stats holds Sidekiq statistics
type Stats struct {
	Processed int64
	Failed    int64
	Busy      int64
	Enqueued  int64
	Retries   int64
	Scheduled int64
	Dead      int64
}

// Client is a Sidekiq API client
type Client struct {
	redis *redis.Client
}

// NewClient creates a new Sidekiq client with hardcoded Redis connection
func NewClient() *Client {
	// Disable connection pool logging by setting MaxRetries to 0
	// This prevents the pool from retrying and logging errors
	rdb := redis.NewClient(&redis.Options{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		MaxRetries:   -1,              // Disable retries completely
		DialTimeout:  2 * time.Second, // Short timeout to fail fast
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     1,               // Minimal pool size
	})

	return &Client{
		redis: rdb,
	}
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.redis.Close()
}

// GetStats fetches current Sidekiq statistics from Redis
func (c *Client) GetStats(ctx context.Context) (Stats, error) {
	stats := Stats{}

	// Get processed count
	processed, err := c.redis.Get(ctx, "stat:processed").Result()
	if err != nil && err != redis.Nil {
		return stats, err
	}
	if err == nil {
		stats.Processed, _ = strconv.ParseInt(processed, 10, 64)
	}

	// Get failed count
	failed, err := c.redis.Get(ctx, "stat:failed").Result()
	if err != nil && err != redis.Nil {
		return stats, err
	}
	if err == nil {
		stats.Failed, _ = strconv.ParseInt(failed, 10, 64)
	}

	// Get busy workers count by summing from all process hashes
	processes, err := c.redis.SMembers(ctx, "processes").Result()
	if err != nil && err != redis.Nil {
		return stats, err
	}
	var busy int64
	for _, processKey := range processes {
		// Get the "busy" field directly from the process hash
		busyStr, err := c.redis.HGet(ctx, processKey, "busy").Result()
		if err != nil && err != redis.Nil {
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
	if err != nil && err != redis.Nil {
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
	if err != nil && err != redis.Nil {
		return stats, err
	}
	stats.Retries = retries

	// Get scheduled count
	scheduled, err := c.redis.ZCard(ctx, "schedule").Result()
	if err != nil && err != redis.Nil {
		return stats, err
	}
	stats.Scheduled = scheduled

	// Get dead count
	dead, err := c.redis.ZCard(ctx, "dead").Result()
	if err != nil && err != redis.Nil {
		return stats, err
	}
	stats.Dead = dead

	return stats, nil
}
