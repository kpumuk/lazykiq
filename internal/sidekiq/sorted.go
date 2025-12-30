package sidekiq

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const sortedSetScanCount int64 = 100

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
func (se *SortedEntry) At() int64 {
	return int64(se.Score)
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

// GetDeadJobs fetches dead jobs with pagination (newest first).
func (c *Client) GetDeadJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, "dead", start, count, true)
}

// ScanDeadJobs scans dead jobs using a match pattern (no paging).
func (c *Client) ScanDeadJobs(ctx context.Context, match string) ([]*SortedEntry, error) {
	return c.scanSortedSetJobs(ctx, "dead", match, true)
}

// GetRetryJobs fetches retry jobs with pagination (earliest retry first).
func (c *Client) GetRetryJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, "retry", start, count, false)
}

// ScanRetryJobs scans retry jobs using a match pattern (no paging).
func (c *Client) ScanRetryJobs(ctx context.Context, match string) ([]*SortedEntry, error) {
	return c.scanSortedSetJobs(ctx, "retry", match, false)
}

// GetScheduledJobs fetches scheduled jobs with pagination (earliest execution time first).
func (c *Client) GetScheduledJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error) {
	return c.getSortedSetJobs(ctx, "schedule", start, count, false)
}

// ScanScheduledJobs scans scheduled jobs using a match pattern (no paging).
func (c *Client) ScanScheduledJobs(ctx context.Context, match string) ([]*SortedEntry, error) {
	return c.scanSortedSetJobs(ctx, "schedule", match, false)
}
