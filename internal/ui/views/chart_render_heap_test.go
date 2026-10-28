package views

import (
	"fmt"
	"runtime"
	"strconv"
	"testing"
)

func chartLiveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return mem.HeapAlloc
}

// These diagnostic deltas are retained Go heap, not RSS or allocation churn.
// Keep the owner and its inputs alive in every phase, including after release.
func TestChartRenderRetainedHeap(t *testing.T) {
	for _, size := range [][2]int{{240, 80}, {500, 200}} {
		for _, owner := range []string{"dashboard", "job_metrics"} {
			t.Run(fmt.Sprintf("%s/%dx%d", owner, size[0], size[1]), func(t *testing.T) {
				var v View
				var release func()
				if owner == "dashboard" {
					d := testDashboard()
					v, release = d, d.CancelRequests
				} else {
					j := chartTestJobMetrics()
					v, release = j, j.Dispose
				}
				v.SetSize(size[0], size[1])
				before := chartLiveHeap()
				v.View()
				warm := chartLiveHeap()
				for range 4 {
					v.View()
				}
				stable := chartLiveHeap()
				release()
				released := chartLiveHeap()
				runtime.KeepAlive(v)
				runtime.KeepAlive(release)
				t.Logf("bytes: before=%d warm=%d stable=%d released=%d; deltas warm=%s stable=%s released=%s", before, warm, stable, released, chartHeapDelta(warm, before), chartHeapDelta(stable, before), chartHeapDelta(released, before))
				// Two panels retain at most 1 MiB apiece, plus small allocator
				// size-class overhead. Inputs are already included in 'before'.
				if stable > before && stable-before > 3<<20 {
					t.Fatal("chart cache exceeded retained-memory budget")
				}
			})
		}
	}
}

func chartHeapDelta(current, before uint64) string {
	if current >= before {
		return strconv.FormatUint(current-before, 10)
	}
	return "-" + strconv.FormatUint(before-current, 10)
}
