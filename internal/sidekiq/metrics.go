package sidekiq

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// MetricsGranularity defines the rollup granularity for metrics queries.
type MetricsGranularity int

const (
	// MetricsGranularityMinutely uses per-minute buckets.
	MetricsGranularityMinutely MetricsGranularity = iota
	// MetricsGranularityHourly uses 10-minute buckets.
	MetricsGranularityHourly
)

// MetricsPeriod defines a query period in minutes or hours.
type MetricsPeriod struct {
	Minutes int
	Hours   int
}

// MetricsPeriods matches Sidekiq's supported metric periods.
var MetricsPeriods = map[string]MetricsPeriod{
	"1h":  {Minutes: 60},
	"2h":  {Minutes: 120},
	"4h":  {Minutes: 240},
	"8h":  {Minutes: 480},
	"24h": {Hours: 24},
	"48h": {Hours: 48},
	"72h": {Hours: 72},
}

// MetricsPeriodOrder defines the display order for periods.
var MetricsPeriodOrder = []string{"1h", "2h", "4h", "8h", "24h", "48h", "72h"}

const metricsHistogramBuckets = 26

// MetricsHistogramLabels defines the histogram bucket labels from Sidekiq.
var MetricsHistogramLabels = []string{
	"20ms", "30ms", "45ms", "65ms", "100ms",
	"150ms", "225ms", "335ms", "500ms", "750ms",
	"1.1s", "1.7s", "2.5s", "3.8s", "5.75s",
	"8.5s", "13s", "20s", "30s", "45s",
	"65s", "100s", "150s", "225s", "335s",
	"∞",
}

// MetricsJobTotals holds aggregated metrics for a job.
type MetricsJobTotals struct {
	Processed    int64
	Failed       int64
	Milliseconds int64
	Seconds      float64
}

// Success returns the count of successful jobs.
func (t MetricsJobTotals) Success() int64 {
	success := t.Processed - t.Failed
	if success < 0 {
		return 0
	}
	return success
}

// AvgSeconds returns the average execution time in seconds.
func (t MetricsJobTotals) AvgSeconds() float64 {
	completed := t.Success()
	if completed == 0 {
		return 0
	}
	return t.Seconds / float64(completed)
}

// MetricsTopJobsResult contains aggregated metrics for multiple jobs.
type MetricsTopJobsResult struct {
	Granularity MetricsGranularity
	StartsAt    time.Time
	EndsAt      time.Time
	Jobs        map[string]MetricsJobTotals
}

// MetricsJobDetailResult contains metrics for a single job.
type MetricsJobDetailResult struct {
	Granularity MetricsGranularity
	StartsAt    time.Time
	EndsAt      time.Time
	Buckets     []time.Time
	Totals      MetricsJobTotals
	Hist        map[string][]int64
}

// GetMetricsTopJobs fetches aggregated metrics for all jobs within the period.
func (c *Client) GetMetricsTopJobs(ctx context.Context, period MetricsPeriod, classFilter string) (MetricsTopJobsResult, error) {
	granularity, count, stride := metricsRollup(period)
	now := time.Now().UTC()
	result := MetricsTopJobsResult{
		Granularity: granularity,
		EndsAt:      now,
		Jobs:        make(map[string]MetricsJobTotals),
	}

	if count == 0 {
		result.StartsAt = now
		return result, nil
	}

	keys := make([]string, 0, count)
	cursor := now
	for range count {
		keys = append(keys, metricsRollupKey(cursor, granularity))
		cursor = cursor.Add(-stride)
	}
	result.StartsAt = cursor.Add(stride)

	var filter string
	if classFilter != "" {
		filter = strings.ToLower(classFilter)
	}

	pipe := c.redis.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, 0, len(keys))
	for _, key := range keys {
		cmds = append(cmds, pipe.HGetAll(ctx, key))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return result, err
	}

	for _, cmd := range cmds {
		if cmd.Err() != nil && cmd.Err() != redis.Nil {
			return result, cmd.Err()
		}
		for key, value := range cmd.Val() {
			className, metric := splitMetricKey(key)
			if className == "" {
				continue
			}
			if filter != "" && !strings.Contains(strings.ToLower(className), filter) {
				continue
			}
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				continue
			}

			totals := result.Jobs[className]
			switch metric {
			case "ms":
				totals.Milliseconds += parsed
				totals.Seconds += float64(parsed) / 1000.0
			case "p":
				totals.Processed += parsed
			case "f":
				totals.Failed += parsed
			}
			result.Jobs[className] = totals
		}
	}

	return result, nil
}

// GetMetricsJobDetail fetches detailed metrics for a single job within the period.
func (c *Client) GetMetricsJobDetail(ctx context.Context, className string, period MetricsPeriod) (MetricsJobDetailResult, error) {
	granularity, count, stride := metricsRollup(period)
	now := time.Now().UTC()
	result := MetricsJobDetailResult{
		Granularity: granularity,
		EndsAt:      now,
		Hist:        make(map[string][]int64),
	}

	if count == 0 {
		result.StartsAt = now
		return result, nil
	}

	keys := make([]string, 0, count)
	histKeys := make([]string, 0, count)
	cursor := now
	for range count {
		keys = append(keys, metricsRollupKey(cursor, granularity))
		if granularity == MetricsGranularityMinutely {
			histKeys = append(histKeys, metricsHistogramKey(className, cursor))
		}
		cursor = cursor.Add(-stride)
	}
	result.StartsAt = cursor.Add(stride)

	pipe := c.redis.Pipeline()
	hmCmds := make([]*redis.SliceCmd, 0, len(keys))
	var histCmds []*redis.IntSliceCmd
	var histFetchArgs []any
	if granularity == MetricsGranularityMinutely {
		histCmds = make([]*redis.IntSliceCmd, 0, len(keys))
		histFetchArgs = metricsHistogramFetchArgs()
	}

	for i, key := range keys {
		hmCmds = append(hmCmds, pipe.HMGet(ctx, key, className+"|ms", className+"|p", className+"|f"))
		if granularity == MetricsGranularityMinutely {
			histCmds = append(histCmds, pipe.BitFieldRO(ctx, histKeys[i], histFetchArgs...))
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return result, err
	}

	cursor = now
	for i, cmd := range hmCmds {
		if cmd.Err() != nil && cmd.Err() != redis.Nil {
			return result, cmd.Err()
		}
		values := cmd.Val()
		ms, ok := parseMetricsValue(values, 0)
		if ok {
			result.Totals.Milliseconds += ms
			result.Totals.Seconds += float64(ms) / 1000.0
		}
		if p, ok := parseMetricsValue(values, 1); ok {
			result.Totals.Processed += p
		}
		if f, ok := parseMetricsValue(values, 2); ok {
			result.Totals.Failed += f
		}

		bucketTime := metricsBucketTime(cursor, granularity)
		if granularity == MetricsGranularityMinutely {
			histCmd := histCmds[i]
			if histCmd.Err() != nil && histCmd.Err() != redis.Nil {
				return result, histCmd.Err()
			}
			hist := histCmd.Val()
			if len(hist) > 0 {
				reversed := slices.Clone(hist)
				slices.Reverse(reversed)
				result.Hist[bucketTime] = reversed
			}
		}

		result.Buckets = append(result.Buckets, cursor)
		cursor = cursor.Add(-stride)
	}

	return result, nil
}

func metricsRollup(period MetricsPeriod) (MetricsGranularity, int, time.Duration) {
	if period.Hours > 0 {
		hours := min(period.Hours, 72)
		return MetricsGranularityHourly, hours * 6, 10 * time.Minute
	}

	minutes := period.Minutes
	if minutes == 0 {
		minutes = 60
	}
	minutes = min(minutes, 480)
	return MetricsGranularityMinutely, minutes, time.Minute
}

func metricsRollupKey(t time.Time, granularity MetricsGranularity) string {
	t = t.UTC()
	date := t.Format("060102")
	hour := t.Hour()
	minute := t.Minute()
	if granularity == MetricsGranularityHourly {
		minute = minute / 10
		return fmt.Sprintf("j|%s|%d:%d", date, hour, minute)
	}
	return fmt.Sprintf("j|%s|%d:%02d", date, hour, minute)
}

func metricsHistogramKey(className string, t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("h|%s-%d-%d:%d", className, t.Day(), t.Hour(), t.Minute())
}

func metricsBucketTime(t time.Time, granularity MetricsGranularity) string {
	truncation := time.Minute
	if granularity == MetricsGranularityHourly {
		truncation = 10 * time.Minute
	}
	return t.UTC().Truncate(truncation).Format(time.RFC3339)
}

func metricsHistogramFetchArgs() []any {
	args := make([]any, 0, metricsHistogramBuckets*2)
	for i := range metricsHistogramBuckets {
		args = append(args, "u16", fmt.Sprintf("#%d", i))
	}
	return args
}

func splitMetricKey(value string) (string, string) {
	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func parseMetricsValue(values []any, index int) (int64, bool) {
	if index >= len(values) {
		return 0, false
	}
	value := values[index]
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case []byte:
		if len(v) == 0 {
			return 0, false
		}
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case int64:
		return v, true
	case nil:
		return 0, false
	default:
		return 0, false
	}
}
