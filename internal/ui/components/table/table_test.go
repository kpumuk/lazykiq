package table

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func blankStyles() Styles {
	return Styles{
		Text:      lipgloss.NewStyle(),
		Muted:     lipgloss.NewStyle(),
		Header:    lipgloss.NewStyle(),
		Selected:  lipgloss.NewStyle(),
		Separator: lipgloss.NewStyle(),
	}
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
			{"foo", "this-is-long"},
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
			{"123456", "short"},
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
			{"one", "twoooooo"},
			{"three", "four"},
			{"five", "six"},
			{"seven", "eight"},
			{"nine", "ten"},
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
			msg:        tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
			wantCursor: 1,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "LineUp",
			msg:        tea.KeyMsg{Type: tea.KeyUp},
			setup:      func(m *Model) { m.cursor = 2 },
			wantCursor: 1,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "PageDownClamp",
			msg:        tea.KeyMsg{Type: tea.KeyPgDown},
			wantCursor: 4,
			wantX:      0,
			wantY:      3,
		},
		{
			name:       "GotoBottom",
			msg:        tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")},
			wantCursor: 4,
			wantX:      0,
			wantY:      3,
		},
		{
			name:       "GotoTop",
			msg:        tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")},
			setup:      func(m *Model) { m.cursor = 3; m.yOffset = 2 },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "ScrollRight",
			msg:        tea.KeyMsg{Type: tea.KeyRight},
			wantCursor: 0,
			wantX:      4,
			wantY:      0,
		},
		{
			name:       "ScrollLeftClamp",
			msg:        tea.KeyMsg{Type: tea.KeyLeft},
			setup:      func(m *Model) { m.xOffset = 2 },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "Home",
			msg:        tea.KeyMsg{Type: tea.KeyHome},
			setup:      func(m *Model) { m.xOffset = 4 },
			wantCursor: 0,
			wantX:      0,
			wantY:      0,
		},
		{
			name:       "End",
			msg:        tea.KeyMsg{Type: tea.KeyEnd},
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
		want           string
	}{
		{
			name:           "Empty",
			content:        "",
			yOffset:        0,
			viewportHeight: 2,
			want:           "",
		},
		{
			name:           "ClampOffset",
			content:        "a\nb\nc",
			yOffset:        5,
			viewportHeight: 1,
			want:           "c",
		},
		{
			name:           "SliceWindow",
			content:        "a\nb\nc",
			yOffset:        1,
			viewportHeight: 2,
			want:           "b\nc",
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
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSelectionKeepsVisible(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 1}}),
		WithRows([]Row{
			{"1"},
			{"2"},
			{"3"},
			{"4"},
			{"5"},
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
			wantX:      9,
			wantY:      0,
		},
	}

	base := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			{"one", "twoooooo"},
			{"three", "four"},
			{"five", "six"},
			{"seven", "eight"},
			{"nine", "ten"},
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
			{"a", "bbbbbbbbbb"},
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
			{"1"},
			{"2"},
			{"3"},
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.cursor = 2
	table.SetRows([]Row{{"1"}})

	if table.cursor != 0 {
		t.Fatalf("want cursor 0, got %d", table.cursor)
	}
}

func TestSetStyles_RefreshesContent(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 1}}),
		WithRows([]Row{{"1"}}),
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
			{"a", "long-value"},
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
		{"a", "b"},
	})

	if table.xOffset != 0 {
		t.Fatalf("want xOffset 0 after rows shrink, got %d", table.xOffset)
	}
}

func TestSetColumns_ClampsHorizontalScroll(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 2},
			{Title: "B", Width: 10},
		}),
		WithRows([]Row{
			{"a", "bbbbbbbbbb"},
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
		{"a", "b"},
	})

	if table.xOffset != 0 {
		t.Fatalf("want xOffset 0 after columns shrink, got %d", table.xOffset)
	}
}

func TestView_BasicSnapshot(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			{"one", "two"},
			{"three", "four"},
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	separator := strings.Repeat("\u2500", 10)
	want := strings.Join([]string{
		"A     B   ",
		separator,
		"one   two ",
		"three four",
	}, "\n")

	got := table.View()
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestView_HorizontalScrollSnapshot(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			{"one", "two"},
			{"three", "four"},
		}),
		WithStyles(blankStyles()),
		WithWidth(5),
		WithHeight(4),
	)

	table.ScrollRight()

	separator := strings.Repeat("\u2500", 5)
	want := strings.Join([]string{
		"  B  ",
		separator,
		"  two",
		"e fou",
	}, "\n")

	got := table.View()
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestView_VerticalScrollSnapshot(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			{"one", "two"},
			{"three", "four"},
			{"five", "six"},
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.MoveDown(2)

	separator := strings.Repeat("\u2500", 10)
	want := strings.Join([]string{
		"A     B   ",
		separator,
		"three four",
		"five  six ",
	}, "\n")

	got := table.View()
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestView_LastColumnVariableWidthSnapshot(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			{"one", "this-is-long"},
		}),
		WithStyles(blankStyles()),
		WithWidth(20),
		WithHeight(3),
	)

	separator := strings.Repeat("\u2500", 16) + strings.Repeat(" ", 4)
	want := strings.Join([]string{
		"A   B               ",
		separator,
		"one this-is-long    ",
	}, "\n")

	got := table.View()
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestView_SetSizeSnapshot(t *testing.T) {
	table := New(
		WithColumns([]Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		WithRows([]Row{
			{"one", "two"},
			{"three", "four"},
		}),
		WithStyles(blankStyles()),
		WithWidth(10),
		WithHeight(4),
	)

	table.SetSize(6, 4)

	separator := strings.Repeat("\u2500", 6)
	want := strings.Join([]string{
		"A     ",
		separator,
		"one   ",
		"three ",
	}, "\n")

	got := table.View()
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestView_SetColumnsSnapshot(t *testing.T) {
	table := New(
		WithColumns([]Column{{Title: "A", Width: 2}}),
		WithRows([]Row{{"x"}}),
		WithStyles(blankStyles()),
		WithWidth(8),
		WithHeight(3),
	)

	table.SetColumns([]Column{
		{Title: "ID", Width: 2},
		{Title: "Name", Width: 4},
	})
	table.SetRows([]Row{{"1", "Bob"}})

	separator := strings.Repeat("\u2500", 6) + "  "
	want := strings.Join([]string{
		"ID Name ",
		separator,
		"1  Bob  ",
	}, "\n")

	got := table.View()
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
