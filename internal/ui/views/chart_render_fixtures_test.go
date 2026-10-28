package views

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/stats"
)

func chartTestJobMetrics() *JobMetrics {
	j := NewJobMetrics(nil)
	j.SetJobMetrics("ExampleJob", "1h")
	base := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	j.Update(jobMetricsDataMsg{result: sidekiq.MetricsJobDetailResult{
		StartsAt: base, EndsAt: base.Add(2 * time.Minute), BucketCount: 5,
		Hist: map[string][]int64{
			base.Format(time.RFC3339):                      {1, 2, 3, 4, 5},
			base.Add(time.Minute).Format(time.RFC3339):     {5, 4, 3, 2, 1},
			base.Add(2 * time.Minute).Format(time.RFC3339): {2, 4, 1, 3, 6},
		},
	}})
	return j
}

func TestGoldenJobMetricsCharts(t *testing.T) {
	for name, size := range map[string][2]int{"normal": {80, 24}, "short": {80, 8}, "one panel": {80, 4}, "narrow": {12, 12}} {
		t.Run(name, func(t *testing.T) {
			j := chartTestJobMetrics()
			j.SetSize(size[0], size[1])
			output := j.View()
			if j.View() != output {
				t.Fatal("cached render changed golden output")
			}
			golden.RequireEqual(t, []byte(ansi.Strip(output)))
		})
	}
}

func BenchmarkChartOwnerChanges(b *testing.B) {
	for _, size := range [][2]int{{120, 30}, {240, 80}, {500, 200}} {
		for _, mode := range []string{"stable", "data", "resize", "styles"} {
			b.Run(fmt.Sprintf("dashboard/%dx%d/%s", size[0], size[1], mode), func(b *testing.B) {
				d := testDashboard()
				d.realtimeProcessed = nil
				d.realtimeFailed = nil
				d.realtimeTimes = nil
				d.SetSize(size[0], size[1])
				for i := range d.realtimeProcessed {
					d.realtimeProcessed[i] = 50
					d.realtimeTimes[i] = time.Unix(1700000000+int64(i-len(d.realtimeTimes)+1)*5, 0)
				}
				d.lastDeltaP, d.lastDeltaF = 50, 0
				d.lastPollAt = time.Unix(1700000000, 0)
				d.Update(stats.UpdateMsg{Data: stats.Data{Processed: 0}})
				auditDashboardSink = d.View()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					switch mode {
					case "data":
						d.Update(stats.UpdateMsg{Data: stats.Data{Processed: int64(i+1) * 50, UpdatedAt: time.Unix(1700000000+int64(i+1)*5, 0)}})
					case "resize":
						d.SetSize(size[0]+1-i%2, size[1])
					case "styles":
						d.SetStyles(Styles{})
					}
					auditDashboardSink = d.View()
				}
			})
			b.Run(fmt.Sprintf("job_metrics/%dx%d/%s", size[0], size[1], mode), func(b *testing.B) {
				j := chartTestJobMetrics()
				j.SetSize(size[0], size[1])
				auditDashboardSink = j.View()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					switch mode {
					case "data":
						result := j.result
						result.Hist[result.StartsAt.Format(time.RFC3339)][0] = int64(i%99 + 50)
						j.Update(jobMetricsDataMsg{result: result})
					case "resize":
						j.SetSize(size[0]+1-i%2, size[1])
					case "styles":
						j.SetStyles(Styles{})
					}
					auditDashboardSink = j.View()
				}
			})
		}
	}
}
