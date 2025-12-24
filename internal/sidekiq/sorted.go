package sidekiq

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

// SortedEntry represents a job stored in a Sidekiq sorted set (dead, retry, schedule).
// Score is typically a Unix timestamp.
type SortedEntry struct {
	value string                 // raw JSON from Redis
	item  map[string]interface{} // parsed job data
	score float64                // sorted set score (timestamp)
}

// NewSortedEntry creates a SortedEntry from raw JSON data and score.
func NewSortedEntry(value string, score float64) *SortedEntry {
	se := &SortedEntry{
		value: value,
		score: score,
	}

	if err := json.Unmarshal([]byte(value), &se.item); err != nil {
		se.item = make(map[string]interface{})
	}

	return se
}

// Score returns the sorted set score (typically Unix timestamp).
func (se *SortedEntry) Score() float64 {
	return se.score
}

// At returns the timestamp as Unix seconds (same as score for dead/retry/schedule).
func (se *SortedEntry) At() int64 {
	return int64(se.score)
}

// Queue returns the queue name for this job.
func (se *SortedEntry) Queue() string {
	if q, ok := se.item["queue"].(string); ok {
		return q
	}
	return ""
}

// JID returns the job ID.
func (se *SortedEntry) JID() string {
	if jid, ok := se.item["jid"].(string); ok {
		return jid
	}
	return ""
}

// Klass returns the job class.
func (se *SortedEntry) Klass() string {
	if klass, ok := se.item["class"].(string); ok {
		return klass
	}
	return ""
}

// DisplayClass returns a human-friendly class name, unwrapping known wrappers.
func (se *SortedEntry) DisplayClass() string {
	klass := se.Klass()

	// Unwrap ActiveJob wrapper
	if klass == "ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper" || klass == "Sidekiq::ActiveJob::Wrapper" {
		if wrapped, ok := se.item["wrapped"].(string); ok {
			return wrapped
		}
		// Fall back to first arg if wrapped not present
		if args := se.Args(); len(args) > 0 {
			if firstArg, ok := args[0].(string); ok {
				return firstArg
			}
		}
	}

	return klass
}

// Args returns the job arguments.
func (se *SortedEntry) Args() []interface{} {
	if args, ok := se.item["args"].([]interface{}); ok {
		return args
	}
	return nil
}

// ErrorClass returns the error class if this job failed.
func (se *SortedEntry) ErrorClass() string {
	if errClass, ok := se.item["error_class"].(string); ok {
		return errClass
	}
	return ""
}

// ErrorMessage returns the error message if this job failed.
func (se *SortedEntry) ErrorMessage() string {
	if errMsg, ok := se.item["error_message"].(string); ok {
		return errMsg
	}
	return ""
}

// HasError returns true if this job has error information.
func (se *SortedEntry) HasError() bool {
	_, ok := se.item["error_class"]
	return ok
}

// RetryCount returns the number of times this job has been retried.
func (se *SortedEntry) RetryCount() int {
	if rc, ok := se.item["retry_count"].(float64); ok {
		return int(rc)
	}
	return 0
}

// Item returns the full parsed job data.
func (se *SortedEntry) Item() map[string]interface{} {
	return se.item
}

// Value returns the raw JSON string from Redis.
func (se *SortedEntry) Value() string {
	return se.value
}

// getSortedSetJobs fetches jobs from a sorted set with pagination.
// If reverse is true, returns highest scores first (ZREVRANGE), otherwise lowest first (ZRANGE).
func (c *Client) getSortedSetJobs(ctx context.Context, key string, start, count int, reverse bool) ([]*SortedEntry, int64, error) {
	size, err := c.redis.ZCard(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return nil, 0, err
	}

	if size == 0 {
		return nil, 0, nil
	}

	end := int64(start + count - 1)
	var results []redis.Z
	if reverse {
		results, err = c.redis.ZRevRangeWithScores(ctx, key, int64(start), end).Result()
	} else {
		results, err = c.redis.ZRangeWithScores(ctx, key, int64(start), end).Result()
	}
	if err != nil {
		return nil, size, err
	}

	entries := make([]*SortedEntry, len(results))
	for i, z := range results {
		value, _ := z.Member.(string)
		entries[i] = NewSortedEntry(value, z.Score)
	}

	return entries, size, nil
}

// GetDeadJobs fetches dead jobs with pagination (newest first).
func (c *Client) GetDeadJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, "dead", start, count, true)
}

// GetRetryJobs fetches retry jobs with pagination (earliest retry first).
func (c *Client) GetRetryJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, "retry", start, count, false)
}

// GetScheduledJobs fetches scheduled jobs with pagination (earliest execution time first).
func (c *Client) GetScheduledJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, "schedule", start, count, false)
}
