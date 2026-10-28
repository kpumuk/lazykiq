package table

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkTableRetainedWidths measures refreshes while preserving width history.
// Fixtures and table construction are outside the measured loop.
func BenchmarkTableRetainedWidths(b *testing.B) {
	for _, historicWidth := range []int{0, 65536} {
		b.Run(fmt.Sprintf("historic-width=%d", historicWidth), func(b *testing.B) {
			m := cardinalityTable(nil)
			if historicWidth > 0 {
				wide := cardinalityRows(75)
				wide[0].Cells[0] = strings.Repeat("x", historicWidth)
				m.SetRows(wide)
			}
			rows := cardinalityRows(75)
			m.SetRows(rows)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				m.SetRows(rows)
			}
			b.StopTimer()
			b.ReportMetric(float64(m.colWidths[0]), "col0-cells")
			b.ReportMetric(float64(m.maxRowWidth), "row-cells")
			b.ReportMetric(float64(m.maxScrollOffset()), "scroll-cells")
		})
	}
}
