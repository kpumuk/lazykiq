package sidekiq

import "context"

// API defines the interface for interacting with Sidekiq via Redis.
// This interface enables mocking the client for testing purposes.
type API interface {
	// Close closes the Redis connection.
	Close() error

	// DisplayRedisURL returns a sanitized URL safe for display.
	DisplayRedisURL() string

	// DetectVersion detects which Sidekiq version is being used based on key format.
	DetectVersion(ctx context.Context) Version

	// MetricsPeriodOrder returns the appropriate period order based on detected Sidekiq version.
	MetricsPeriodOrder(ctx context.Context) []string

	// GetStats fetches current Sidekiq statistics from Redis.
	GetStats(ctx context.Context) (Stats, error)

	// GetRedisInfo fetches Redis INFO and extracts fields used on the dashboard.
	GetRedisInfo(ctx context.Context) (RedisInfo, error)

	// GetStatsHistory fetches per-day processed and failed stats for the last N days.
	GetStatsHistory(ctx context.Context, days int) (StatsHistory, error)

	// GetMetricsTopJobs fetches aggregated metrics for all jobs within the period.
	GetMetricsTopJobs(ctx context.Context, period MetricsPeriod, classFilter string) (MetricsTopJobsResult, error)

	// GetMetricsJobDetail fetches detailed metrics for a single job within the period.
	GetMetricsJobDetail(ctx context.Context, className string, period MetricsPeriod) (MetricsJobDetailResult, error)

	// NewQueue creates a new Queue instance for the given queue name.
	NewQueue(name string) *Queue

	// GetQueues fetches all known queues from Redis, sorted alphabetically.
	GetQueues(ctx context.Context) ([]*Queue, error)

	// NewProcess creates a new Process instance for the given identity.
	NewProcess(identity string) *Process

	// GetProcesses fetches all process identities from Redis, sorted alphabetically.
	GetProcesses(ctx context.Context) ([]*Process, error)

	// GetBusyData fetches detailed process and active job information from Redis.
	// If filter is non-empty, only jobs whose raw payload contains the substring are returned.
	GetBusyData(ctx context.Context, filter string) (BusyData, error)

	// GetSortedEntries fetches sorted-set jobs with pagination.
	GetSortedEntries(ctx context.Context, kind SortedSetKind, start, count int) ([]*SortedEntry, int64, error)

	// ScanSortedEntries scans sorted-set jobs using a match pattern (no paging).
	ScanSortedEntries(ctx context.Context, kind SortedSetKind, match string) ([]*SortedEntry, error)

	// ScanSortedEntriesWindow scans sorted-set jobs using a match pattern and returns one window.
	ScanSortedEntriesWindow(ctx context.Context, kind SortedSetKind, match string, start, count int) (SortedEntriesWindow, error)

	// GetSortedEntryBounds fetches the oldest and newest entries for a sorted set.
	GetSortedEntryBounds(ctx context.Context, kind SortedSetKind) (*SortedEntry, *SortedEntry, error)

	// GetErrorSummary fetches exact error summary rows across dead and retry sets.
	GetErrorSummary(ctx context.Context, query string) ([]ErrorSummaryRow, ErrorSummaryMeta, error)

	// GetErrorGroupWindow fetches one exact paged error group window across dead and retry sets.
	GetErrorGroupWindow(ctx context.Context, key ErrorGroupKey, query string, start, count int) (ErrorGroupWindow, error)

	// DeleteSortedEntry removes a job from a sorted set.
	DeleteSortedEntry(ctx context.Context, kind SortedSetKind, entry *SortedEntry) error

	// DeleteAllSortedEntries removes all jobs from a sorted set.
	DeleteAllSortedEntries(ctx context.Context, kind SortedSetKind) error

	// EnqueueSortedEntry moves a sorted-set job to its queue immediately.
	EnqueueSortedEntry(ctx context.Context, kind SortedSetKind, entry *SortedEntry) error

	// EnqueueAllSortedEntries moves all sorted-set jobs to their queues immediately.
	EnqueueAllSortedEntries(ctx context.Context, kind SortedSetKind) error

	// MoveSortedEntryToDead moves a supported sorted-set job to the dead set.
	MoveSortedEntryToDead(ctx context.Context, kind SortedSetKind, entry *SortedEntry) error

	// MoveAllSortedEntriesToDead moves all supported sorted-set jobs to the dead set.
	MoveAllSortedEntriesToDead(ctx context.Context, kind SortedSetKind) error
}

// Ensure Client implements API at compile time.
var _ API = (*Client)(nil)
