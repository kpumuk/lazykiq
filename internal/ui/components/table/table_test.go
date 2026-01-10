package table

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
	"github.com/charmbracelet/x/exp/golden"
)

func blankStyles() Styles {
	return Styles{
		Text:           lipgloss.NewStyle(),
		Muted:          lipgloss.NewStyle(),
		Header:         lipgloss.NewStyle(),
		Selected:       lipgloss.NewStyle(),
		Separator:      lipgloss.NewStyle(),
		ScrollbarTrack: lipgloss.NewStyle(),
		ScrollbarThumb: lipgloss.NewStyle(),
	}
}

func row(id string, cells ...string) Row {
	return Row{ID: id, Cells: cells}
}

func TestApplyHorizontalScroll(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		offset       int
		visibleWidth int
		want         string
	}{
		{
			name:         "NoOffset",
			line:         "abcdef",
			offset:       0,
			visibleWidth: 4,
			want:         "abcd",
		},
		{
			name:         "OffsetWithinLine",
			line:         "abcdef",
			offset:       2,
			visibleWidth: 4,
			want:         "cdef",
		},
		{
			name:         "OffsetBeyondLine",
			line:         "abcdef",
			offset:       6,
			visibleWidth: 4,
			want:         "    ",
		},
		{
			name:         "PadWhenShort",
			line:         "abcdef",
			offset:       4,
			visibleWidth: 6,
			want:         "ef    ",
		},
		{
			name:         "WideRuneOffset",
			line:         "a\u754cb",
			offset:       1,
			visibleWidth: 3,
			want:         "\u754cb",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyHorizontalScroll(tc.line, tc.offset, tc.visibleWidth)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestApplyHorizontalScroll_ANSIIntegrity(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	line := style.Render("abcdef")

	got := applyHorizontalScroll(line, 2, 3)
	if len(got) <= 3 {
		t.Fatalf("expected ANSI codes to be preserved, got %q", got)
	}
	if lipgloss.Width(got) != 3 {
		t.Fatalf("want width 3, got %d", lipgloss.Width(got))
	}

	assertANSIGrounded(t, got)
}

func TestRenderBody_EmptyMessage(t *testing.T) {
	table := New(
		WithStyles(blankStyles()),
		WithEmptyMessage("Nothing here"),
	)

	got := table.renderBody()
	if got != "Nothing here" {
		t.Fatalf("want %q, got %q", "Nothing here", got)
	}
}

func assertANSIGrounded(t *testing.T, s string) {
	t.Helper()

	p := ansi.NewParser()
	for i := range len(s) {
		p.Advance(s[i])
	}
	if p.State() != parser.GroundState {
		t.Fatalf("expected ANSI parser in ground state, got %s", p.StateName())
	}
}

func TestSetEmptyMessage(t *testing.T) {
	table := New(
		WithStyles(blankStyles()),
	)
	table.SetEmptyMessage("Nada")

	got := table.renderBody()
	if got != "Nada" {
		t.Fatalf("want %q, got %q", "Nada", got)
	}
}

func TestRenderBody_LastColumnNotTruncated(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "foo", "this-is-long"),
		}),
		WithStyles(blankStyles()),
		WithWidth(80),
	)

	got := table.renderBody()
	got = strings.TrimRight(got, " ")
	want := "foo this-is-long"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestRenderHeader_UsesDynamicWidth(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "ID", Width: 2},
			{Title: "Value", Width: 5},
		}),
		WithRows([]Row{
			row("row-1", "123456", "short"),
		}),
		WithStyles(blankStyles()),
		WithWidth(80),
	)

	header := table.renderHeader()
	lines := strings.Split(header, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header and separator, got %q", header)
	}

	want := "ID     Value"
	got := strings.TrimRight(lines[0], " ")
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestUpdate_KeyHandling(t *testing.T) {
	base := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "one", "twoooooo"),
			row("row-2", "three", "four"),
			row("row-3", "five", "six"),
			row("row-4", "seven", "eight"),
			row("row-5", "nine", "ten"),
		}),
		WithStyles(blankStyles()),
		WithWidth(5),
		WithHeight(4),
	)

	tests := []struct {
		name       string
		msg        tea.KeyMsg
		setup      func(*Model)
		wantCursor int
		wantX      int
		wantY      int
	}{
		{
			name:       "LineDown",
			msg:        tea.KeyPressMsg{Code: 'j'},
			wantCursor: 1,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "LineUp",
			msg:        tea.KeyPressMsg{Code: tea.KeyUp},
			setup:      func(m *Model) { m.cursor = 2 },
			wantCursor: 1,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "PageDownClamp",
			msg:        tea.KeyPressMsg{Code: tea.KeyPgDown},
			wantCursor: 4,
			wantX:      0,
			wantY:      3,
		},
		{
			name:       "GotoBottom",
			msg:        tea.KeyPressMsg{Code: 'G'},
			wantCursor: 4,
			wantX:      0,
			wantY:      3,
		},
		{
			name:       "GotoTop",
			msg:        tea.KeyPressMsg{Code: 'g'},
			setup:      func(m *Model) { m.cursor = 3; m.yOffset = 2 },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "ScrollRight",
			msg:        tea.KeyPressMsg{Code: tea.KeyRight},
			wantCursor: 0,
			wantX:      4,
			wantY:      0,
		},
		{
			name:       "ScrollLeftClamp",
			msg:        tea.KeyPressMsg{Code: tea.KeyLeft},
			setup:      func(m *Model) { m.xOffset = 2 },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "Home",
			msg:        tea.KeyPressMsg{Code: tea.KeyHome},
			setup:      func(m *Model) { m.xOffset = 4 },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "End",
			msg:        tea.KeyPressMsg{Code: tea.KeyEnd},
			wantCursor: 0,
			wantX:      base.maxScrollOffset(),
			wantY:      0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			table := base
			if tc.setup != nil {
				tc.setup(&table)
			}

			table, _ = table.Update(tc.msg)

			if table.cursor != tc.wantCursor {
				t.Fatalf("want cursor %d, got %d", tc.wantCursor, table.cursor)
			}
			if table.xOffset != tc.wantX {
				t.Fatalf("want xOffset %d, got %d", tc.wantX, table.xOffset)
			}
			if table.yOffset != tc.wantY {
				t.Fatalf("want yOffset %d, got %d", tc.wantY, table.yOffset)
			}
		})
	}
}

func TestGetVisibleContent(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		yOffset        int
		viewportHeight int
		want           []string
	}{
		{
			name:           "Empty",
			content:        "",
			yOffset:        0,
			viewportHeight: 2,
			want:           []string{"", ""},
		},
		{
			name:           "ClampOffset",
			content:        "a\nb\nc",
			yOffset:        5,
			viewportHeight: 1,
			want:           []string{"c"},
		},
		{
			name:           "SliceWindow",
			content:        "a\nb\nc",
			yOffset:        1,
			viewportHeight: 2,
			want:           []string{"b", "c"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			table := Model{
				content:        tc.content,
				yOffset:        tc.yOffset,
				viewportHeight: tc.viewportHeight,
			}
			got := table.getVisibleContent()
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSelectionKeepsVisible(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 1}}),
		WithRows([]Row{
			row("row-1", "1"),
			row("row-2", "2"),
			row("row-3", "3"),
			row("row-4", "4"),
			row("row-5", "5"),
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.MoveDown(3)

	if table.cursor != 3 {
		t.Fatalf("want cursor 3, got %d", table.cursor)
	}
	if table.yOffset != 2 {
		t.Fatalf("want yOffset 2, got %d", table.yOffset)
	}
}

func TestNavigationMethods(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*Model)
		action     func(*Model)
		wantCursor int
		wantX      int
		wantY      int
	}{
		{
			name:       "GotoTop",
			setup:      func(m *Model) { m.cursor = 4; m.yOffset = 3; m.xOffset = 2 },
			action:     func(m *Model) { m.GotoTop() },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "GotoBottom",
			action:     func(m *Model) { m.GotoBottom() },
			wantCursor: 4,
			wantX:      0,
			wantY:      3,
		},
		{
			name:       "ScrollToStart",
			setup:      func(m *Model) { m.xOffset = 6 },
			action:     func(m *Model) { m.ScrollToStart() },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "ScrollToEnd",
			action:     func(m *Model) { m.ScrollToEnd() },
			wantCursor: 0,
			wantX:      10,
			wantY:      0,
		},
	}

	base := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "one", "twoooooo"),
			row("row-2", "three", "four"),
			row("row-3", "five", "six"),
			row("row-4", "seven", "eight"),
			row("row-5", "nine", "ten"),
		}),
		WithStyles(blankStyles()),
		WithWidth(5),
		WithHeight(4),
	)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			table := base
			if tc.setup != nil {
				tc.setup(&table)
			}

			tc.action(&table)

			if table.cursor != tc.wantCursor {
				t.Fatalf("want cursor %d, got %d", tc.wantCursor, table.cursor)
			}
			if table.xOffset != tc.wantX {
				t.Fatalf("want xOffset %d, got %d", tc.wantX, table.xOffset)
			}
			if table.yOffset != tc.wantY {
				t.Fatalf("want yOffset %d, got %d", tc.wantY, table.yOffset)
			}
		})
	}
}

func TestScrollRightClamps(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 1},
			{Title: "B", Width: 1},
		}),
		WithRows([]Row{
			row("row-1", "a", "bbbbbbbbbb"),
		}),
		WithStyles(blankStyles()),
		WithWidth(5),
	)

	table.ScrollRight()
	table.ScrollRight()

	if table.xOffset != table.maxScrollOffset() {
		t.Fatalf("want xOffset %d, got %d", table.maxScrollOffset(), table.xOffset)
	}
}

func TestSetRows_ClampsCursor(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 1}}),
		WithRows([]Row{
			row("row-1", "1"),
			row("row-2", "2"),
			row("row-3", "3"),
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.cursor = 2
	table.SetRows([]Row{row("row-1", "1")})

	if table.cursor != 0 {
		t.Fatalf("want cursor 0, got %d", table.cursor)
	}
}

func TestSetRows_PreservesSelectionByID(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 1}}),
		WithRows([]Row{
			row("row-a", "a"),
			row("row-b", "b"),
			row("row-c", "c"),
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.SetCursor(0)
	table.SetRows([]Row{
		row("row-b", "b"),
		row("row-c", "c"),
		row("row-a", "a"),
	})

	if table.cursor != 2 {
		t.Fatalf("want cursor 2, got %d", table.cursor)
	}
	if table.yOffset != 1 {
		t.Fatalf("want yOffset 1, got %d", table.yOffset)
	}
}

func TestSetStyles_RefreshesContent(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 1}}),
		WithRows([]Row{row("row-1", "1")}),
		WithStyles(blankStyles()),
		WithWidth(4),
		WithHeight(3),
	)

	got := table.View()
	if !strings.Contains(got, "1") {
		t.Fatalf("expected content before style update")
	}

	table.SetStyles(DefaultStyles())

	got = table.View()
	if !strings.Contains(got, "1") {
		t.Fatalf("expected content after style update")
	}
}

func TestSetRows_ClampsHorizontalScroll(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 2},
			{Title: "B", Width: 2},
		}),
		WithRows([]Row{
			row("row-1", "a", "long-value"),
		}),
		WithStyles(blankStyles()),
		WithWidth(4),
		WithHeight(3),
	)

	table.ScrollToEnd()
	if table.xOffset == 0 {
		t.Fatalf("expected horizontal scroll before rows update")
	}

	table.SetRows([]Row{
		row("row-1", "a", "b"),
	})

	if table.xOffset != table.maxScrollOffset() {
		t.Fatalf("want xOffset %d after rows shrink, got %d", table.maxScrollOffset(), table.xOffset)
	}
}

func TestSetColumns_ClampsHorizontalScroll(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 2},
			{Title: "B", Width: 10},
		}),
		WithRows([]Row{
			row("row-1", "a", "bbbbbbbbbb"),
		}),
		WithStyles(blankStyles()),
		WithWidth(4),
		WithHeight(3),
	)

	table.ScrollToEnd()
	if table.xOffset == 0 {
		t.Fatalf("expected horizontal scroll before columns update")
	}

	table.SetColumns([]Column{
		{Title: "A", Width: 2},
		{Title: "B", Width: 2},
	})
	table.SetRows([]Row{
		row("row-1", "a", "b"),
	})

	if table.xOffset != table.maxScrollOffset() {
		t.Fatalf("want xOffset %d after columns shrink, got %d", table.maxScrollOffset(), table.xOffset)
	}
}

// Golden File Tests - These catch visual regressions (misalignment, spacing, etc.)
// Run with GOLDEN_UPDATE=1 to regenerate golden files after intentional changes.

func TestGoldenBasic(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "one", "two"),
			row("row-2", "three", "four"),
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	output := ansi.Strip(table.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenHorizontalScroll(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "one", "two"),
			row("row-2", "three", "four"),
		}),
		WithStyles(blankStyles()),
		WithWidth(5),
		WithHeight(4),
	)

	table.ScrollRight()

	output := ansi.Strip(table.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenVerticalScroll(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "one", "two"),
			row("row-2", "three", "four"),
			row("row-3", "five", "six"),
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.MoveDown(2)

	output := ansi.Strip(table.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenLastColumnVariableWidth(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "one", "this-is-long"),
		}),
		WithStyles(blankStyles()),
		WithWidth(20),
		WithHeight(3),
	)

	output := ansi.Strip(table.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenSetSize(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			row("row-1", "one", "two"),
			row("row-2", "three", "four"),
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.SetSize(6, 4)

	output := ansi.Strip(table.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenSetColumns(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 2}}),
		WithRows([]Row{row("row-1", "x")}),
		WithStyles(blankStyles()),
		WithWidth(8),
		WithHeight(3),
	)

	table.SetColumns([]Column{
		{Title: "ID", Width: 2},
		{Title: "Name", Width: 4},
	})
	table.SetRows([]Row{row("row-1", "1", "Bob")})

	output := ansi.Strip(table.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenWidths(t *testing.T) {
	for _, width := range []int{6, 10, 20} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			table := New(
				WithColumns([]Column{
					{Title: "A", Width: 3},
					{Title: "B", Width: 3},
				}),
				WithRows([]Row{
					row("row-1", "one", "two"),
					row("row-2", "three", "four"),
				}),
				WithStyles(blankStyles()),
				WithWidth(width),
				WithHeight(4),
			)

			output := ansi.Strip(table.View())
			golden.RequireEqual(t, []byte(output))
		})
	}
}

func TestGoldenHeights(t *testing.T) {
	for _, height := range []int{3, 4, 6} {
		t.Run(fmt.Sprintf("height %d", height), func(t *testing.T) {
			table := New(
				WithColumns([]Column{
					{Title: "A", Width: 3},
					{Title: "B", Width: 3},
				}),
				WithRows([]Row{
					row("row-1", "one", "two"),
					row("row-2", "three", "four"),
					row("row-3", "five", "six"),
				}),
				WithStyles(blankStyles()),
				WithWidth(10),
				WithHeight(height),
			)

			output := ansi.Strip(table.View())
			golden.RequireEqual(t, []byte(output))
		})
	}
}
