package views

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

const (
	benchmarkSortedSetSize       = 1000
	benchmarkFilteredMatches     = 500
	benchmarkSortedWindowPages   = 3
	benchmarkSortedFallbackSize  = 25
	benchmarkSortedWindowSize    = benchmarkSortedWindowPages * benchmarkSortedFallbackSize
	benchmarkSortedScrollStep    = benchmarkSortedFallbackSize
	benchmarkSortedFilterNeedle  = "match_even"
	benchmarkSortedFilterMissTag = "odd_only"
)

var benchmarkSortedWindowSink sortedWindowResult

func BenchmarkFilteredSortedScroll(b *testing.B) {
	client := setupSortedWindowBenchmarkRedis(b)
	ctx := context.Background()
	scrollStarts := benchmarkScrollStarts(benchmarkFilteredMatches, benchmarkSortedWindowSize, benchmarkSortedScrollStep)
	benchmarkFilteredSortedWindow(ctx, b, benchmarkSortedWindowConfig(client), scrollStarts)
}

func BenchmarkFilteredSortedFirstWindow(b *testing.B) {
	client := setupSortedWindowBenchmarkRedis(b)
	ctx := context.Background()
	benchmarkFilteredSortedWindow(ctx, b, benchmarkSortedWindowConfig(client), []int{0})
}

func benchmarkFilteredSortedWindow(
	ctx context.Context,
	b *testing.B,
	baseCfg sortedWindowConfig,
	windowStarts []int,
) {
	b.Run("LegacyFullScan", func(b *testing.B) {
		cfg := baseCfg
		cfg.scanWindow = nil
		benchmarkFilteredSortedVariant(ctx, b, cfg, windowStarts)
	})

	b.Run("WindowedFilteredScan", func(b *testing.B) {
		benchmarkFilteredSortedVariant(ctx, b, baseCfg, windowStarts)
	})
}

func benchmarkFilteredSortedVariant(
	ctx context.Context,
	b *testing.B,
	cfg sortedWindowConfig,
	windowStarts []int,
) {
	b.ReportAllocs()
	for range b.N {
		for _, start := range windowStarts {
			cfg.windowStart = start
			got, err := fetchSortedWindow(ctx, cfg)
			if err != nil {
				b.Fatalf("fetchSortedWindow: %v", err)
			}
			if got.total != benchmarkFilteredMatches {
				b.Fatalf("total = %d, want %d", got.total, benchmarkFilteredMatches)
			}
			benchmarkSortedWindowSink = got
		}
	}
}

func benchmarkSortedWindowConfig(client *sidekiq.Client) sortedWindowConfig {
	return sortedWindowConfig{
		filter:           benchmarkSortedFilterNeedle,
		windowSize:       benchmarkSortedWindowSize,
		fallbackPageSize: benchmarkSortedFallbackSize,
		windowPages:      benchmarkSortedWindowPages,
		scanWindow:       client.ScanDeadJobsWindow,
		scan:             client.ScanDeadJobs,
		fetch:            client.GetDeadJobs,
		bounds:           client.GetDeadBounds,
	}
}

func setupSortedWindowBenchmarkRedis(b *testing.B) *sidekiq.Client {
	b.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("start miniredis: %v", err)
	}
	b.Cleanup(mr.Close)

	client, err := sidekiq.NewClient("redis://" + mr.Addr() + "/0")
	if err != nil {
		b.Fatalf("create client: %v", err)
	}
	b.Cleanup(func() {
		_ = client.Close()
	})

	for i := range benchmarkSortedSetSize {
		marker := benchmarkSortedFilterMissTag
		if i%2 == 0 {
			marker = benchmarkSortedFilterNeedle
		}
		job := fmt.Sprintf(
			`{"jid":"job-%04d","class":"BenchJob","args":[%d], "marker":"%s"}`,
			i,
			i,
			marker,
		)
		if _, err := mr.ZAdd("dead", float64(i+1), job); err != nil {
			b.Fatalf("seed job %d: %v", i, err)
		}
	}

	return client
}

func benchmarkScrollStarts(total, windowSize, step int) []int {
	if total <= 0 {
		return []int{0}
	}
	if windowSize <= 0 {
		windowSize = 1
	}
	if step <= 0 {
		step = 1
	}

	maxStart := max(total-windowSize, 0)
	starts := make([]int, 0, maxStart/step+2)
	for start := 0; start <= maxStart; start += step {
		starts = append(starts, start)
	}
	if starts[len(starts)-1] != maxStart {
		starts = append(starts, maxStart)
	}
	return starts
}
