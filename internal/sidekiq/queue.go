package sidekiq

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

// Queue represents a Sidekiq queue.
// Mirrors the Sidekiq::Queue Ruby class.
type Queue struct {
	client *Client
	name   string
}

// NewQueue creates a new Queue instance for the given queue name.
func (c *Client) NewQueue(name string) *Queue {
	return &Queue{
		client: c,
		name:   name,
	}
}

// GetQueues fetches all known queues from Redis, sorted alphabetically.
// Mirrors Sidekiq::Queue.all
func (c *Client) GetQueues(ctx context.Context) ([]*Queue, error) {
	names, err := c.redis.SMembers(ctx, "queues").Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	sort.Strings(names)

	queues := make([]*Queue, len(names))
	for i, name := range names {
		queues[i] = c.NewQueue(name)
	}

	return queues, nil
}

// Name returns the queue name.
func (q *Queue) Name() string {
	return q.name
}

// Size returns the current size of the queue.
// This value is real-time and can change between calls.
func (q *Queue) Size(ctx context.Context) (int64, error) {
	return q.client.redis.LLen(ctx, "queue:"+q.name).Result()
}

// Latency calculates the queue's latency - the difference in seconds
// since the oldest job in the queue was enqueued.
// Mirrors Sidekiq::Queue#latency
func (q *Queue) Latency(ctx context.Context) (float64, error) {
	entry, err := q.client.redis.LIndex(ctx, "queue:"+q.name, -1).Result()
	if err == redis.Nil || entry == "" {
		return 0.0, nil
	}
	if err != nil {
		return 0.0, err
	}

	var jobData map[string]interface{}
	if err := json.Unmarshal([]byte(entry), &jobData); err != nil {
		return 0.0, nil
	}

	enqueuedAt, ok := jobData["enqueued_at"].(float64)
	if !ok {
		return 0.0, nil
	}

	// Sidekiq enqueued_at can be:
	// - Old format: float seconds (e.g., 1703000000.123)
	// - New format: integer milliseconds (e.g., 1703000000123)
	// Detect by magnitude: if > 1e12, it's milliseconds
	var latency float64
	if enqueuedAt > 1e12 {
		// New format: milliseconds
		nowMs := float64(time.Now().UnixMilli())
		latency = (nowMs - enqueuedAt) / 1000.0
	} else {
		// Old format: seconds
		nowSec := float64(time.Now().Unix())
		latency = nowSec - enqueuedAt
	}

	if latency < 0 {
		latency = 0
	}

	return latency, nil
}

// JobRecord represents a pending job within a Sidekiq queue.
// Mirrors Sidekiq::JobRecord
type JobRecord struct {
	value   string                 // the underlying String in Redis
	item    map[string]interface{} // the parsed job data
	queue   string                 // the queue associated with this job
	Position int                   // position in queue (for display)
}

// NewJobRecord creates a JobRecord from raw JSON data.
func NewJobRecord(value string, queueName string, position int) *JobRecord {
	jr := &JobRecord{
		value:    value,
		queue:    queueName,
		Position: position,
	}

	if err := json.Unmarshal([]byte(value), &jr.item); err != nil {
		jr.item = make(map[string]interface{})
	}

	return jr
}

// Queue returns the queue name associated with this job.
func (jr *JobRecord) Queue() string {
	return jr.queue
}

// JID returns the job ID.
func (jr *JobRecord) JID() string {
	if jid, ok := jr.item["jid"].(string); ok {
		return jid
	}
	return ""
}

// Klass returns the job class which Sidekiq will execute.
func (jr *JobRecord) Klass() string {
	if klass, ok := jr.item["class"].(string); ok {
		return klass
	}
	return ""
}

// DisplayClass returns a human-friendly class name, unwrapping known wrappers.
func (jr *JobRecord) DisplayClass() string {
	klass := jr.Klass()

	// Unwrap ActiveJob wrapper
	if klass == "ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper" || klass == "Sidekiq::ActiveJob::Wrapper" {
		if wrapped, ok := jr.item["wrapped"].(string); ok {
			return wrapped
		}
		// Fall back to first arg if wrapped not present
		if args := jr.Args(); len(args) > 0 {
			if firstArg, ok := args[0].(string); ok {
				return firstArg
			}
		}
	}

	return klass
}

// Args returns the job arguments.
func (jr *JobRecord) Args() []interface{} {
	if args, ok := jr.item["args"].([]interface{}); ok {
		return args
	}
	return nil
}

// Context returns the current attributes (cattr) for the job.
func (jr *JobRecord) Context() map[string]interface{} {
	if cattr, ok := jr.item["cattr"].(map[string]interface{}); ok {
		return cattr
	}
	return nil
}

// Item returns the full parsed job data.
func (jr *JobRecord) Item() map[string]interface{} {
	return jr.item
}

// Value returns the raw JSON string from Redis.
func (jr *JobRecord) Value() string {
	return jr.value
}

// GetJobs fetches jobs from the queue with pagination.
// start is 0-indexed, count is the number of jobs to fetch.
// Jobs are returned newest-first (matching Sidekiq's default display order).
func (q *Queue) GetJobs(ctx context.Context, start, count int) ([]*JobRecord, int64, error) {
	// Get total size for position calculation
	size, err := q.Size(ctx)
	if err != nil {
		return nil, 0, err
	}

	if size == 0 {
		return nil, 0, nil
	}

	// Fetch jobs from Redis (newest jobs at lower indices)
	end := start + count - 1
	entries, err := q.client.redis.LRange(ctx, "queue:"+q.name, int64(start), int64(end)).Result()
	if err != nil {
		return nil, size, err
	}

	jobs := make([]*JobRecord, len(entries))
	for i, entry := range entries {
		// Position is calculated as total_size - index (descending, matching Sidekiq UI)
		position := int(size) - start - i
		jobs[i] = NewJobRecord(entry, q.name, position)
	}

	return jobs, size, nil
}
