// Package sidekiq provides a Redis-backed Sidekiq client and data models.
package sidekiq

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/logging"
)

// Version represents detected Sidekiq version for key format selection.
type Version int

const (
	// VersionUnknown means version could not be detected.
	VersionUnknown Version = iota
	// Version7 uses j|YYYYMMDD|H:M format (8-digit date).
	Version7
	// Version8 uses j|YYMMDD|H:M format (6-digit date).
	Version8

	uiRedisPoolSize = 4
)

// DisableRedisLogging silences go-redis process-level logging for the TUI.
func DisableRedisLogging() {
	redis.SetLogger(&logging.VoidLogger{})
}

// Client is a Sidekiq API client.
type Client struct {
	redis           *redis.Client
	displayRedisURL string
	versionMu       sync.Mutex
	version         Version
	versionDetected bool
	versionPending  chan struct{}
	capabilities    Capabilities
}

// ClientOption configures a client at construction.
type ClientOption func(*Client)

// WithFeatures configures the optional capabilities selected by ResolveFeatures.
func WithFeatures(features Capabilities) ClientOption {
	return func(c *Client) { c.capabilities = features }
}

// NewClient creates a new Sidekiq client configured from a Redis URL.
func NewClient(redisURL string, options ...ClientOption) (*Client, error) {
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
	opts.ContextTimeoutEnabled = true
	opts.PoolSize = uiRedisPoolSize
	opts.MaxActiveConns = uiRedisPoolSize
	// Keep pipelines within the shared connection limit.
	opts.PipelinePoolSize = -1

	rdb := redis.NewClient(opts)

	client := &Client{
		redis:           rdb,
		displayRedisURL: sanitizeRedisURL(redisURL),
	}
	for _, option := range options {
		option(client)
	}
	return client, nil
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

// Do executes a raw Redis command.
func (c *Client) Do(ctx context.Context, args ...any) (any, error) {
	if c == nil || c.redis == nil {
		return nil, errors.New("redis client is nil")
	}
	if len(args) == 0 {
		return nil, errors.New("no command provided")
	}
	return c.redis.Do(ctx, args...).Result()
}

// AddHook attaches a Redis hook to the underlying client.
func (c *Client) AddHook(h redis.Hook) {
	if c == nil || c.redis == nil || h == nil {
		return
	}
	c.redis.AddHook(h)
}

// DetectVersion detects which Sidekiq version is being used based on key format.
// Uses SCAN to efficiently find any existing metrics key.
// Successful observations, including VersionUnknown, are cached for the client
// lifetime. Failed scans return VersionUnknown without caching, allowing retries.
func (c *Client) DetectVersion(ctx context.Context) Version {
	for {
		c.versionMu.Lock()
		if c.versionDetected {
			version := c.version
			c.versionMu.Unlock()
			return version
		}
		if ctx.Err() != nil {
			c.versionMu.Unlock()
			return VersionUnknown
		}
		if pending := c.versionPending; pending != nil {
			c.versionMu.Unlock()
			select {
			case <-ctx.Done():
				return VersionUnknown
			case <-pending:
				// Retry if the previous caller's scan failed or was cancelled.
				continue
			}
		}

		pending := make(chan struct{})
		c.versionPending = pending
		c.versionMu.Unlock()

		version, err := c.detectVersion(ctx)

		c.versionMu.Lock()
		if err == nil {
			c.version = version
			c.versionDetected = true
		}
		c.versionPending = nil
		close(pending)
		c.versionMu.Unlock()
		return version
	}
}

// MetricsPeriodOrder returns the appropriate period order based on detected Sidekiq version.
// Sidekiq 7 metrics are limited to 8 hours.
func (c *Client) MetricsPeriodOrder(ctx context.Context) []string {
	version := c.DetectVersion(ctx)
	if version == Version7 {
		return MetricsPeriodOrderSidekiq7
	}
	return MetricsPeriodOrder
}

func (c *Client) detectVersion(ctx context.Context) (Version, error) {
	// Sidekiq 8 uses j|YYMMDD|H:M (6-digit date)
	// Sidekiq 7 uses j|YYYYMMDD|H:M (8-digit date)
	// We can distinguish by the date portion length after "j|"
	// If both formats exist (during upgrade), prefer Version8

	const sampleLimit = 10

	cursor := uint64(0)
	found7 := false
	processed := 0

	for {
		// Redis can return zero keys and a cursor for the next scan.
		keys, nextCursor, err := c.redis.Scan(ctx, cursor, "j|*", 100).Result()
		if err != nil {
			return VersionUnknown, err
		}

		for _, key := range keys {
			processed++
			switch metricsKeyVersion(key) {
			case Version8:
				return Version8, nil
			case Version7:
				found7 = true
			case VersionUnknown:
			}
		}

		cursor = nextCursor
		if processed >= sampleLimit || cursor == 0 {
			break
		}
	}

	if found7 {
		return Version7, nil
	}

	return VersionUnknown, nil
}

func metricsKeyVersion(key string) Version {
	if len(key) < 4 {
		return VersionUnknown
	}

	// Key format: j|DATE|H:M - find second pipe to get date length.
	pipeIdx := strings.IndexRune(key[2:], '|')
	switch pipeIdx {
	case 6:
		return Version8
	case 8:
		return Version7
	default:
		return VersionUnknown
	}
}
