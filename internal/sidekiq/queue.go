package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
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
// Mirrors Sidekiq::Queue.all.
func (c *Client) GetQueues(ctx context.Context) ([]*Queue, error) {
	names, err := c.redis.SMembers(ctx, "queues").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
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
// Mirrors Sidekiq::Queue#latency.
func (q *Queue) Latency(ctx context.Context) (float64, error) {
	entry, err := q.client.redis.LIndex(ctx, "queue:"+q.name, -1).Result()
	if errors.Is(err, redis.Nil) || entry == "" {
		return 0.0, nil
	}
	if err != nil {
		return 0.0, err
	}

	return queueLatency(entry, time.Now())
}

func queueLatency(entry string, now time.Time) (float64, error) {
	// Decode only the timestamp; arguments and other large fields are skipped.
	var jobData struct {
		EnqueuedAt any `json:"enqueued_at"`
	}
	if err := json.Unmarshal([]byte(entry), &jobData); err != nil {
		return 0.0, err
	}

	enqueuedAt := parseTimestamp(jobData.EnqueuedAt)
	if enqueuedAt.IsZero() {
		return 0.0, nil
	}

	return max(now.Sub(enqueuedAt).Seconds(), 0), nil
}

// QueueMetadata holds a queue's size and latency at refresh time.
type QueueMetadata struct {
	Name          string
	Size          int64
	Latency       float64
	OldestJobTime time.Time
}

const queueMetadataBatchSize = 100

// FetchQueueMetadata fetches metadata in bounded pipelines, preserving input order.
// Queues must come from the same Client. Missing queues and individual invalid
// payloads/keys retain zero values, matching the queue views' best-effort display.
// Transport errors and cancellation fail the entire snapshot.
func FetchQueueMetadata(ctx context.Context, queues []*Queue) ([]QueueMetadata, error) {
	metadata := make([]QueueMetadata, len(queues))
	for start := 0; start < len(queues); start += queueMetadataBatchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := queues[start:min(start+queueMetadataBatchSize, len(queues))]
		sizes := make([]*redis.IntCmd, len(batch))
		oldest := make([]*redis.StringCmd, len(batch))
		commands, err := batch[0].client.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for i, queue := range batch {
				key := "queue:" + queue.name
				sizes[i] = pipe.LLen(ctx, key)
				oldest[i] = pipe.LIndex(ctx, key, -1)
			}
			return nil
		})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var serverErr redis.Error
		if err != nil && !errors.As(err, &serverErr) {
			return nil, err
		}
		// Exec can report an earlier missing entry even if a later reply failed.
		for _, command := range commands {
			if err := command.Err(); err != nil && !errors.As(err, &serverErr) {
				return nil, err
			}
		}
		now := time.Now()
		for i, queue := range batch {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			info := QueueMetadata{Name: queue.name}
			info.Size, _ = sizes[i].Result()
			if entry, err := oldest[i].Result(); err == nil && entry != "" {
				info.Latency, _ = queueLatency(entry, now)
				if info.Latency > 0 {
					info.OldestJobTime = now.Add(-time.Duration(info.Latency * float64(time.Second)))
				}
			}
			metadata[start+i] = info
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return metadata, nil
}

// PositionedEntry wraps a JobRecord with its position in the queue.
type PositionedEntry struct {
	*JobRecord // embedded for method promotion
	Position   int
}

// QueueEntriesWindow holds a filtered window plus aggregate metadata.
type QueueEntriesWindow struct {
	Entries []*PositionedEntry
	Total   int64
}

// GetJobs fetches jobs from the queue with pagination.
// start is 0-indexed, count is the number of jobs to fetch.
// Jobs are returned newest-first (matching Sidekiq's default display order).
func (q *Queue) GetJobs(ctx context.Context, start, count int) ([]*PositionedEntry, int64, error) {
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

	jobs := make([]*PositionedEntry, len(entries))
	for i, entry := range entries {
		jobs[i] = q.newPositionedEntry(entry, int(size)-start-i)
	}

	return jobs, size, nil
}

// ScanJobsWindow scans queue jobs using a raw payload substring filter and returns one window.
func (q *Queue) ScanJobsWindow(ctx context.Context, filter string, start, count int) (QueueEntriesWindow, error) {
	size, err := q.Size(ctx)
	if err != nil {
		return QueueEntriesWindow{}, err
	}
	if size == 0 {
		return QueueEntriesWindow{}, nil
	}

	start = max(start, 0)
	count = max(count, 1)
	windowEnd := start + count
	batchSize := max(count, 100)

	window := QueueEntriesWindow{
		Entries: make([]*PositionedEntry, 0, count),
	}

	for batchStart := 0; batchStart < int(size); batchStart += batchSize {
		if err := ctx.Err(); err != nil {
			return QueueEntriesWindow{}, err
		}
		batchEnd := min(batchStart+batchSize-1, int(size)-1)
		entries, err := q.client.redis.LRange(ctx, "queue:"+q.name, int64(batchStart), int64(batchEnd)).Result()
		if err != nil {
			return QueueEntriesWindow{}, err
		}

		for i, entry := range entries {
			if err := ctx.Err(); err != nil {
				return QueueEntriesWindow{}, err
			}
			if filter != "" && !strings.Contains(entry, filter) {
				continue
			}

			if window.Total >= int64(start) && window.Total < int64(windowEnd) {
				window.Entries = append(window.Entries, q.newPositionedEntry(entry, int(size)-batchStart-i))
			}
			window.Total++
		}
	}

	return window, nil
}

// Clear deletes all jobs within this queue and removes it from the queues set.
func (q *Queue) Clear(ctx context.Context) error {
	if q.client == nil {
		return errors.New("queue client is nil")
	}

	_, err := q.client.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Unlink(ctx, "queue:"+q.name)
		pipe.SRem(ctx, "queues", q.name)
		return nil
	})
	return err
}

func (q *Queue) newPositionedEntry(entry string, position int) *PositionedEntry {
	return &PositionedEntry{
		JobRecord: NewJobRecord(entry, q.name),
		Position:  position,
	}
}
