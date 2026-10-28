package ui

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type retentionMetricsClient struct {
	sidekiq.API
	classes int
	calls   int
}

func (*retentionMetricsClient) MetricsPeriodOrder(context.Context) []string {
	return []string{"1h"}
}

func (c *retentionMetricsClient) GetMetricsTopJobs(context.Context, sidekiq.MetricsPeriod, string) (sidekiq.MetricsTopJobsResult, error) {
	c.calls++
	result := sidekiq.MetricsTopJobsResult{Jobs: make(map[string]sidekiq.MetricsJobTotals, c.classes)}
	for i := range c.classes {
		result.Jobs[fmt.Sprintf("Application::Tenant%06d::Worker", i)] = sidekiq.MetricsJobTotals{
			Processed: int64(i + 1), Seconds: float64(i+1) / 10,
		}
	}
	return result, nil
}

func retentionMetricsApp(classes int) App {
	app := New(&retentionMetricsClient{classes: classes}, "retention", false, nil)
	resized, _ := app.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	return resized.(App)
}

func loadRetentionMetrics(app App) App {
	cmd := app.setActiveView(viewMetrics)
	updated, _ := app.Update(cmd())
	return updated.(App)
}

func metricsHeap(app *App) int64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	runtime.KeepAlive(app)
	if stats.HeapAlloc > math.MaxInt64 {
		panic("heap sample exceeds signed byte range")
	}
	return int64(stats.HeapAlloc)
}

// TestAppHiddenMetricsRetention diagnoses live heap; byte counts are not assertions.
func TestAppHiddenMetricsRetention(t *testing.T) {
	for _, count := range []int{10000, 100000} {
		t.Run(fmt.Sprintf("classes_%d", count), func(t *testing.T) {
			app := retentionMetricsApp(count)
			base := metricsHeap(&app)
			app = loadRetentionMetrics(app)
			active := metricsHeap(&app)
			app.setActiveView(viewDashboard)
			if app.activeViewID() != viewDashboard {
				t.Fatal("did not navigate away from Metrics")
			}
			hidden := metricsHeap(&app)
			_, disposable := app.viewRegistry[viewMetrics].(views.Disposable)
			app.viewRegistry[viewMetrics] = views.NewMetrics(nil)
			released := metricsHeap(&app)
			runtime.KeepAlive(app)
			t.Logf("classes=%d disposable=%t active_delta_B=%d hidden_delta_B=%d navigation_released_B=%d registry_release_B=%d",
				count, disposable, active-base, hidden-base, active-hidden, hidden-released)
		})
	}
}

func BenchmarkAppMetricsEnterLeave(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprintf("classes_%d", count), func(b *testing.B) {
			app := retentionMetricsApp(count)
			client := app.sidekiq.(*retentionMetricsClient)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				app = loadRetentionMetrics(app)
				app.setActiveView(viewDashboard)
			}
			b.ReportMetric(float64(client.calls)/float64(b.N), "fetches/op")
			runtime.KeepAlive(app)
		})
	}
}
