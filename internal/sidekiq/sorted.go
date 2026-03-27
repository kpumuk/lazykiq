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

// SortedSetKind identifies one of Sidekiq's time-ordered job sets.
type SortedSetKind int

const (
	// SortedSetRetry identifies the retry set.
	SortedSetRetry SortedSetKind = iota
	// SortedSetScheduled identifies the scheduled set.
	SortedSetScheduled
	// SortedSetDead identifies the dead set.
	SortedSetDead
)

func (k SortedSetKind) String() string {
	switch k {
	case SortedSetRetry:
		return "retry"
	case SortedSetScheduled:
		return "scheduled"
	case SortedSetDead:
		return "dead"
	default:
		return "unknown"
	}
}

type sortedSetSpec struct {
	key                 string
	reverse             bool
	decrementRetryCount bool
	canMoveToDead       bool
}

func sortedSetSpecFor(kind SortedSetKind) (sortedSetSpec, error) {
	switch kind {
	case SortedSetRetry:
		return sortedSetSpec{
			key:                 retrySetKey,
			decrementRetryCount: true,
			canMoveToDead:       true,
		}, nil
	case SortedSetScheduled:
		return sortedSetSpec{
			key: scheduleSetKey,
		}, nil
	case SortedSetDead:
		return sortedSetSpec{
			key:                 deadSetKey,
			reverse:             true,
			decrementRetryCount: true,
		}, nil
	default:
		return sortedSetSpec{}, errors.New("unsupported sorted set kind")
	}
}

// SortedEntry represents a job stored in a Sidekiq sorted set (dead, retry, schedule).
// It embeds a JobRecord for the job data and adds the sorted set score (timestamp).
type SortedEntry struct {
	*JobRecord         // the actual job data (embedded for method promotion)
	Score      float64 // sorted set score (timestamp)
}

// SortedEntriesWindow holds a filtered window plus aggregate metadata.
type SortedEntriesWindow struct {
	Entries    []*SortedEntry
	Total      int64
	FirstEntry *SortedEntry
	LastEntry  *SortedEntry
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
	window, err := c.scanSortedSetWindow(ctx, key, match, 0, -1, reverse)
	if err != nil {
		return nil, err
	}
	return window.Entries, nil
}

func (c *Client) scanSortedSetWindow(
	ctx context.Context,
	key, match string,
	start, count int,
	reverse bool,
) (SortedEntriesWindow, error) {
	start = max(start, 0)
	match = normalizeSortedSetMatch(match)

	limit := -1
	if count > 0 {
		limit = start + count
	}

	result := SortedEntriesWindow{}
	selected := make([]*SortedEntry, 0, max(min(limit, int(sortedSetScanCount)), 0))
	var cursor uint64
	for {
		values, nextCursor, err := c.redis.ZScan(ctx, key, cursor, match, sortedSetScanCount).Result()
		if err != nil {
			return SortedEntriesWindow{}, err
		}

		for i := 0; i+1 < len(values); i += 2 {
			score, err := strconv.ParseFloat(values[i+1], 64)
			if err != nil {
				continue
			}

			entry := NewSortedEntry(values[i], score)
			result.Total++
			if result.FirstEntry == nil || sortedEntryBefore(entry, result.FirstEntry, reverse) {
				result.FirstEntry = entry
			}
			if result.LastEntry == nil || sortedEntryBefore(result.LastEntry, entry, reverse) {
				result.LastEntry = entry
			}

			if limit < 0 {
				selected = append(selected, entry)
				continue
			}
			selected = insertSortedEntry(selected, entry, reverse, limit)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if limit < 0 {
		sortSortedEntries(selected, reverse)
	}
	if start >= len(selected) {
		return result, nil
	}
	if count > 0 {
		end := min(start+count, len(selected))
		result.Entries = append([]*SortedEntry(nil), selected[start:end]...)
		return result, nil
	}

	result.Entries = append([]*SortedEntry(nil), selected[start:]...)
	return result, nil
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

// GetSortedEntries fetches sorted-set jobs with pagination.
func (c *Client) GetSortedEntries(
	ctx context.Context,
	kind SortedSetKind,
	start, count int,
) ([]*SortedEntry, int64, error) {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return nil, 0, err
	}
	return c.getSortedSetJobs(ctx, spec.key, start, count, spec.reverse)
}

// ScanSortedEntries scans sorted-set jobs using a match pattern (no paging).
func (c *Client) ScanSortedEntries(ctx context.Context, kind SortedSetKind, match string) ([]*SortedEntry, error) {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return nil, err
	}
	return c.scanSortedSetJobs(ctx, spec.key, match, spec.reverse)
}

// ScanSortedEntriesWindow scans sorted-set jobs using a match pattern and returns one window.
func (c *Client) ScanSortedEntriesWindow(
	ctx context.Context,
	kind SortedSetKind,
	match string,
	start, count int,
) (SortedEntriesWindow, error) {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return SortedEntriesWindow{}, err
	}
	return c.scanSortedSetWindow(ctx, spec.key, match, start, count, spec.reverse)
}

// GetSortedEntryBounds fetches the oldest and newest entries for a sorted set.
func (c *Client) GetSortedEntryBounds(
	ctx context.Context,
	kind SortedSetKind,
) (*SortedEntry, *SortedEntry, error) {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return nil, nil, err
	}
	return c.getSortedSetBounds(ctx, spec.key)
}

// DeleteSortedEntry removes one job from a sorted set.
func (c *Client) DeleteSortedEntry(ctx context.Context, kind SortedSetKind, entry *SortedEntry) error {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return err
	}
	return c.deleteSortedEntry(ctx, spec.key, entry)
}

// MoveSortedEntryToDead moves a supported sorted-set job into the dead set.
func (c *Client) MoveSortedEntryToDead(ctx context.Context, kind SortedSetKind, entry *SortedEntry) error {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return err
	}
	if !spec.canMoveToDead {
		return errors.New("sorted set does not support move to dead: " + kind.String())
	}
	return c.moveSortedEntryToDead(ctx, spec.key, entry)
}

func (c *Client) moveSortedEntryToDead(ctx context.Context, key string, entry *SortedEntry) error {
	if entry == nil || entry.JobRecord == nil {
		return errors.New("sorted entry is nil")
	}
	value := entry.Value()
	if value == "" {
		return errors.New("sorted entry payload is empty")
	}

	_, err := c.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRem(ctx, key, value)
		pipe.ZAdd(ctx, deadSetKey, redis.Z{
			Score:  nowSortedSetScore(),
			Member: value,
		})
		return nil
	})
	return err
}

// EnqueueSortedEntry moves a sorted-set job to its queue immediately.
func (c *Client) EnqueueSortedEntry(ctx context.Context, kind SortedSetKind, entry *SortedEntry) error {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return err
	}
	return c.moveSortedEntryToQueue(ctx, spec.key, entry, spec.decrementRetryCount)
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

// DeleteAllSortedEntries removes all jobs from a sorted set.
func (c *Client) DeleteAllSortedEntries(ctx context.Context, kind SortedSetKind) error {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return err
	}
	return c.clearSortedSet(ctx, spec.key)
}

// EnqueueAllSortedEntries moves all jobs from a sorted set to their queues immediately.
func (c *Client) EnqueueAllSortedEntries(ctx context.Context, kind SortedSetKind) error {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return err
	}
	return c.moveAllSortedEntriesToQueue(ctx, spec.key, spec.decrementRetryCount)
}

// MoveAllSortedEntriesToDead moves all jobs from a supported sorted set into the dead set.
func (c *Client) MoveAllSortedEntriesToDead(ctx context.Context, kind SortedSetKind) error {
	spec, err := sortedSetSpecFor(kind)
	if err != nil {
		return err
	}
	if !spec.canMoveToDead {
		return errors.New("sorted set does not support move to dead: " + kind.String())
	}
	return c.moveAllSortedEntriesToDead(ctx, spec.key)
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
				pipe.ZAdd(ctx, deadSetKey, redis.Z{
					Score:  nowSortedSetScore(),
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

func nowSortedSetScore() float64 {
	return float64(nowFuncSidekiq().Truncate(time.Microsecond).UnixNano()) / float64(time.Second)
}

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

func normalizeSortedSetMatch(match string) string {
	if match != "" && !strings.Contains(match, "*") {
		return "*" + match + "*"
	}
	return match
}

func (c *Client) scanSortedSetEntries(
	ctx context.Context,
	key, match string,
	visit func(*SortedEntry) error,
) error {
	match = normalizeSortedSetMatch(match)

	var cursor uint64
	for {
		values, nextCursor, err := c.redis.ZScan(ctx, key, cursor, match, sortedSetScanCount).Result()
		if err != nil {
			return err
		}

		for i := 0; i+1 < len(values); i += 2 {
			score, err := strconv.ParseFloat(values[i+1], 64)
			if err != nil {
				continue
			}
			if err := visit(NewSortedEntry(values[i], score)); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

func sortedEntryBefore(a, b *SortedEntry, reverse bool) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	if reverse {
		return a.Score > b.Score
	}
	return a.Score < b.Score
}

func sortSortedEntries(entries []*SortedEntry, reverse bool) {
	sort.Slice(entries, func(i, j int) bool {
		return sortedEntryBefore(entries[i], entries[j], reverse)
	})
}

func insertSortedEntry(entries []*SortedEntry, entry *SortedEntry, reverse bool, limit int) []*SortedEntry {
	if limit == 0 {
		return entries
	}

	pos := sort.Search(len(entries), func(i int) bool {
		return sortedEntryBefore(entry, entries[i], reverse)
	})
	if limit > 0 && len(entries) == limit && pos == len(entries) {
		return entries
	}

	entries = append(entries, nil)
	copy(entries[pos+1:], entries[pos:])
	entries[pos] = entry
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}
