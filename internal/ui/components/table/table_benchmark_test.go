package table

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
