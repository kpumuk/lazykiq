package devtools

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

var benchmarkTracker *Tracker

func BenchmarkTrackerRecord(b *testing.B) {
	for _, width := range []int{32, 65536, 262144, 1048576} {
		for _, binary := range []bool{false, true} {
			b.Run(fmt.Sprintf("width=%d/bytes=%t", width, binary), func(b *testing.B) {
				tracker := NewTracker()
				ctx := WithOrigin(context.Background(), "benchmark")
				var payload any = strings.Repeat("x", width)
				if binary {
					payload = []byte(strings.Repeat("x", width))
				}
				cmd := redis.NewCmd(ctx, "lpush", "queue:default", payload)
				h := hook{tracker: tracker}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					h.record(ctx, cmd, 123)
				}
				benchmarkTracker = tracker
			})
		}
	}
}

func BenchmarkTrackerRetention(b *testing.B) {
	for _, width := range []int{32, 65536, 262144} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			for range b.N {
				benchmarkTracker = nil
				base := trackerHeap()
				tracker := NewTracker()
				ctx := WithOrigin(context.Background(), "benchmark")
				for i := range 600 {
					cmd := redis.NewCmd(ctx, "lpush", "queue:default", fmt.Sprintf("%06d%s", i, strings.Repeat("x", width)))
					hook{tracker: tracker}.record(ctx, cmd, 123)
				}
				benchmarkTracker = tracker
				live := trackerHeap()
				displayBytes := trackerDisplayBytes(b, tracker)
				for range 500 {
					tracker.AppendLog(LogEntry{Entry: Entry{Command: "short"}})
				}
				overwritten := trackerHeap()
				runtime.KeepAlive(tracker)
				tracker = nil
				benchmarkTracker = nil
				released := trackerHeap()
				b.ReportMetric(float64(displayBytes), "display-B")
				b.ReportMetric(float64(live)-float64(base), "retained-B")
				b.ReportMetric(float64(overwritten)-float64(base), "overwritten-B")
				b.ReportMetric(float64(released)-float64(base), "released-B")
			}
		})
	}
}

func trackerDisplayBytes(b *testing.B, tracker *Tracker) int {
	b.Helper()
	entries := tracker.LogEntries()
	if len(entries) != 500 || entries[0].Seq != 100 || entries[499].Seq != 599 {
		b.Fatal("ring ordering/count changed")
	}
	displayBytes := 0
	for _, entry := range entries {
		displayBytes += len(entry.Entry.Command)
	}
	return displayBytes
}

func trackerHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}
