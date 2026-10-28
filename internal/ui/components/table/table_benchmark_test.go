package table

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var tableRenderSink string

func cardinalityRows(n int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{ID: strconv.Itoa(i), Cells: []string{"default", "SomeJob", "[123, \"short arguments\"]"}}
	}
	return rows
}

func cardinalityTable(rows []Row) Model {
	m := New(WithColumns([]Column{{Title: "Queue", Width: 12}, {Title: "Class", Width: 16}, {Title: "Args", Width: 20}}))
	m.SetSize(120, 24)
	m.SetRows(rows)
	return m
}

// BenchmarkTableCardinality varies loaded rows with 22 visible rows.
func BenchmarkTableCardinality(b *testing.B) {
	for _, n := range []int{75, 1000, 10000, 100000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			rows := cardinalityRows(n)
			m := cardinalityTable(rows)
			b.Run("SetRows", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					m.SetRows(rows)
				}
			})
			b.Run("Navigate", func(b *testing.B) {
				b.ReportAllocs()
				for i := range b.N {
					m.SetCursor(i % n)
				}
			})
			b.Run("View", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					tableRenderSink = m.View()
				}
			})
		})
	}
}

func BenchmarkTableViewportChanges(b *testing.B) {
	for _, n := range []int{75, 1000, 10000, 100000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			rows := cardinalityRows(n)
			rows[n-1].Cells[2] = strings.Repeat("x", 240)
			m := cardinalityTable(rows)
			b.Run("Horizontal", func(b *testing.B) {
				m.ScrollToStart()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					m.scrollX((1 - i%2*2) * 4)
				}
			})
			b.Run("Resize", func(b *testing.B) {
				m.SetSize(110, 24)
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					m.SetSize(120-i%2*10, 24)
				}
			})
		})
	}
}

// BenchmarkTableOutlier keeps total input small while one cell widens every row.
// Fixtures and table construction are outside the measured refresh loop.
func BenchmarkTableOutlier(b *testing.B) {
	for _, n := range []int{75, 300, 1200} {
		for _, width := range []int{0, 1024, 4096, 16384, 65536} {
			b.Run(fmt.Sprintf("rows=%d/width=%d", n, width), func(b *testing.B) {
				rows := make([]Row, n)
				for i := range rows {
					rows[i] = Row{ID: strconv.Itoa(i), Cells: []string{"default", "SomeJob", "[123, \"short arguments\"]"}}
				}
				if width > 0 {
					rows[0].Cells[0] = strings.Repeat("x", width)
				}
				m := New(WithColumns([]Column{{Title: "Queue", Width: 12}, {Title: "Class", Width: 16}, {Title: "Args", Width: 20}}))
				m.SetSize(120, 24)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					m.SetRows(rows)
				}
			})
		}
	}
}

func BenchmarkTableOutlierScroll(b *testing.B) {
	for name, wide := range map[string]string{
		"ASCII":        strings.Repeat("x", 65536),
		"ANSI Unicode": "\x1b[1m" + strings.Repeat("界e\u0301", 10923) + "TAIL\x1b[0m",
	} {
		for column := range 3 {
			b.Run(fmt.Sprintf("%s/column=%d", name, column), func(b *testing.B) {
				rows := make([]Row, 75)
				for i := range rows {
					rows[i] = Row{ID: strconv.Itoa(i), Cells: []string{"default", "SomeJob", "short arguments"}}
				}
				rows[0].Cells[column] = wide
				m := New(WithColumns([]Column{{Title: "Queue", Width: 12}, {Title: "Class", Width: 16}, {Title: "Args", Width: 20}}))
				m.SetSize(120, 24)
				m.SetRows(rows)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					m.ScrollToStart()
					m.ScrollToEnd()
				}
			})
		}
	}
}

func BenchmarkWideSearchRedraw(b *testing.B) {
	for _, width := range []int{8 * 1024, 64 * 1024} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			m := New(WithColumns([]Column{{Title: "Job", Width: 20}, {Title: "Arguments", Width: 40}}))
			m.SetSize(120, 20)
			rows := make([]Row, 75)
			args := strings.Repeat("x", width)
			for i := range rows {
				rows[i] = Row{ID: strconv.Itoa(i), Cells: []string{"WideJob", args}}
			}
			m.SetRows(rows)
			b.Run("View", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					_ = m.View()
				}
			})
			b.Run("Navigate", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
				}
			})
		})
	}
}
