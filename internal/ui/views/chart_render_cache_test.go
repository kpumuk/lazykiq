package views

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/stats"
)

func TestDashboardChartCacheUpdates(t *testing.T) {
	d := testDashboard()
	d.SetSize(80, 24)
	before := d.View()
	if d.View() != before {
		t.Fatal("stable redraw changed output")
	}
	historyBefore := d.historyChart
	d.Update(stats.UpdateMsg{Data: stats.Data{Processed: 100, Failed: 2}})
	d.Update(stats.UpdateMsg{Data: stats.Data{Processed: 130, Failed: 3, UpdatedAt: time.Date(2026, 10, 25, 12, 0, 15, 0, time.UTC)}})
	if d.realtimeChart.ready {
		t.Fatal("realtime update did not invalidate chart")
	}
	if got := d.View(); got == before || !strings.Contains(got, "Processed: 30") {
		t.Fatal("realtime update did not render")
	}
	if d.historyChart != historyBefore {
		t.Fatal("realtime update changed historical chart")
	}
	realtimeBefore := d.realtimeChart
	d.Update(stats.UpdateMsg{Data: stats.Data{Processed: 130, Failed: 3}})
	d.Update(DashboardRedisInfoMsg{RedisInfo: sidekiq.RedisInfo{Version: "7.4"}})
	d.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	d.View()
	if d.realtimeChart != realtimeBefore || d.historyChart != historyBefore {
		t.Fatal("unrelated messages invalidated charts")
	}
	d.Update(DashboardHistoryMsg{history: sidekiq.StatsHistory{Dates: d.historyDates, Processed: []int64{10, 20, 40}, Failed: []int64{1, 2, 3}}})
	if d.historyChart.ready || d.realtimeChart != realtimeBefore {
		t.Fatal("history update invalidated wrong chart")
	}
	if got := d.View(); !strings.Contains(got, "Processed: 70") || d.historyChart == historyBefore {
		t.Fatal("history refresh did not render")
	}
	for _, size := range [][2]int{{120, 30}, {20, 8}, {1, 1}, {80, 24}} {
		d.SetSize(size[0], size[1])
		got := d.View()
		if lipgloss.Height(got) != size[1] || lipgloss.Width(got) > size[0] {
			t.Fatalf("invalid layout at %v", size)
		}
	}
	before = d.View()
	d.SetStyles(Styles{MetricValue: lipgloss.NewStyle().Bold(true), ChartSuccess: lipgloss.NewStyle().Bold(true)})
	if d.realtimeChart.ready || d.historyChart.ready {
		t.Fatal("styles did not invalidate charts")
	}
	styled := d.View()
	d.CancelRequests()
	if d.realtimeChart != (chartRenderCache{}) || d.historyChart != (chartRenderCache{}) {
		t.Fatal("hidden dashboard retained chart data")
	}
	if got := d.View(); got != styled {
		t.Fatal("dashboard did not rebuild after hiding")
	}
	if before == "" {
		t.Fatal("missing dashboard")
	}
}

func TestJobMetricsChartCacheUpdates(t *testing.T) {
	j := chartTestJobMetrics()
	j.SetSize(80, 24)
	before := j.View()
	if j.View() != before {
		t.Fatal("stable redraw changed output")
	}
	histogramBefore, scatterBefore := j.histogramChart, j.scatterChart
	j.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if j.focused != 1 {
		t.Fatal("panel focus did not change")
	}
	j.Update(tea.KeyPressMsg{Code: '}', Text: "}"})
	if j.period != "2h" || !strings.Contains(j.View(), "2h") {
		t.Fatal("period selection was cached")
	}
	if j.histogramChart != histogramBefore || j.scatterChart != scatterBefore {
		t.Fatal("focus or period metadata invalidated charts before data arrived")
	}
	j.Update(jobMetricsDataMsg{result: j.result})
	if j.histogramChart.ready || j.scatterChart.ready {
		t.Fatal("identical detail reply did not invalidate charts")
	}
	j.View()
	result := j.result
	result.Hist = map[string][]int64{result.StartsAt.Format(time.RFC3339): {50, 10, 1, 0, 0}}
	j.Update(jobMetricsDataMsg{result: result})
	if j.View() == before {
		t.Fatal("data refresh was cached")
	}
	for _, size := range [][2]int{{120, 30}, {12, 12}, {80, 4}, {80, 24}} {
		j.SetSize(size[0], size[1])
		got := j.View()
		if lipgloss.Height(got) != max(size[1], 5) || lipgloss.Width(got) > size[0] {
			t.Fatalf("invalid layout at %v", size)
		}
	}
	j.SetStyles(Styles{ChartHistogram: lipgloss.NewStyle().Bold(true)})
	if j.histogramChart.ready || j.scatterChart.ready {
		t.Fatal("styles did not invalidate charts")
	}
	styled := j.View()
	j.SetStyles(j.styles)
	if j.View() != styled {
		t.Fatal("styles invalidation changed render")
	}
	j.CancelRequests()
	if j.histogramChart.ready || j.scatterChart.ready {
		t.Fatal("hidden metrics retained chart output")
	}
	if j.View() != styled {
		t.Fatal("hidden metrics did not rebuild correctly")
	}
	j.Update(jobMetricsDataMsg{})
	if j.histogramChart != (chartRenderCache{}) || j.scatterChart != (chartRenderCache{}) {
		t.Fatal("empty refresh retained charts")
	}
	j.SetJobMetrics("OtherJob", "4h")
	if got := j.View(); !strings.Contains(got, "Loading...") || !strings.Contains(got, "OtherJob") {
		t.Fatal("job selection retained previous render")
	}
	j.Update(jobMetricsDataMsg{result: result})
	j.View()
	j.Dispose()
	if j.histogramChart != (chartRenderCache{}) || j.scatterChart != (chartRenderCache{}) || j.processed != nil {
		t.Fatal("disposed metrics retained chart data")
	}
}

func TestChartOwnersOwnData(t *testing.T) {
	d := testDashboard()
	d.SetSize(80, 24)
	history := sidekiq.StatsHistory{Dates: d.historyDates, Processed: []int64{10, 20, 40}, Failed: []int64{1, 2, 3}}
	d.Update(DashboardHistoryMsg{history: history})
	dashboardOutput := d.View()
	history.Processed[0] = 9999
	history.Failed[0] = 9999
	history.Dates[0] = time.Time{}
	d.CancelRequests()
	if d.View() != dashboardOutput {
		t.Fatal("dashboard retained borrowed chart inputs")
	}

	j := chartTestJobMetrics()
	j.SetSize(80, 24)
	metricsOutput := j.View()
	for _, counts := range j.result.Hist {
		counts[0] = 9999
	}
	j.CancelRequests()
	if j.View() != metricsOutput {
		t.Fatal("metrics retained borrowed chart inputs")
	}
}

func TestChartRenderCacheBound(t *testing.T) {
	var c chartRenderCache
	if _, cached := c.get(0, 0); cached {
		t.Fatal("zero cache was ready")
	}
	output := strings.Repeat("x", maxChartRenderBytes)
	if c.store(80, 24, output) != output {
		t.Fatal("store changed output")
	}
	if got, cached := c.get(80, 24); !cached || got != output {
		t.Fatal("bounded output was not cached")
	}
	if _, cached := c.get(81, 24); cached {
		t.Fatal("resized chart reused output")
	}
	oversized := output + "x"
	if c.store(80, 24, oversized) != oversized || c != (chartRenderCache{}) {
		t.Fatal("oversized output was changed or retained")
	}
}

func TestChartOwnerStableAllocationBound(t *testing.T) {
	for name, makeView := range map[string]func() View{
		"dashboard":   func() View { return testDashboard() },
		"job metrics": func() View { return chartTestJobMetrics() },
	} {
		t.Run(name, func(t *testing.T) {
			v := makeView()
			v.SetSize(240, 80)
			v.View()
			// A stable 240x80 redraw used >12 MB of backing canvases.
			result := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					v.View()
				}
			})
			if got := result.AllocedBytesPerOp(); got > 1<<20 {
				t.Fatalf("stable redraw allocated %d bytes/op", got)
			}
		})
	}
}
