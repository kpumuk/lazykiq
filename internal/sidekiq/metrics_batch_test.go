package sidekiq

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type metricsPipelineHook struct {
	process func(context.Context, []redis.Cmder) error
}

func (h metricsPipelineHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h metricsPipelineHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }

func (h metricsPipelineHook) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return h.process
}

func metricsHookClient(t *testing.T, process func(context.Context, []redis.Cmder) error) *Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{})
	t.Cleanup(func() { _ = rdb.Close() })
	rdb.AddHook(metricsPipelineHook{process: process})
	return &Client{redis: rdb}
}

func TestGetMetricsTopJobs_BoundedBatches(t *testing.T) {
	for name, spec := range map[string]struct {
		period      MetricsPeriod
		buckets     int
		stride      time.Duration
		granularity MetricsGranularity
		version     Version
	}{
		"Sidekiq 7":          {MetricsPeriod{Minutes: 60}, 60, time.Minute, MetricsGranularityMinutely, Version7},
		"Sidekiq 8":          {MetricsPeriod{Minutes: 60}, 60, time.Minute, MetricsGranularityMinutely, Version8},
		"mixed formats":      {MetricsPeriod{Minutes: 480}, 480, time.Minute, MetricsGranularityMinutely, VersionUnknown},
		"partial last batch": {MetricsPeriod{Minutes: 9}, 9, time.Minute, MetricsGranularityMinutely, Version8},
		"clamped minutes":    {MetricsPeriod{Minutes: 600}, 480, time.Minute, MetricsGranularityMinutely, Version8},
		"default period":     {MetricsPeriod{}, 60, time.Minute, MetricsGranularityMinutely, Version7},
		"hourly":             {MetricsPeriod{Hours: 24}, 144, 10 * time.Minute, MetricsGranularityHourly, Version8},
		"clamped hours":      {MetricsPeriod{Hours: 100}, 432, 10 * time.Minute, MetricsGranularityHourly, Version8},
		"Sidekiq 7 hourly":   {MetricsPeriod{Hours: 24}, 144, 10 * time.Minute, MetricsGranularityHourly, Version7},
	} {
		t.Run(name, func(t *testing.T) {
			for _, filter := range []string{"", "éMAIL", "missing"} {
				t.Run("filter="+filter, func(t *testing.T) {
					var previous []redis.Cmder
					var fetched []string
					var populated int64
					batches := 0
					assertReleased := func() {
						for _, cmd := range previous {
							if cmd.(*redis.MapStringStringCmd).Val() != nil {
								t.Fatal("previous batch still retains a response map")
							}
						}
						previous = nil
					}
					client := metricsHookClient(t, func(_ context.Context, cmds []redis.Cmder) error {
						assertReleased()
						batches++
						if len(cmds) > metricsTopJobsBatchSize {
							t.Fatalf("in-flight responses = %d, limit %d", len(cmds), metricsTopJobsBatchSize)
						}
						previous = cmds
						for _, cmd := range cmds {
							if cmd.Name() != "hgetall" {
								t.Fatalf("unexpected command: %s", cmd.Name())
							}
							key := cmd.Args()[1].(string)
							fetched = append(fetched, key)
							isSidekiq7 := len(strings.Split(key, "|")[1]) == 8
							if spec.version == Version7 && !isSidekiq7 || spec.version == Version8 && isSidekiq7 {
								continue
							}
							populated++
							cmd.(*redis.MapStringStringCmd).SetVal(map[string]string{
								"App::ÉMAILJob|p": "11", "App::ÉMAILJob|f": "1", "App::ÉMAILJob|ms": "2500",
								"app::émailJob|p": "3", "app::émailJob|ms": "500",
								"OtherJob|p": "20", "OtherJob|f": "2", "OtherJob|ms": "5000",
								"invalid": "123", "|p": "100", "InvalidJob|p": "NaN",
							})
						}
						return nil
					})
					result, err := client.GetMetricsTopJobs(context.Background(), spec.period, filter)
					if err != nil {
						t.Fatal(err)
					}
					assertReleased()
					if result.Granularity != spec.granularity || !result.StartsAt.Equal(result.EndsAt.Add(-time.Duration(spec.buckets-1)*spec.stride)) {
						t.Fatalf("incorrect period boundaries: %+v", result)
					}
					var wantKeys []string
					for bucket := range spec.buckets {
						wantKeys = append(wantKeys, metricsRollupKeys(result.EndsAt.Add(-time.Duration(bucket)*spec.stride), spec.granularity)...)
					}
					if !slices.Equal(fetched, wantKeys) {
						t.Fatal("bucket keys missing, duplicated or fetched out of order")
					}
					if wantBatches := (len(wantKeys) + metricsTopJobsBatchSize - 1) / metricsTopJobsBatchSize; batches != wantBatches {
						t.Fatalf("batches = %d, want %d", batches, wantBatches)
					}
					want := make(map[string]MetricsJobTotals)
					if populated > 0 && filter != "missing" {
						want["App::ÉMAILJob"] = MetricsJobTotals{Processed: 11 * populated, Failed: populated, Milliseconds: 2500 * populated, Seconds: 2.5 * float64(populated)}
						want["app::émailJob"] = MetricsJobTotals{Processed: 3 * populated, Milliseconds: 500 * populated, Seconds: 0.5 * float64(populated)}
						if filter == "" {
							want["OtherJob"] = MetricsJobTotals{Processed: 20 * populated, Failed: 2 * populated, Milliseconds: 5000 * populated, Seconds: 5 * float64(populated)}
						}
					}
					if !reflect.DeepEqual(result.Jobs, want) {
						t.Fatalf("totals = %+v, want %+v", result.Jobs, want)
					}
				})
			}
		})
	}
}

func TestGetMetricsTopJobs_BatchErrors(t *testing.T) {
	failure := errors.New("metrics pipeline failed")
	for name, spec := range map[string]struct {
		cancelAt int
		failAt   int
		cmdError bool
		wantErr  error
	}{
		"pre-canceled":       {cancelAt: -1, wantErr: context.Canceled},
		"canceled on reply":  {cancelAt: 1, wantErr: context.Canceled},
		"canceled mid-fetch": {cancelAt: 2, wantErr: context.Canceled},
		"pipeline error":     {failAt: 2, wantErr: failure},
		"command error":      {failAt: 2, cmdError: true, wantErr: failure},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if spec.cancelAt == -1 {
				cancel()
			}
			calls := 0
			client := metricsHookClient(t, func(_ context.Context, cmds []redis.Cmder) error {
				calls++
				for _, cmd := range cmds {
					cmd.(*redis.MapStringStringCmd).SetVal(map[string]string{"EmailJob|p": "1"})
				}
				if calls == spec.cancelAt {
					cancel()
				}
				if calls == spec.failAt {
					if spec.cmdError {
						cmds[0].SetErr(failure)
						return nil
					}
					return failure
				}
				return nil
			})
			result, err := client.GetMetricsTopJobs(ctx, MetricsPeriod{Minutes: 60}, "email")
			if !errors.Is(err, spec.wantErr) {
				t.Fatalf("error = %v, want %v", err, spec.wantErr)
			}
			if len(result.Jobs) != 0 {
				t.Fatalf("failed request returned partial totals: %+v", result.Jobs)
			}
			wantCalls := max(spec.cancelAt, spec.failAt)
			if calls != wantCalls {
				t.Fatalf("pipeline calls = %d, want %d", calls, wantCalls)
			}
		})
	}
}
