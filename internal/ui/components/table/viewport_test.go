package table

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestTableOutlierViewport(t *testing.T) {
	for name, column := range map[string]int{"first": 0, "middle": 1, "last": 2} {
		t.Run(name, func(t *testing.T) {
			wide := "\x1b[1mHEAD" + strings.Repeat("界e\u0301", 2048) + "TAIL\x1b[0m"
			rows := []Row{row("short", "one", "two", "three"), row("wide", "one", "two", "three"), row("full", "", "", "")}
			rows[1].Cells[column] = wide
			m := newTestTable(
				WithColumns([]Column{{Title: "First", Width: 8}, {Title: "Middle", Width: 8}, {Title: "Last", Width: 8}}),
				WithWidth(120), WithHeight(5),
			)
			m.SetRowsWithMeta(rows, map[int]string{2: "\x1b[3mfull override\x1b[0m"}, nil)
			if m.colWidths[column] != ansi.StringWidth(wide) {
				t.Fatal("wide metadata was capped")
			}
			start := 0
			for i := range column {
				start += m.colWidths[i] + 1
			}
			m.scrollX(start)
			if !strings.Contains(ansi.Strip(m.View()), "HEAD") {
				t.Fatal("cannot inspect start of metadata")
			}
			m.ScrollToEnd()
			if !strings.Contains(ansi.Strip(m.View()), "TAIL") {
				t.Fatal("cannot inspect end of metadata")
			}
			for _, width := range []int{0, 1, 2, 5, 12, 120} {
				m.SetSize(width, 5)
				m.ScrollToStart()
				assertSingleLineTableSize(t, m)
				m.scrollX(start + 5) // Includes offsets inside a double-width grapheme.
				assertSingleLineTableSize(t, m)
				m.ScrollToEnd()
				assertSingleLineTableSize(t, m)
				assertANSIGrounded(t, m.View())
			}
			m.ScrollToStart()
			if !strings.Contains(m.View(), "\x1b[3mfull override\x1b[0m") {
				t.Fatal("full-row style was lost")
			}
			if m.Rows()[1].Cells[column] != wide {
				t.Fatal("rendering changed stored metadata")
			}
		})
	}
}

func TestTableViewportSelectionSpan(t *testing.T) {
	styles := blankStyles()
	styles.Selected = lipgloss.NewStyle().Reverse(true)
	m := New(
		WithStyles(styles), WithColumns([]Column{{Title: "Value", Width: 65536}}),
		WithRows([]Row{row("selected", "value"), row("other", "short")}),
		WithWidth(12), WithHeight(4),
	)
	m.SetSelectionSpans(map[int]SelectionSpan{0: {Start: 65530, End: -1}})
	m.ScrollToEnd()
	if !strings.Contains(m.View(), "\x1b[7m      \x1b[m") {
		t.Fatalf("selection span missing from visible padding: %q", m.View())
	}
	assertSingleLineTableSize(t, m)
	m.MoveDown(1)
	if !strings.Contains(m.View(), "\x1b[7m           \x1b[m") {
		t.Fatalf("whole-row selection missing from visible padding: %q", m.View())
	}
}

func TestTableViewportPreservesANSIState(t *testing.T) {
	for name, prefix := range map[string]string{
		"unclosed style": "\x1b[1mA",
		"style only":     "\x1b[1m",
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestTable(
				WithColumns([]Column{{Title: "A", Width: 8}, {Title: "B", Width: 8}, {Title: "C", Width: 8}}),
				WithRows([]Row{row("selected"), row("styled", prefix, "B", "\x1b[0mC")}),
				WithWidth(2), WithHeight(4),
			)
			m.scrollX(9)
			if !strings.Contains(m.rowContent[1], "\x1b[1mB\x1b[0m") {
				t.Fatalf("ANSI state from invisible cells was lost: %q", m.rowContent[1])
			}
			assertSingleLineTableSize(t, m)
			assertANSIGrounded(t, m.View())
		})
	}
}

func TestTableViewportMatchesPaddedLayout(t *testing.T) {
	for name, tc := range map[string]struct {
		columns []Column
		cells   []string
	}{
		"alignment": {
			columns: []Column{{Title: "Left", Width: 12}, {Title: "Right", Width: 12, Align: AlignRight}, {Title: "Center", Width: 13, Align: AlignCenter}},
			cells:   []string{"abc", "\x1b[1mright\x1b[0m", "center"},
		},
		"Unicode": {
			columns: []Column{{Title: "東京", Width: 8}, {Title: "Value", Width: 5}},
			cells:   []string{"界e\u0301👩‍💻", "\x1b[3m東京\x1b[0m"},
		},
		"long headers": {
			columns: []Column{{Title: "Long\tHeader", Width: 2}, {Title: "Another header", Width: 3}},
			cells:   []string{"a", "b"},
		},
		"single column": {
			columns: []Column{{Title: "Only", Width: 10, Align: AlignCenter}},
			cells:   []string{"only"},
		},
		"missing cells": {
			columns: []Column{{Title: "A", Width: 12}, {Title: "B", Width: 12}},
			cells:   []string{"first"},
		},
		"extra cells": {
			columns: []Column{{Title: "A", Width: 8}},
			cells:   []string{"first", "extra", "last"},
		},
		"empty row": {
			columns: []Column{{Title: "A", Width: 20}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, width := range []int{1, 2, 4, 9, 16, 40} {
				m := newTestTable(WithColumns(tc.columns), WithRows([]Row{row("one", tc.cells...)}), WithWidth(width), WithHeight(3))
				cells := make([]string, 0, len(tc.cells))
				titles := make([]string, 0, len(tc.columns))
				for i, cell := range tc.cells {
					align := AlignLeft
					if i < len(tc.columns) {
						align = tc.columns[i].Align
					}
					cellWidth := m.lastColWidth
					if i < len(tc.columns)-1 {
						cellWidth = m.colWidths[i]
					}
					cells = append(cells, padCell(cell, cellWidth, align))
				}
				for i, column := range tc.columns {
					cellWidth := m.colWidths[i]
					if i == len(tc.columns)-1 {
						cellWidth = m.lastColWidth
					}
					titles = append(titles, padCell(column.Title, cellWidth, column.Align))
				}
				rawRow := strings.Join(cells, " ")
				rawHeader := strings.Join(titles, " ")
				for offset := 0; offset <= m.maxScrollOffset(); offset++ {
					m.scrollX(offset - m.xOffset)
					assertPaddedViewport(t, m.rowContent[0], rawRow, offset, m.contentWidth())
					assertPaddedViewport(t, strings.Split(m.header, "\n")[0], rawHeader, offset, m.contentWidth())
					assertSingleLineTableSize(t, m)
					assertANSIGrounded(t, m.View())
				}
			}
		})
	}
}

func assertPaddedViewport(t *testing.T, got, padded string, offset, width int) {
	t.Helper()
	// Whole-glyph offsets preserve the previous padded layout exactly.
	if offset < ansi.StringWidth(padded) && ansi.StringWidth(ansi.Cut(padded, 0, offset)) < offset {
		return
	}
	got = ansi.Strip(got)
	want := ansi.Strip(applyHorizontalScroll(padded, offset, width))
	if got != want {
		t.Fatalf("width=%d offset=%d: got %q, want %q", width, offset, got, want)
	}
}

func TestTableFullRowOutlierViewport(t *testing.T) {
	wide := "\x1b[3mHEAD" + strings.Repeat("界", 32768) + "TAIL\x1b[0m"
	m := newTestTable(
		WithColumns([]Column{{Title: "Class", Width: 8}, {Title: "Value", Width: 12}}),
		WithWidth(24), WithHeight(5),
	)
	m.SetRowsWithMeta([]Row{row("one", "a", "b"), row("full"), row("three", "c", "d")}, map[int]string{1: wide}, nil)
	if !strings.Contains(m.View(), "HEAD") {
		t.Fatal("missing full-row start")
	}
	m.ScrollToEnd()
	if !strings.Contains(m.View(), "TAIL") {
		t.Fatal("missing full-row tail")
	}
	if m.maxRowWidth != ansi.StringWidth(wide) {
		t.Fatal("full-row content was capped")
	}
	assertSingleLineTableSize(t, m)
	assertANSIGrounded(t, m.View())
}

func TestGoldenOutlierViewport(t *testing.T) {
	for _, column := range []int{0, 1, 2} {
		for _, width := range []int{12, 36} {
			t.Run(fmt.Sprintf("column=%d/width=%d", column, width), func(t *testing.T) {
				rows := []Row{row("short", "one", "two", "three"), row("wide", "one", "two", "three"), row("full", "", "", "")}
				rows[1].Cells[column] = "\x1b[1mHEAD" + strings.Repeat("x", 64) + "TAIL\x1b[0m"
				m := newTestTable(
					WithColumns([]Column{{Title: "First", Width: 8}, {Title: "Middle", Width: 8, Align: AlignRight}, {Title: "Last", Width: 8, Align: AlignCenter}}),
					WithWidth(width), WithHeight(5),
				)
				m.SetRowsWithMeta(rows, map[int]string{2: "\x1b[3mfull override\x1b[0m"}, nil)
				start := m.View()
				m.ScrollRight()
				middle := m.View()
				m.ScrollToEnd()
				end := m.View()
				golden.RequireEqual(t, []byte(ansi.Strip(start+"\n\n"+middle+"\n\n"+end)))
			})
		}
	}
}

func TestGoldenUnicodeViewport(t *testing.T) {
	for _, width := range []int{2, 5, 12} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			m := newTestTable(
				WithColumns([]Column{{Title: "文字", Width: 1}}),
				WithWidth(width), WithHeight(4),
			)
			m.SetRowsWithMeta([]Row{row("one", "A界e\u0301👩‍💻TAIL"), row("full")}, map[int]string{1: "\x1b[3m東京 tail\x1b[0m"}, nil)
			start := m.View()
			m.scrollX(2)
			middle := m.View()
			m.ScrollToEnd()
			end := m.View()
			golden.RequireEqual(t, []byte(ansi.Strip(start+"\n\n"+middle+"\n\n"+end)))
		})
	}
}
