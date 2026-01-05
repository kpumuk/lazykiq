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

	// GetDeadJobs fetches dead jobs with pagination (newest first).
	GetDeadJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error)

	// ScanDeadJobs scans dead jobs using a match pattern (no paging).
	ScanDeadJobs(ctx context.Context, match string) ([]*SortedEntry, error)

	// GetDeadBounds fetches the oldest and newest dead jobs.
	GetDeadBounds(ctx context.Context) (*SortedEntry, *SortedEntry, error)

	// GetRetryJobs fetches retry jobs with pagination (earliest retry first).
	GetRetryJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error)

	// ScanRetryJobs scans retry jobs using a match pattern (no paging).
	ScanRetryJobs(ctx context.Context, match string) ([]*SortedEntry, error)

	// GetRetryBounds fetches the earliest and latest retry jobs.
	GetRetryBounds(ctx context.Context) (*SortedEntry, *SortedEntry, error)

	// GetScheduledJobs fetches scheduled jobs with pagination (earliest execution time first).
	GetScheduledJobs(ctx context.Context, start, count int) ([]*SortedEntry, int64, error)

	// ScanScheduledJobs scans scheduled jobs using a match pattern (no paging).
	ScanScheduledJobs(ctx context.Context, match string) ([]*SortedEntry, error)

	// GetScheduledBounds fetches the earliest and latest scheduled jobs.
	GetScheduledBounds(ctx context.Context) (*SortedEntry, *SortedEntry, error)
}

// Ensure Client implements API at compile time.
var _ API = (*Client)(nil)
