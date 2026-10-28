package views

import (
	"fmt"
	"testing"
	"time"
)

var auditDashboardSink string

func BenchmarkAuditDashboardStableView(b *testing.B) {
	for _, size := range []struct{ w, h int }{{120, 30}, {240, 80}, {500, 200}} {
		b.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(b *testing.B) {
			d := NewDashboard(nil)
			d.SetSize(size.w, size.h)
			for i := range d.realtimeProcessed {
				d.realtimeProcessed[i] = int64(i % 17)
				d.realtimeFailed[i] = int64(i % 3)
			}
			for i := range 180 {
				d.historyDates = append(d.historyDates, time.Unix(1700000000, 0).AddDate(0, 0, i))
				d.historyProcessed = append(d.historyProcessed, int64(i%23+100))
				d.historyFailed = append(d.historyFailed, int64(i%3))
			}
			auditDashboardSink = d.View()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				auditDashboardSink = d.View()
			}
		})
	}
}
