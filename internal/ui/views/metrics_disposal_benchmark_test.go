package views

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func BenchmarkMetricsDispose(b *testing.B) {
	for _, count := range []int{100, 10000, 100000} {
		b.Run(fmt.Sprintf("classes_%d", count), func(b *testing.B) {
			jobs := make(map[string]sidekiq.MetricsJobTotals, count)
			for i := range count {
				jobs[fmt.Sprintf("Application::Tenant%06d::Worker", i)] = sidekiq.MetricsJobTotals{
					Processed: int64(i + 1), Seconds: float64(i+1) / 10,
				}
			}
			loaded := *metricsListFixture(160, 30, "", jobs)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				m := loaded
				m.Dispose()
				runtime.KeepAlive(m)
			}
			runtime.KeepAlive(loaded)
		})
	}
}
