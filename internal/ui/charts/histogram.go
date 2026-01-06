// Package charts provides data processing utilities for chart rendering.
package charts

import (
	"slices"
	"sort"
	"time"
)

// ScatterPoint represents a single point on a scatter plot.
type ScatterPoint struct {
	X     float64
	Y     float64
	Count int64
}

// ProcessedMetrics holds pre-computed data from histogram to avoid repeated processing.
type ProcessedMetrics struct {
	SortedBuckets []time.Time    // Time buckets sorted chronologically
	BucketTotals  []int64        // Histogram totals per bucket (reversed for chart)
	ScatterPoints []ScatterPoint // Pre-built scatter points
	MaxCount      int64          // Maximum count for scatter point sizing
	MaxBucket     int            // Maximum bucket index with data
	BucketCount   int            // Number of histogram buckets
}

// ProcessHistogramData performs single-pass processing of histogram data.
// Returns a pointer to all computed values needed for rendering charts.
func ProcessHistogramData(hist map[string][]int64, bucketCount int) *ProcessedMetrics {
	if len(hist) == 0 || bucketCount == 0 {
		return &ProcessedMetrics{}
	}

	// First pass: parse times, collect entries, and count non-zero values for pre-allocation
	type bucketEntry struct {
		time   time.Time
		values []int64
	}
	entries := make([]bucketEntry, 0, len(hist))
	nonZeroCount := 0

	for key, values := range hist {
		t, err := time.Parse(time.RFC3339, key)
		if err != nil {
			continue
		}
		entries = append(entries, bucketEntry{time: t, values: values})
		for _, count := range values {
			if count > 0 {
				nonZeroCount++
			}
		}
	}

	// Sort by time
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].time.Before(entries[j].time)
	})

	// Pre-allocate with exact/known capacities
	result := &ProcessedMetrics{
		SortedBuckets: make([]time.Time, 0, len(entries)),
		BucketTotals:  make([]int64, bucketCount),
		ScatterPoints: make([]ScatterPoint, 0, nonZeroCount),
		BucketCount:   bucketCount,
		MaxBucket:     -1,
	}

	// Second pass: compute totals and scatter points (now in sorted order)
	for tIdx, entry := range entries {
		result.SortedBuckets = append(result.SortedBuckets, entry.time)

		for bIdx, count := range entry.values {
			if bIdx >= bucketCount {
				break
			}
			result.BucketTotals[bIdx] += count

			if count == 0 {
				continue
			}
			if count > result.MaxCount {
				result.MaxCount = count
			}
			bucketIdx := bucketCount - 1 - bIdx
			if bucketIdx > result.MaxBucket {
				result.MaxBucket = bucketIdx
			}
			result.ScatterPoints = append(result.ScatterPoints, ScatterPoint{
				X:     float64(tIdx),
				Y:     float64(bucketIdx),
				Count: count,
			})
		}
	}

	// Reverse totals for chart display (matches original behavior)
	slices.Reverse(result.BucketTotals)

	return result
}
