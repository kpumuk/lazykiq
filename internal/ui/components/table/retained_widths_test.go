package table

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestTableRetainedWidthsAcrossWindows(t *testing.T) {
	for name, column := range map[string]int{"first": 0, "middle": 1, "last": 2} {
		t.Run(name, func(t *testing.T) {
			columns := []Column{{Title: "A", Width: 4}, {Title: "B", Width: 5}, {Title: "C", Width: 6}}
			rows := []Row{row("wide", "a", "b", "c"), row("selected", "job", "args", "tail"), row("next", "x", "y", "z")}
			rows[0].Cells[column] = "\x1b[1m" + strings.Repeat("界", 40) + "\x1b[0m"
			m := newTestTable(WithColumns(columns), WithWidth(24), WithHeight(5))
			m.SetRows(rows)
			m.SetCursor(1)
			m.ScrollToEnd()
			widths := slices.Clone(m.colWidths)
			maxWidth, offset := m.maxRowWidth, m.xOffset
			selected := m.content[m.Cursor()-m.YOffset()]

			for _, window := range [][]Row{rows[1:], rows, rows[1:]} {
				m.SetRows(window)
				if !slices.Equal(m.colWidths, widths) || m.maxRowWidth != maxWidth || m.xOffset != offset {
					t.Fatalf("window replacement moved horizontal layout: widths=%v offset=%d", m.colWidths, m.xOffset)
				}
				if m.SelectedRow().ID != "selected" || m.content[m.Cursor()-m.YOffset()] != selected {
					t.Fatal("window replacement changed the selected row's content")
				}
				assertSingleLineTableSize(t, m)
			}

			m.SetRows(nil)
			if !slices.Equal(m.colWidths, widths) || m.xOffset != 0 {
				t.Fatal("clearing rows changed retained widths or failed to reset scroll")
			}
			m.SetRows(rows[1:])
			m.SetColumns(columns)
			if !slices.Equal(m.colWidths, []int{4, 5, 6}) || m.maxScrollOffset() != 0 {
				t.Fatalf("explicit column reset did not discard width history: %v", m.colWidths)
			}
		})
	}
}

func TestTableRetainedWidthsMatchExactMeasurement(t *testing.T) {
	for name, cell := range map[string]string{
		"short ASCII":   "job",
		"long ASCII":    strings.Repeat("x", 100),
		"CJK":           strings.Repeat("界", 20),
		"combining":     strings.Repeat("e\u0301", 30),
		"emoji":         strings.Repeat("👩‍💻1️⃣", 10),
		"ANSI":          "\x1b[1;38;2;100;200;255mjob\x1b[0m",
		"hyperlink":     "\x1b]8;;https://example.com\x1b\\job\x1b]8;;\x1b\\",
		"whitespace":    "job\r\nname\targs\vnext\flast",
		"invalid UTF-8": "job\xff\xc0\xaf",
		"zero width":    strings.Repeat("\u200b\u0301", 30),
		"empty":         "",
		"control bytes": "\x00\x01\x1b[31mjob\x1b[0m\x7f",
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestTable(WithColumns([]Column{{Title: "Value", Width: 16}}), WithWidth(24), WithHeight(4))
			want := 16
			for _, value := range []string{cell, "short", cell + cell, ""} {
				m.SetRows([]Row{row("row", value)})
				want = max(want, lipgloss.Width(singleLine(value)))
				if m.colWidths[0] != want {
					t.Fatalf("measured width=%d, want %d for %q", m.colWidths[0], want, value)
				}
			}
		})
	}
}

func FuzzTableRetainedWidths(f *testing.F) {
	for _, cell := range []string{"job", "界e\u0301", "1️⃣👩‍💻", "\x1b[31mjob\x1b[0m", "a\r\nb\tc", "\xff\x9b0m"} {
		f.Add(cell, "short", uint8(16))
	}
	f.Fuzz(func(t *testing.T, first, next string, minimum uint8) {
		m := newTestTable(WithColumns([]Column{{Title: "Value", Width: int(minimum)}}), WithWidth(24), WithHeight(4))
		want := int(minimum)
		for _, cell := range []string{first, next} {
			m.SetRows([]Row{row("row", cell)})
			want = max(want, lipgloss.Width(singleLine(cell)))
			if m.colWidths[0] != want {
				t.Fatalf("measured width=%d, want %d for %q", m.colWidths[0], want, cell)
			}
		}
	})
}

func TestGoldenTableRetainedWidths(t *testing.T) {
	for _, width := range []int{12, 36} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			columns := []Column{{Title: "First", Width: 8}, {Title: "Middle", Width: 8, Align: AlignRight}, {Title: "Last", Width: 8, Align: AlignCenter}}
			m := newTestTable(WithColumns(columns), WithWidth(width), WithHeight(6))
			rows := []Row{row("wide", strings.Repeat("x", 64), "old", "tail"), row("selected", "界e\u0301", "bb", "TAIL"), row("full")}
			m.SetRowsWithMeta(rows, map[int]string{2: "\x1b[3mfull override\x1b[0m"}, nil)
			m.SetCursor(1)
			m.ScrollToEnd()
			frames := make([]string, 0, 4)
			frames = append(frames, m.View())
			m.SetRowsWithMeta(rows[1:], map[int]string{1: "\x1b[3mfull override\x1b[0m"}, map[int]SelectionSpan{0: {Start: 65, End: -1}})
			frames = append(frames, m.View())
			m.ScrollToStart()
			frames = append(frames, m.View())
			m.SetColumns(columns)
			frames = append(frames, m.View())
			golden.RequireEqual(t, []byte(ansi.Strip(strings.Join(frames, "\n\n"))))
		})
	}
}
