package sidekiq

import (
	"testing"
	"time"
)

// MetricsJobTotals tests

func TestMetricsJobTotals_Success(t *testing.T) {
	tests := []struct {
		name      string
		totals    MetricsJobTotals
		wantValue int64
	}{
		{
			name:      "all successful",
			totals:    MetricsJobTotals{Processed: 100, Failed: 0},
			wantValue: 100,
		},
		{
			name:      "some failures",
			totals:    MetricsJobTotals{Processed: 100, Failed: 25},
			wantValue: 75,
		},
		{
			name:      "negative success clamped to zero",
			totals:    MetricsJobTotals{Processed: 10, Failed: 20},
			wantValue: 0,
		},
		{
			name:      "zero processed",
			totals:    MetricsJobTotals{Processed: 0, Failed: 0},
			wantValue: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.totals.Success()
			if got != tt.wantValue {
				t.Errorf("Success() = %d, want %d", got, tt.wantValue)
			}
		})
	}
}

func TestMetricsJobTotals_AvgSeconds(t *testing.T) {
	tests := []struct {
		name      string
		totals    MetricsJobTotals
		wantValue float64
	}{
		{
			name:      "normal average",
			totals:    MetricsJobTotals{Processed: 100, Failed: 0, Seconds: 50.0},
			wantValue: 0.5,
		},
		{
			name:      "with failures",
			totals:    MetricsJobTotals{Processed: 100, Failed: 25, Seconds: 75.0},
			wantValue: 1.0, // 75 seconds / 75 successful jobs
		},
		{
			name:      "zero successful jobs",
			totals:    MetricsJobTotals{Processed: 10, Failed: 10, Seconds: 100.0},
			wantValue: 0.0,
		},
		{
			name:      "zero processed",
			totals:    MetricsJobTotals{Processed: 0, Failed: 0, Seconds: 0},
			wantValue: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.totals.AvgSeconds()
			if got != tt.wantValue {
				t.Errorf("AvgSeconds() = %f, want %f", got, tt.wantValue)
			}
		})
	}
}

// Helper function tests

func TestMetricsRollup(t *testing.T) {
	tests := []struct {
		name            string
		period          MetricsPeriod
		wantGranularity MetricsGranularity
		wantCount       int
		wantStride      time.Duration
	}{
		{
			name:            "1 hour (60 minutes)",
			period:          MetricsPeriod{Minutes: 60},
			wantGranularity: MetricsGranularityMinutely,
			wantCount:       60,
			wantStride:      time.Minute,
		},
		{
			name:            "2 hours (120 minutes)",
			period:          MetricsPeriod{Minutes: 120},
			wantGranularity: MetricsGranularityMinutely,
			wantCount:       120,
			wantStride:      time.Minute,
		},
		{
			name:            "8 hours (480 minutes max)",
			period:          MetricsPeriod{Minutes: 480},
			wantGranularity: MetricsGranularityMinutely,
			wantCount:       480,
			wantStride:      time.Minute,
		},
		{
			name:            "10 hours clamped to 480 minutes",
			period:          MetricsPeriod{Minutes: 600},
			wantGranularity: MetricsGranularityMinutely,
			wantCount:       480,
			wantStride:      time.Minute,
		},
		{
			name:            "24 hours (hourly granularity)",
			period:          MetricsPeriod{Hours: 24},
			wantGranularity: MetricsGranularityHourly,
			wantCount:       144, // 24 * 6 (10-minute buckets)
			wantStride:      10 * time.Minute,
		},
		{
			name:            "48 hours",
			period:          MetricsPeriod{Hours: 48},
			wantGranularity: MetricsGranularityHourly,
			wantCount:       288, // 48 * 6
			wantStride:      10 * time.Minute,
		},
		{
			name:            "72 hours",
			period:          MetricsPeriod{Hours: 72},
			wantGranularity: MetricsGranularityHourly,
			wantCount:       432, // 72 * 6
			wantStride:      10 * time.Minute,
		},
		{
			name:            "100 hours clamped to 72",
			period:          MetricsPeriod{Hours: 100},
			wantGranularity: MetricsGranularityHourly,
			wantCount:       432, // 72 * 6
			wantStride:      10 * time.Minute,
		},
		{
			name:            "zero defaults to 60 minutes",
			period:          MetricsPeriod{},
			wantGranularity: MetricsGranularityMinutely,
			wantCount:       60,
			wantStride:      time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gran, count, stride := metricsRollup(tt.period)

			if gran != tt.wantGranularity {
				t.Errorf("granularity = %v, want %v", gran, tt.wantGranularity)
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
			if stride != tt.wantStride {
				t.Errorf("stride = %v, want %v", stride, tt.wantStride)
			}
		})
	}
}

func TestMetricsRollupKeySidekiq8(t *testing.T) {
	tests := []struct {
		name        string
		time        time.Time
		granularity MetricsGranularity
		wantKey     string
	}{
		{
			name:        "minutely single digit hour",
			time:        time.Date(2022, 7, 22, 9, 3, 0, 0, time.UTC),
			granularity: MetricsGranularityMinutely,
			wantKey:     "j|220722|9:03",
		},
		{
			name:        "minutely double digit hour",
			time:        time.Date(2022, 7, 22, 22, 3, 0, 0, time.UTC),
			granularity: MetricsGranularityMinutely,
			wantKey:     "j|220722|22:03",
		},
		{
			name:        "hourly 10-minute bucket 0",
			time:        time.Date(2022, 7, 22, 22, 3, 0, 0, time.UTC),
			granularity: MetricsGranularityHourly,
			wantKey:     "j|220722|22:0",
		},
		{
			name:        "hourly 10-minute bucket 5",
			time:        time.Date(2022, 7, 22, 22, 55, 0, 0, time.UTC),
			granularity: MetricsGranularityHourly,
			wantKey:     "j|220722|22:5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricsRollupKeySidekiq8(tt.time, tt.granularity)
			if got != tt.wantKey {
				t.Errorf("metricsRollupKeySidekiq8() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestMetricsRollupKeySidekiq7(t *testing.T) {
	tests := []struct {
		name        string
		time        time.Time
		granularity MetricsGranularity
		wantKey     string
	}{
		{
			name:        "minutely",
			time:        time.Date(2022, 7, 22, 22, 3, 0, 0, time.UTC),
			granularity: MetricsGranularityMinutely,
			wantKey:     "j|20220722|22:3",
		},
		{
			name:        "hourly returns empty (not supported)",
			time:        time.Date(2022, 7, 22, 22, 3, 0, 0, time.UTC),
			granularity: MetricsGranularityHourly,
			wantKey:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricsRollupKeySidekiq7(tt.time, tt.granularity)
			if got != tt.wantKey {
				t.Errorf("metricsRollupKeySidekiq7() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestMetricsRollupKeyForVersion(t *testing.T) {
	fixedTime := time.Date(2022, 7, 22, 22, 3, 0, 0, time.UTC)

	tests := []struct {
		name        string
		version     Version
		granularity MetricsGranularity
		wantKey     string
	}{
		{
			name:        "sidekiq 7 minutely",
			version:     Version7,
			granularity: MetricsGranularityMinutely,
			wantKey:     "j|20220722|22:3",
		},
		{
			name:        "sidekiq 7 hourly (empty)",
			version:     Version7,
			granularity: MetricsGranularityHourly,
			wantKey:     "",
		},
		{
			name:        "sidekiq 8 minutely",
			version:     Version8,
			granularity: MetricsGranularityMinutely,
			wantKey:     "j|220722|22:03",
		},
		{
			name:        "sidekiq 8 hourly",
			version:     Version8,
			granularity: MetricsGranularityHourly,
			wantKey:     "j|220722|22:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricsRollupKeyForVersion(fixedTime, tt.granularity, tt.version)
			if got != tt.wantKey {
				t.Errorf("metricsRollupKeyForVersion() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestMetricsHistogramKeyForVersion(t *testing.T) {
	fixedTime := time.Date(2022, 7, 22, 22, 3, 0, 0, time.UTC)

	tests := []struct {
		name      string
		className string
		version   Version
		wantKey   string
	}{
		{
			name:      "sidekiq 7",
			className: "App::FooJob",
			version:   Version7,
			wantKey:   "App::FooJob-22-22:3",
		},
		{
			name:      "sidekiq 8",
			className: "App::FooJob",
			version:   Version8,
			wantKey:   "h|App::FooJob-22-22:3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricsHistogramKeyForVersion(tt.className, fixedTime, tt.version)
			if got != tt.wantKey {
				t.Errorf("metricsHistogramKeyForVersion() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestMetricsBucketTime(t *testing.T) {
	tests := []struct {
		name        string
		time        time.Time
		granularity MetricsGranularity
		wantTime    string
	}{
		{
			name:        "minutely truncates to minute",
			time:        time.Date(2022, 7, 22, 22, 3, 45, 123, time.UTC),
			granularity: MetricsGranularityMinutely,
			wantTime:    "2022-07-22T22:03:00Z",
		},
		{
			name:        "hourly truncates to 10 minutes",
			time:        time.Date(2022, 7, 22, 22, 3, 45, 123, time.UTC),
			granularity: MetricsGranularityHourly,
			wantTime:    "2022-07-22T22:00:00Z",
		},
		{
			name:        "hourly at 15 minutes",
			time:        time.Date(2022, 7, 22, 22, 15, 45, 123, time.UTC),
			granularity: MetricsGranularityHourly,
			wantTime:    "2022-07-22T22:10:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricsBucketTime(tt.time, tt.granularity)
			if got != tt.wantTime {
				t.Errorf("metricsBucketTime() = %q, want %q", got, tt.wantTime)
			}
		})
	}
}

func TestSplitMetricKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		wantClassName string
		wantMetric    string
	}{
		{
			name:          "ms metric",
			key:           "App::FooJob|ms",
			wantClassName: "App::FooJob",
			wantMetric:    "ms",
		},
		{
			name:          "p metric",
			key:           "App::FooJob|p",
			wantClassName: "App::FooJob",
			wantMetric:    "p",
		},
		{
			name:          "f metric",
			key:           "App::FooJob|f",
			wantClassName: "App::FooJob",
			wantMetric:    "f",
		},
		{
			name:          "no pipe separator",
			key:           "App::FooJob",
			wantClassName: "",
			wantMetric:    "",
		},
		{
			name:          "multiple pipes (takes first)",
			key:           "App::FooJob|ms|extra",
			wantClassName: "App::FooJob",
			wantMetric:    "ms|extra",
		},
		{
			name:          "empty string",
			key:           "",
			wantClassName: "",
			wantMetric:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			className, metric := splitMetricKey(tt.key)
			if className != tt.wantClassName {
				t.Errorf("className = %q, want %q", className, tt.wantClassName)
			}
			if metric != tt.wantMetric {
				t.Errorf("metric = %q, want %q", metric, tt.wantMetric)
			}
		})
	}
}

func TestParseMetricsValue(t *testing.T) {
	tests := []struct {
		name      string
		values    []any
		index     int
		wantValue int64
		wantOk    bool
	}{
		{
			name:      "string number",
			values:    []any{"123"},
			index:     0,
			wantValue: 123,
			wantOk:    true,
		},
		{
			name:      "empty string",
			values:    []any{""},
			index:     0,
			wantValue: 0,
			wantOk:    false,
		},
		{
			name:      "bytes number",
			values:    []any{[]byte("456")},
			index:     0,
			wantValue: 456,
			wantOk:    true,
		},
		{
			name:      "empty bytes",
			values:    []any{[]byte{}},
			index:     0,
			wantValue: 0,
			wantOk:    false,
		},
		{
			name:      "int64",
			values:    []any{int64(789)},
			index:     0,
			wantValue: 789,
			wantOk:    true,
		},
		{
			name:      "nil value",
			values:    []any{nil},
			index:     0,
			wantValue: 0,
			wantOk:    false,
		},
		{
			name:      "index out of bounds",
			values:    []any{"123"},
			index:     5,
			wantValue: 0,
			wantOk:    false,
		},
		{
			name:      "invalid string",
			values:    []any{"not a number"},
			index:     0,
			wantValue: 0,
			wantOk:    false,
		},
		{
			name:      "invalid bytes",
			values:    []any{[]byte("not a number")},
			index:     0,
			wantValue: 0,
			wantOk:    false,
		},
		{
			name:      "unknown type",
			values:    []any{123.45},
			index:     0,
			wantValue: 0,
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := parseMetricsValue(tt.values, tt.index)
			if value != tt.wantValue {
				t.Errorf("value = %d, want %d", value, tt.wantValue)
			}
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

// Integration tests

func TestGetMetricsTopJobs_Empty(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := testContext(t)

	result, err := client.GetMetricsTopJobs(ctx, MetricsPeriod{Minutes: 60}, "")
	if err != nil {
		t.Fatalf("GetMetricsTopJobs failed: %v", err)
	}

	if result.Granularity != MetricsGranularityMinutely {
		t.Errorf("Granularity = %v, want MetricsGranularityMinutely", result.Granularity)
	}

	if len(result.Jobs) != 0 {
		t.Errorf("len(Jobs) = %d, want 0", len(result.Jobs))
	}
}

func TestGetMetricsTopJobs_WithData(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Use current time (rounded to minute) for key generation
	now := time.Now().UTC().Truncate(time.Minute)

	// Create Sidekiq 8 format keys for current minute
	key := metricsRollupKeySidekiq8(now, MetricsGranularityMinutely)

	// Add metrics for two jobs
	mr.HSet(key, "App::FooJob|ms", "1500")
	mr.HSet(key, "App::FooJob|p", "10")
	mr.HSet(key, "App::FooJob|f", "2")

	mr.HSet(key, "App::BarJob|ms", "3000")
	mr.HSet(key, "App::BarJob|p", "20")
	mr.HSet(key, "App::BarJob|f", "5")

	result, err := client.GetMetricsTopJobs(ctx, MetricsPeriod{Minutes: 1}, "")
	if err != nil {
		t.Fatalf("GetMetricsTopJobs failed: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Fatalf("len(Jobs) = %d, want 2", len(result.Jobs))
	}

	fooJob := result.Jobs["App::FooJob"]
	if fooJob.Milliseconds != 1500 {
		t.Errorf("FooJob Milliseconds = %d, want 1500", fooJob.Milliseconds)
	}
	if fooJob.Processed != 10 {
		t.Errorf("FooJob Processed = %d, want 10", fooJob.Processed)
	}
	if fooJob.Failed != 2 {
		t.Errorf("FooJob Failed = %d, want 2", fooJob.Failed)
	}
	if fooJob.Success() != 8 {
		t.Errorf("FooJob Success() = %d, want 8", fooJob.Success())
	}

	barJob := result.Jobs["App::BarJob"]
	if barJob.Processed != 20 {
		t.Errorf("BarJob Processed = %d, want 20", barJob.Processed)
	}
}

func TestGetMetricsTopJobs_WithFilter(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	now := time.Now().UTC().Truncate(time.Minute)
	key := metricsRollupKeySidekiq8(now, MetricsGranularityMinutely)

	// Add metrics for jobs with different names
	mr.HSet(key, "App::FooJob|p", "10")
	mr.HSet(key, "App::BarJob|p", "20")
	mr.HSet(key, "Other::BazJob|p", "30")

	// Filter for jobs containing "app" (case insensitive)
	result, err := client.GetMetricsTopJobs(ctx, MetricsPeriod{Minutes: 1}, "app")
	if err != nil {
		t.Fatalf("GetMetricsTopJobs failed: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Fatalf("len(Jobs) = %d, want 2 (filtered)", len(result.Jobs))
	}

	if _, ok := result.Jobs["App::FooJob"]; !ok {
		t.Error("App::FooJob not found in filtered results")
	}
	if _, ok := result.Jobs["App::BarJob"]; !ok {
		t.Error("App::BarJob not found in filtered results")
	}
	if _, ok := result.Jobs["Other::BazJob"]; ok {
		t.Error("Other::BazJob should be filtered out")
	}
}

func TestGetMetricsTopJobs_MultipleBuckets(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	now := time.Now().UTC().Truncate(time.Minute)

	// Add metrics for two buckets (current and previous minute)
	key1 := metricsRollupKeySidekiq8(now, MetricsGranularityMinutely)
	key2 := metricsRollupKeySidekiq8(now.Add(-time.Minute), MetricsGranularityMinutely)

	mr.HSet(key1, "App::FooJob|ms", "1000")
	mr.HSet(key1, "App::FooJob|p", "5")

	mr.HSet(key2, "App::FooJob|ms", "2000")
	mr.HSet(key2, "App::FooJob|p", "10")

	result, err := client.GetMetricsTopJobs(ctx, MetricsPeriod{Minutes: 2}, "")
	if err != nil {
		t.Fatalf("GetMetricsTopJobs failed: %v", err)
	}

	// Metrics should be aggregated across buckets
	fooJob := result.Jobs["App::FooJob"]
	if fooJob.Milliseconds != 3000 {
		t.Errorf("FooJob Milliseconds = %d, want 3000 (aggregated)", fooJob.Milliseconds)
	}
	if fooJob.Processed != 15 {
		t.Errorf("FooJob Processed = %d, want 15 (aggregated)", fooJob.Processed)
	}
}

func TestGetMetricsTopJobs_InvalidValues(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	now := time.Now().UTC().Truncate(time.Minute)
	key := metricsRollupKeySidekiq8(now, MetricsGranularityMinutely)

	// Add invalid metric values (should be skipped)
	mr.HSet(key, "App::FooJob|ms", "not a number")
	mr.HSet(key, "App::FooJob|p", "10")
	mr.HSet(key, "InvalidKey", "123") // No pipe separator

	result, err := client.GetMetricsTopJobs(ctx, MetricsPeriod{Minutes: 1}, "")
	if err != nil {
		t.Fatalf("GetMetricsTopJobs failed: %v", err)
	}

	fooJob := result.Jobs["App::FooJob"]
	if fooJob.Milliseconds != 0 {
		t.Errorf("FooJob Milliseconds = %d, want 0 (invalid value skipped)", fooJob.Milliseconds)
	}
	if fooJob.Processed != 10 {
		t.Errorf("FooJob Processed = %d, want 10", fooJob.Processed)
	}
}

func TestGetMetricsJobDetail_Sidekiq8_Minutely(t *testing.T) {
	t.Skip("Skipped: Minutely granularity uses BITFIELD_RO (histogram data) not supported by miniredis")
}

func TestGetMetricsJobDetail_Sidekiq8_Hourly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed Sidekiq 8 version key
	_ = mr.Set("j|250101|0:0", "1")

	now := time.Now().UTC().Truncate(10 * time.Minute)

	// Test with 24-hour period (10-minute buckets)
	key := metricsRollupKeySidekiq8(now, MetricsGranularityHourly)

	mr.HSet(key, "App::FooJob|ms", "5000")
	mr.HSet(key, "App::FooJob|p", "50")
	mr.HSet(key, "App::FooJob|f", "5")

	result, err := client.GetMetricsJobDetail(ctx, "App::FooJob", MetricsPeriod{Hours: 24})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	if result.Granularity != MetricsGranularityHourly {
		t.Errorf("Granularity = %v, want MetricsGranularityHourly", result.Granularity)
	}

	if result.Totals.Milliseconds != 5000 {
		t.Errorf("Totals.Milliseconds = %d, want 5000", result.Totals.Milliseconds)
	}

	// 24 hours * 6 buckets per hour = 144 buckets
	if len(result.Buckets) != 144 {
		t.Errorf("len(Buckets) = %d, want 144", len(result.Buckets))
	}
}

func TestGetMetricsJobDetail_Sidekiq7_Minutely(t *testing.T) {
	t.Skip("Skipped: Minutely granularity uses BITFIELD_RO (histogram data) not supported by miniredis")
}

func TestGetMetricsJobDetail_ZeroPeriod(t *testing.T) {
	t.Skip("Skipped: Empty period defaults to 60 minutes (minutely) which uses BITFIELD_RO not supported by miniredis")
}

func TestGetMetricsJobDetail_NoData_Hourly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed version key
	_ = mr.Set("j|250101|0:0", "1")

	// Request metrics for a job with no data (use hourly to avoid BITFIELD_RO)
	result, err := client.GetMetricsJobDetail(ctx, "App::NonExistentJob", MetricsPeriod{Hours: 24})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	if result.Totals.Milliseconds != 0 {
		t.Errorf("Totals.Milliseconds = %d, want 0", result.Totals.Milliseconds)
	}
	if result.Totals.Processed != 0 {
		t.Errorf("Totals.Processed = %d, want 0", result.Totals.Processed)
	}

	// Should have 144 buckets (24h * 6 per hour)
	if len(result.Buckets) != 144 {
		t.Errorf("len(Buckets) = %d, want 144", len(result.Buckets))
	}
}

func TestGetMetricsJobDetail_PartialData_Hourly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed Sidekiq 8 version key
	_ = mr.Set("j|250101|0:0", "1")

	now := time.Now().UTC().Truncate(10 * time.Minute)

	// Only add data for one bucket (use hourly to avoid BITFIELD_RO)
	key := metricsRollupKeySidekiq8(now.Add(-20*time.Minute), MetricsGranularityHourly)

	mr.HSet(key, "App::FooJob|ms", "1000")
	mr.HSet(key, "App::FooJob|p", "5")
	mr.HSet(key, "App::FooJob|f", "1")

	result, err := client.GetMetricsJobDetail(ctx, "App::FooJob", MetricsPeriod{Hours: 24})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	// Should only count data from the one bucket with data
	if result.Totals.Milliseconds != 1000 {
		t.Errorf("Totals.Milliseconds = %d, want 1000", result.Totals.Milliseconds)
	}
	if result.Totals.Processed != 5 {
		t.Errorf("Totals.Processed = %d, want 5", result.Totals.Processed)
	}
	if result.Totals.Failed != 1 {
		t.Errorf("Totals.Failed = %d, want 1", result.Totals.Failed)
	}
}

func TestGetMetricsJobDetail_InvalidValues_Hourly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed version key
	_ = mr.Set("j|250101|0:0", "1")

	now := time.Now().UTC().Truncate(10 * time.Minute)
	key := metricsRollupKeySidekiq8(now, MetricsGranularityHourly)

	// Add invalid metric values (non-numeric strings)
	mr.HSet(key, "App::FooJob|ms", "invalid")
	mr.HSet(key, "App::FooJob|p", "10")
	mr.HSet(key, "App::FooJob|f", "not a number")

	result, err := client.GetMetricsJobDetail(ctx, "App::FooJob", MetricsPeriod{Hours: 24})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	// Invalid values should be skipped (parsed as 0 by parseMetricsValue returning false)
	if result.Totals.Milliseconds != 0 {
		t.Errorf("Totals.Milliseconds = %d, want 0 (invalid value skipped)", result.Totals.Milliseconds)
	}
	if result.Totals.Processed != 10 {
		t.Errorf("Totals.Processed = %d, want 10", result.Totals.Processed)
	}
	if result.Totals.Failed != 0 {
		t.Errorf("Totals.Failed = %d, want 0 (invalid value skipped)", result.Totals.Failed)
	}
}

func TestGetMetricsJobDetail_UnknownVersion_Hourly(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := testContext(t)

	// No version keys seeded - should default to Version8
	// Use hourly period to avoid BITFIELD_RO
	result, err := client.GetMetricsJobDetail(ctx, "App::FooJob", MetricsPeriod{Hours: 24})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	// Should have 144 buckets (24h * 6 per hour)
	if len(result.Buckets) != 144 {
		t.Errorf("len(Buckets) = %d, want 144", len(result.Buckets))
	}

	if result.Granularity != MetricsGranularityHourly {
		t.Errorf("Granularity = %v, want MetricsGranularityHourly", result.Granularity)
	}
}

func TestGetMetricsJobDetail_Sidekiq7_Hourly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed Sidekiq 7 version key (no Sidekiq 8 keys)
	_ = mr.Set("j|20250101|0:0", "1")

	// Sidekiq 7 doesn't support hourly rollups, so the function should handle empty keys gracefully
	result, err := client.GetMetricsJobDetail(ctx, "App::BarJob", MetricsPeriod{Hours: 24})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	// Should have 144 buckets (24h * 6 per hour) even though no data
	if len(result.Buckets) != 144 {
		t.Errorf("len(Buckets) = %d, want 144", len(result.Buckets))
	}

	// All totals should be zero (Sidekiq 7 doesn't write hourly keys)
	if result.Totals.Milliseconds != 0 {
		t.Errorf("Totals.Milliseconds = %d, want 0", result.Totals.Milliseconds)
	}
	if result.Totals.Processed != 0 {
		t.Errorf("Totals.Processed = %d, want 0", result.Totals.Processed)
	}
}

func TestGetMetricsJobDetail_MultipleBuckets_Hourly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed Sidekiq 8 version key
	_ = mr.Set("j|250101|0:0", "1")

	now := time.Now().UTC().Truncate(10 * time.Minute)

	// Add data for 3 different 10-minute buckets
	for i := range 3 {
		offset := time.Duration(i) * 10 * time.Minute
		key := metricsRollupKeySidekiq8(now.Add(-offset), MetricsGranularityHourly)

		mr.HSet(key, "App::AggJob|ms", "1000")
		mr.HSet(key, "App::AggJob|p", "10")
		mr.HSet(key, "App::AggJob|f", "1")
	}

	result, err := client.GetMetricsJobDetail(ctx, "App::AggJob", MetricsPeriod{Hours: 24})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	// Should aggregate across all 3 buckets
	if result.Totals.Milliseconds != 3000 {
		t.Errorf("Totals.Milliseconds = %d, want 3000", result.Totals.Milliseconds)
	}
	if result.Totals.Processed != 30 {
		t.Errorf("Totals.Processed = %d, want 30", result.Totals.Processed)
	}
	if result.Totals.Failed != 3 {
		t.Errorf("Totals.Failed = %d, want 3", result.Totals.Failed)
	}
	if result.Totals.Seconds != 3.0 {
		t.Errorf("Totals.Seconds = %f, want 3.0", result.Totals.Seconds)
	}
}

func TestGetMetricsJobDetail_72HourMaxPeriod(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed version key
	_ = mr.Set("j|250101|0:0", "1")

	// Request 72-hour period (maximum)
	result, err := client.GetMetricsJobDetail(ctx, "App::FooJob", MetricsPeriod{Hours: 72})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	// 72 hours * 6 buckets per hour = 432 buckets
	if len(result.Buckets) != 432 {
		t.Errorf("len(Buckets) = %d, want 432", len(result.Buckets))
	}

	if result.Granularity != MetricsGranularityHourly {
		t.Errorf("Granularity = %v, want MetricsGranularityHourly", result.Granularity)
	}
}

func TestGetMetricsJobDetail_PeriodClamping(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed version key
	_ = mr.Set("j|250101|0:0", "1")

	// Request 100 hours (should be clamped to 72)
	result, err := client.GetMetricsJobDetail(ctx, "App::FooJob", MetricsPeriod{Hours: 100})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	// Should be clamped to 72 hours * 6 = 432 buckets
	if len(result.Buckets) != 432 {
		t.Errorf("len(Buckets) = %d, want 432 (clamped to 72h)", len(result.Buckets))
	}
}

func TestGetMetricsJobDetail_BucketTimeConsistency_Hourly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	// Seed version key
	_ = mr.Set("j|250101|0:0", "1")

	// Use hourly period (48 hours = 48*6 = 288 buckets) to avoid BITFIELD_RO
	result, err := client.GetMetricsJobDetail(ctx, "App::FooJob", MetricsPeriod{Hours: 48})
	if err != nil {
		t.Fatalf("GetMetricsJobDetail failed: %v", err)
	}

	if len(result.Buckets) != 288 {
		t.Fatalf("len(Buckets) = %d, want 288", len(result.Buckets))
	}

	// Verify buckets are in descending order (newest first), 10-minute intervals
	// Check that consecutive buckets are exactly 10 minutes apart
	for i := range min(10, len(result.Buckets)-1) {
		diff := result.Buckets[i].Sub(result.Buckets[i+1])
		if diff != 10*time.Minute {
			t.Errorf("Bucket[%d] - Bucket[%d] = %v, want 10m", i, i+1, diff)
		}
	}

	// Verify EndsAt is close to now
	now := time.Now().UTC()
	if result.EndsAt.Before(now.Add(-time.Second)) || result.EndsAt.After(now.Add(time.Second)) {
		t.Errorf("EndsAt = %v, want around %v", result.EndsAt, now)
	}

	// Verify StartsAt is approximately 48 hours before EndsAt
	expectedDuration := time.Duration(287) * 10 * time.Minute // 287 intervals of 10 minutes
	actualDuration := result.EndsAt.Sub(result.StartsAt)
	if actualDuration < expectedDuration-time.Second || actualDuration > expectedDuration+time.Second {
		t.Errorf("EndsAt - StartsAt = %v, want approximately %v", actualDuration, expectedDuration)
	}
}
