package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sortedSetScanCount int64 = 100
	sortedSetPopBatch  int64 = 100
)

const (
	retrySetKey    = "retry"
	scheduleSetKey = "schedule"
	deadSetKey     = "dead"
	queueSetKey    = "queues"
	queuePrefixKey = "queue:"
)

// SortedEntry represents a job stored in a Sidekiq sorted set (dead, retry, schedule).
// It embeds a JobRecord for the job data and adds the sorted set score (timestamp).
type SortedEntry struct {
	*JobRecord         // the actual job data (embedded for method promotion)
	Score      float64 // sorted set score (timestamp)
}

// NewSortedEntry creates a SortedEntry from raw JSON data and score.
func NewSortedEntry(value string, score float64) *SortedEntry {
	return &SortedEntry{
		JobRecord: NewJobRecord(value, ""),
		Score:     score,
	}
}

// At returns the timestamp as Unix seconds (same as score for dead/retry/schedule).
func (se *SortedEntry) At() time.Time {
	return time.Unix(0, int64(se.Score*float64(time.Second)))
}

// getSortedSetJobs fetches jobs from a sorted set with pagination.
// If reverse is true, returns highest scores first (ZREVRANGE), otherwise lowest first (ZRANGE).
func (c *Client) getSortedSetJobs(ctx context.Context, key string, start, count int, reverse bool) ([]*SortedEntry, int64, error) {
	size, err := c.redis.ZCard(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, 0, err
	}

	if size == 0 {
		return nil, 0, nil
	}

	end := int64(start + count - 1)
	if count <= 0 {
		end = -1
	}
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

func (c *Client) scanSortedSetJobs(ctx context.Context, key, match string, reverse bool) ([]*SortedEntry, error) {
	if match != "" && !strings.Contains(match, "*") {
		match = "*" + match + "*"
	}

	var cursor uint64
	var entries []*SortedEntry
	for {
		values, nextCursor, err := c.redis.ZScan(ctx, key, cursor, match, sortedSetScanCount).Result()
		if err != nil {
			return nil, err
		}

		for i := 0; i+1 < len(values); i += 2 {
			score, err := strconv.ParseFloat(values[i+1], 64)
			if err != nil {
				continue
			}
			entries = append(entries, NewSortedEntry(values[i], score))
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if reverse {
			return entries[i].Score > entries[j].Score
		}
		return entries[i].Score < entries[j].Score
	})

	return entries, nil
}

func (c *Client) getSortedSetBounds(ctx context.Context, key string) (*SortedEntry, *SortedEntry, error) {
	pipe := c.redis.Pipeline()
	minCmd := pipe.ZRangeWithScores(ctx, key, 0, 0)
	maxCmd := pipe.ZRevRangeWithScores(ctx, key, 0, 0)

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, nil, err
	}

	minResults, err := minCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, nil, err
	}
	if len(minResults) == 0 {
		return nil, nil, nil
	}

	maxResults, err := maxCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, nil, err
	}
	if len(maxResults) == 0 {
		return nil, nil, nil
	}

	minValue, _ := minResults[0].Member.(string)
	maxValue, _ := maxResults[0].Member.(string)
	return NewSortedEntry(minValue, minResults[0].Score), NewSortedEntry(maxValue, maxResults[0].Score), nil
}

// GetDeadJobs fetches dead jobs with pagination (newest first).
func (c *Client) GetDeadJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, deadSetKey, start, count, true)
}

// ScanDeadJobs scans dead jobs using a match pattern (no paging).
func (c *Client) ScanDeadJobs(ctx context.Context, match string) ([]*SortedEntry, error) {
	return c.scanSortedSetJobs(ctx, deadSetKey, match, true)
}

// GetDeadBounds fetches the oldest and newest dead jobs.
func (c *Client) GetDeadBounds(ctx context.Context) (*SortedEntry, *SortedEntry, error) {
	return c.getSortedSetBounds(ctx, deadSetKey)
}

// GetRetryJobs fetches retry jobs with pagination (earliest retry first).
func (c *Client) GetRetryJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, retrySetKey, start, count, false)
}

// ScanRetryJobs scans retry jobs using a match pattern (no paging).
func (c *Client) ScanRetryJobs(ctx context.Context, match string) ([]*SortedEntry, error) {
	return c.scanSortedSetJobs(ctx, retrySetKey, match, false)
}

// GetRetryBounds fetches the earliest and latest retry jobs.
func (c *Client) GetRetryBounds(ctx context.Context) (*SortedEntry, *SortedEntry, error) {
	return c.getSortedSetBounds(ctx, retrySetKey)
}

// GetScheduledJobs fetches scheduled jobs with pagination (earliest execution time first).
func (c *Client) GetScheduledJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, scheduleSetKey, start, count, false)
}

// ScanScheduledJobs scans scheduled jobs using a match pattern (no paging).
func (c *Client) ScanScheduledJobs(ctx context.Context, match string) ([]*SortedEntry, error) {
	return c.scanSortedSetJobs(ctx, scheduleSetKey, match, false)
}

// GetScheduledBounds fetches the earliest and latest scheduled jobs.
func (c *Client) GetScheduledBounds(ctx context.Context) (*SortedEntry, *SortedEntry, error) {
	return c.getSortedSetBounds(ctx, scheduleSetKey)
}

// DeleteRetryJob removes a job from the retry set.
func (c *Client) DeleteRetryJob(ctx context.Context, entry *SortedEntry) error {
	return c.deleteSortedEntry(ctx, retrySetKey, entry)
}

// KillRetryJob moves a retry job into the dead set.
func (c *Client) KillRetryJob(ctx context.Context, entry *SortedEntry) error {
	if entry == nil || entry.JobRecord == nil {
		return errors.New("sorted entry is nil")
	}
	value := entry.Value()
	if value == "" {
		return errors.New("sorted entry payload is empty")
	}

	_, err := c.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRem(ctx, retrySetKey, value)
		pipe.ZAdd(ctx, deadSetKey, redis.Z{
			Score:  float64(time.Now().Truncate(time.Microsecond).UnixNano()) / float64(time.Second),
			Member: value,
		})
		return nil
	})
	return err
}

// RetryNowRetryJob moves a retry job to its queue immediately (Sidekiq "Retry Now").
func (c *Client) RetryNowRetryJob(ctx context.Context, entry *SortedEntry) error {
	return c.moveSortedEntryToQueue(ctx, retrySetKey, entry, true)
}

// RetryNowDeadJob moves a dead job to its queue immediately (Sidekiq "Retry Now").
func (c *Client) RetryNowDeadJob(ctx context.Context, entry *SortedEntry) error {
	return c.moveSortedEntryToQueue(ctx, deadSetKey, entry, true)
}

// AddScheduledJobToQueue moves a scheduled job to its queue immediately (Sidekiq "Add to queue").
func (c *Client) AddScheduledJobToQueue(ctx context.Context, entry *SortedEntry) error {
	return c.moveSortedEntryToQueue(ctx, scheduleSetKey, entry, false)
}

func (c *Client) moveSortedEntryToQueue(ctx context.Context, key string, entry *SortedEntry, decrementRetryCount bool) error {
	if entry == nil || entry.JobRecord == nil {
		return errors.New("sorted entry is nil")
	}
	rawValue := entry.Value()
	if rawValue == "" {
		return errors.New("sorted entry payload is empty")
	}

	queueName, encoded, err := buildQueuePayload(rawValue, decrementRetryCount, c.DetectVersion(ctx))
	if err != nil {
		return err
	}

	removed, err := c.redis.ZRem(ctx, key, rawValue).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	if removed == 0 {
		return errors.New("job not found")
	}

	_, err = c.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SAdd(ctx, queueSetKey, queueName)
		pipe.LPush(ctx, queuePrefixKey+queueName, encoded)
		return nil
	})
	return err
}

// DeleteAllRetryJobs removes all jobs from the retry set.
func (c *Client) DeleteAllRetryJobs(ctx context.Context) error {
	return c.clearSortedSet(ctx, retrySetKey)
}

// RetryAllRetryJobs moves all retry jobs to their queues immediately.
func (c *Client) RetryAllRetryJobs(ctx context.Context) error {
	return c.moveAllSortedEntriesToQueue(ctx, retrySetKey, true)
}

// KillAllRetryJobs moves all retry jobs into the dead set.
func (c *Client) KillAllRetryJobs(ctx context.Context) error {
	return c.moveAllSortedEntriesToDead(ctx, retrySetKey)
}

// DeleteScheduledJob removes a job from the scheduled set.
func (c *Client) DeleteScheduledJob(ctx context.Context, entry *SortedEntry) error {
	return c.deleteSortedEntry(ctx, scheduleSetKey, entry)
}

// DeleteAllScheduledJobs removes all jobs from the scheduled set.
func (c *Client) DeleteAllScheduledJobs(ctx context.Context) error {
	return c.clearSortedSet(ctx, scheduleSetKey)
}

// AddAllScheduledJobsToQueue moves all scheduled jobs to their queues immediately.
func (c *Client) AddAllScheduledJobsToQueue(ctx context.Context) error {
	return c.moveAllSortedEntriesToQueue(ctx, scheduleSetKey, false)
}

// DeleteDeadJob removes a job from the dead set.
func (c *Client) DeleteDeadJob(ctx context.Context, entry *SortedEntry) error {
	return c.deleteSortedEntry(ctx, deadSetKey, entry)
}

// DeleteAllDeadJobs removes all jobs from the dead set.
func (c *Client) DeleteAllDeadJobs(ctx context.Context) error {
	return c.clearSortedSet(ctx, deadSetKey)
}

// RetryAllDeadJobs moves all dead jobs to their queues immediately.
func (c *Client) RetryAllDeadJobs(ctx context.Context) error {
	return c.moveAllSortedEntriesToQueue(ctx, deadSetKey, true)
}

func (c *Client) deleteSortedEntry(ctx context.Context, key string, entry *SortedEntry) error {
	if entry == nil || entry.JobRecord == nil {
		return errors.New("sorted entry is nil")
	}
	value := entry.Value()
	if value == "" {
		return errors.New("sorted entry payload is empty")
	}

	_, err := c.redis.ZRem(ctx, key, value).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return nil
}

func (c *Client) clearSortedSet(ctx context.Context, key string) error {
	_, err := c.redis.Unlink(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return nil
}

type queuePayload struct {
	queue string
	body  []byte
}

func buildQueuePayload(rawValue string, decrementRetryCount bool, version Version) (string, []byte, error) {
	if rawValue == "" {
		return "", nil, errors.New("sorted entry payload is empty")
	}

	payload := make(map[string]any)
	if err := safeParseJSON([]byte(rawValue), &payload); err != nil {
		return "", nil, err
	}

	queueName, ok := payload["queue"].(string)
	if !ok || strings.TrimSpace(queueName) == "" {
		return "", nil, errors.New("job payload missing queue")
	}

	format := detectTimestampFormat(payload, version)
	if decrementRetryCount {
		decrementRetryCountField(payload)
	}

	// Ensure we always enqueue immediately.
	delete(payload, "at")

	if payload["created_at"] == nil {
		payload["created_at"] = nowTimestamp(format)
	}
	payload["enqueued_at"] = nowTimestamp(format)

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}

	return queueName, encoded, nil
}

func (c *Client) moveAllSortedEntriesToQueue(ctx context.Context, key string, decrementRetryCount bool) error {
	version := c.DetectVersion(ctx)
	for {
		entries, err := c.redis.ZPopMin(ctx, key, sortedSetPopBatch).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if len(entries) == 0 {
			return nil
		}

		payloads := make([]queuePayload, 0, len(entries))
		for _, entry := range entries {
			rawValue, _ := entry.Member.(string)
			queueName, encoded, err := buildQueuePayload(rawValue, decrementRetryCount, version)
			if err != nil {
				return err
			}
			payloads = append(payloads, queuePayload{
				queue: queueName,
				body:  encoded,
			})
		}

		_, err = c.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, payload := range payloads {
				pipe.SAdd(ctx, queueSetKey, payload.queue)
				pipe.LPush(ctx, queuePrefixKey+payload.queue, payload.body)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
}

func (c *Client) moveAllSortedEntriesToDead(ctx context.Context, key string) error {
	for {
		entries, err := c.redis.ZPopMin(ctx, key, sortedSetPopBatch).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if len(entries) == 0 {
			return nil
		}

		_, err = c.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, entry := range entries {
				rawValue, _ := entry.Member.(string)
				if rawValue == "" {
					continue
				}
				now := time.Now().Truncate(time.Microsecond)
				pipe.ZAdd(ctx, deadSetKey, redis.Z{
					Score:  float64(now.UnixNano()) / float64(time.Second),
					Member: rawValue,
				})
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
}

var nowFuncSidekiq = time.Now

func nowTimestamp(format timestampFormat) json.Number {
	now := nowFuncSidekiq()
	if format == timestampMilliseconds {
		ms := now.UnixNano() / int64(time.Millisecond)
		return json.Number(strconv.FormatInt(ms, 10))
	}
	seconds := float64(now.UnixNano()) / float64(time.Second)
	seconds = math.Round(seconds*1e6) / 1e6
	return json.Number(strconv.FormatFloat(seconds, 'f', -1, 64))
}

func decrementRetryCountField(payload map[string]any) {
	raw, ok := payload["retry_count"]
	if !ok || raw == nil {
		return
	}
	count, ok := parseOptionalInt64(raw)
	if !ok {
		return
	}
	payload["retry_count"] = json.Number(strconv.FormatInt(count-1, 10))
}
