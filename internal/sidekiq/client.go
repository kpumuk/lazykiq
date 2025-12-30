// Package sidekiq provides a Redis-backed Sidekiq client and data models.
package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// Process represents a Sidekiq worker process.
type Process struct {
	Identity    string   // hostname:pid:nonce (e.g., "be4860dbdb68:14:96908d62200c")
	Hostname    string   // Parsed from identity (e.g., "be4860dbdb68")
	PID         string   // Parsed from identity (e.g., "14")
	Tag         string   // From info.tag (e.g., "myapp")
	Concurrency int      // From info.concurrency
	Busy        int      // From busy field (converted to int)
	Queues      []string // From info.queues
	RSS         int64    // From rss field in KB, convert to bytes (*1024)
	StartedAt   int64    // From info.started_at (Unix timestamp)
}

// Job represents an active Sidekiq job (currently running).
type Job struct {
	*JobRecord             // embedded job data from payload
	ProcessIdentity string // process identity running this job
	ThreadID        string // Base-36 encoded TID
	RunAt           int64  // Unix timestamp when job started
}

// BusyData holds process and job information.
type BusyData struct {
	Processes []Process
	Jobs      []Job
}

// Client is a Sidekiq API client.
type Client struct {
	redis *redis.Client
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

	return &Client{redis: rdb}, nil
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

// GetBusyData fetches detailed process and active job information from Redis.
func (c *Client) GetBusyData(ctx context.Context) (BusyData, error) {
	var data BusyData

	// Get all process identities
	processes, err := c.redis.SMembers(ctx, "processes").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return data, err
	}

	// Fetch each process details
	for _, identity := range processes {
		// Get process hash fields
		fields, err := c.redis.HMGet(ctx, identity, "info", "busy", "rss").Result()
		if err != nil {
			continue
		}

		// Check if we got results
		if len(fields) < 3 {
			continue
		}

		// Parse process info
		process := Process{
			Identity: identity,
		}

		// Parse identity to extract hostname and PID (format: hostname:pid:nonce)
		parts := strings.Split(identity, ":")
		if len(parts) >= 2 {
			process.Hostname = parts[0]
			process.PID = parts[1]
		}

		// Parse info JSON
		parseProcessInfo(fields[0], &process)

		// Parse busy count
		if busyCount, ok := parseOptionalInt64(fields[1]); ok {
			process.Busy = int(busyCount)
		}

		// Parse RSS (in KB, convert to bytes)
		if rss, ok := parseOptionalInt64(fields[2]); ok {
			process.RSS = rss * 1024
		}

		data.Processes = append(data.Processes, process)

		// Get active jobs for this process
		workKey := identity + ":work"
		work, err := c.redis.HGetAll(ctx, workKey).Result()
		if err != nil {
			continue
		}

		// Parse each job
		for tid, workJSON := range work {
			var workData map[string]any
			if err := json.Unmarshal([]byte(workJSON), &workData); err != nil {
				continue
			}

			job := Job{
				ProcessIdentity: identity,
				ThreadID:        tid,
			}

			// Get run_at from work data
			if runAt, ok := workData["run_at"].(float64); ok {
				job.RunAt = int64(runAt)
			}

			// Parse payload as JobRecord (queue is inside payload, fallback to workData)
			if payloadStr, ok := workData["payload"].(string); ok {
				queueName := ""
				if q, ok := workData["queue"].(string); ok {
					queueName = q
				}
				job.JobRecord = NewJobRecord(payloadStr, queueName)
			}

			data.Jobs = append(data.Jobs, job)
		}
	}

	return data, nil
}

func parseProcessInfo(field any, process *Process) {
	infoStr, ok := field.(string)
	if !ok || infoStr == "" {
		return
	}

	var info map[string]any
	if err := json.Unmarshal([]byte(infoStr), &info); err != nil {
		return
	}

	if concurrency, ok := info["concurrency"].(float64); ok {
		process.Concurrency = int(concurrency)
	}
	if queues, ok := info["queues"].([]any); ok {
		process.Queues = make([]string, 0, len(queues))
		for _, q := range queues {
			queueName, ok := q.(string)
			if ok {
				process.Queues = append(process.Queues, queueName)
			}
		}
	}
	if tag, ok := info["tag"].(string); ok {
		process.Tag = tag
	}
	if startedAt, ok := info["started_at"].(float64); ok {
		process.StartedAt = int64(startedAt)
	}
}

func parseOptionalInt64(field any) (int64, bool) {
	value, ok := field.(string)
	if !ok || value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
