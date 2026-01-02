// Package sidekiq provides a Redis-backed Sidekiq client and data models.
package sidekiq

import (
	"fmt"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/logging"
)

func init() {
	// Disable all Redis logging globally using the built-in VoidLogger
	redis.SetLogger(&logging.VoidLogger{})
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
