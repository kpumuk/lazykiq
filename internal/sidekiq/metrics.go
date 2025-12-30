package sidekiq

import (
	"context"
	"errors"
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

	keys := make([]string, 0, count*2)
	cursor := now
	for range count {
		keys = append(keys, metricsRollupKeys(cursor, granularity)...)
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
	if err != nil && !errors.Is(err, redis.Nil) {
		return result, err
	}

	for _, cmd := range cmds {
		if cmd.Err() != nil && !errors.Is(cmd.Err(), redis.Nil) {
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

	type metricsBucketCmd struct {
		at         time.Time
		rollupKeys []string
		histKeys   []string
		hmCmds     []*redis.SliceCmd
		histCmds   []*redis.IntSliceCmd
	}

	bucketCmds := make([]metricsBucketCmd, 0, count)
	cursor := now
	for range count {
		rollupKeys := metricsRollupKeys(cursor, granularity)
		histKeys := make([]string, 0, 2)
		if granularity == MetricsGranularityMinutely {
			histKeys = metricsHistogramKeys(className, cursor)
		}
		bucketCmds = append(bucketCmds, metricsBucketCmd{
			at:         cursor,
			rollupKeys: rollupKeys,
			histKeys:   histKeys,
			hmCmds:     make([]*redis.SliceCmd, 0, len(rollupKeys)),
			histCmds:   make([]*redis.IntSliceCmd, 0, len(histKeys)),
		})
		cursor = cursor.Add(-stride)
	}
	result.StartsAt = cursor.Add(stride)

	pipe := c.redis.Pipeline()
	var histFetchArgs []any
	if granularity == MetricsGranularityMinutely {
		histFetchArgs = metricsHistogramFetchArgs()
	}
	for i, cmd := range bucketCmds {
		for _, key := range cmd.rollupKeys {
			cmd.hmCmds = append(cmd.hmCmds, pipe.HMGet(ctx, key, className+"|ms", className+"|p", className+"|f"))
		}
		if granularity == MetricsGranularityMinutely {
			for _, key := range cmd.histKeys {
				cmd.histCmds = append(cmd.histCmds, pipe.BitFieldRO(ctx, key, histFetchArgs...))
			}
		}
		bucketCmds[i] = cmd
	}

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return result, err
	}

	for _, bucket := range bucketCmds {
		var msTotal int64
		var pTotal int64
		var fTotal int64
		for _, cmd := range bucket.hmCmds {
			if cmd.Err() != nil && !errors.Is(cmd.Err(), redis.Nil) {
				return result, cmd.Err()
			}
			values := cmd.Val()
			if ms, ok := parseMetricsValue(values, 0); ok {
				msTotal += ms
			}
			if p, ok := parseMetricsValue(values, 1); ok {
				pTotal += p
			}
			if f, ok := parseMetricsValue(values, 2); ok {
				fTotal += f
			}
		}

		result.Totals.Milliseconds += msTotal
		result.Totals.Seconds += float64(msTotal) / 1000.0
		result.Totals.Processed += pTotal
		result.Totals.Failed += fTotal

		bucketTime := metricsBucketTime(bucket.at, granularity)
		if granularity == MetricsGranularityMinutely {
			var merged []int64
			histFound := false
			for _, histCmd := range bucket.histCmds {
				if histCmd.Err() != nil && !errors.Is(histCmd.Err(), redis.Nil) {
					return result, histCmd.Err()
				}
				hist := histCmd.Val()
				if len(hist) == 0 {
					continue
				}
				if merged == nil {
					merged = make([]int64, len(hist))
				}
				for i, value := range hist {
					merged[i] += value
				}
				histFound = true
			}
			if histFound {
				reversed := slices.Clone(merged)
				slices.Reverse(reversed)
				result.Hist[bucketTime] = reversed
			}
		}

		result.Buckets = append(result.Buckets, bucket.at)
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

func metricsRollupKeySidekiq8(t time.Time, granularity MetricsGranularity) string {
	t = t.UTC()
	date := t.Format("060102")
	hour := t.Hour()
	minute := t.Minute()
	if granularity == MetricsGranularityHourly {
		minute /= 10
		return fmt.Sprintf("j|%s|%d:%d", date, hour, minute)
	}
	return fmt.Sprintf("j|%s|%d:%02d", date, hour, minute)
}

func metricsRollupKeys(t time.Time, granularity MetricsGranularity) []string {
	keys := []string{
		metricsRollupKeySidekiq8(t, granularity),
	}
	if sidekiq7Key := metricsRollupKeySidekiq7(t, granularity); sidekiq7Key != "" && sidekiq7Key != keys[0] {
		keys = append(keys, sidekiq7Key)
	}
	return keys
}

func metricsRollupKeySidekiq7(t time.Time, granularity MetricsGranularity) string {
	// Sidekiq 7 only writes minute buckets (no 10-minute rollups).
	if granularity == MetricsGranularityHourly {
		return ""
	}
	t = t.UTC()
	date := t.Format("20060102")
	return fmt.Sprintf("j|%s|%d:%d", date, t.Hour(), t.Minute())
}

func metricsHistogramKeySidekiq8(className string, t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("h|%s-%d-%d:%d", className, t.Day(), t.Hour(), t.Minute())
}

func metricsHistogramKeys(className string, t time.Time) []string {
	keys := []string{
		metricsHistogramKeySidekiq8(className, t),
	}
	if sidekiq7Key := metricsHistogramKeySidekiq7(className, t); sidekiq7Key != keys[0] {
		keys = append(keys, sidekiq7Key)
	}
	return keys
}

func metricsHistogramKeySidekiq7(className string, t time.Time) string {
	// Sidekiq 7 stores histograms without the "h|" prefix and zero-pads day/hour.
	t = t.UTC()
	return fmt.Sprintf("%s-%02d-%02d:%d", className, t.Day(), t.Hour(), t.Minute())
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
