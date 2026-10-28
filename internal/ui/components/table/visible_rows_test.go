package table

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTableViewportCacheBounded(t *testing.T) {
	for _, n := range []int{75, 1000, 10000, 100000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			m := cardinalityTable(cardinalityRows(n))
			for _, cursor := range []int{0, 1, 23, n - 1, n / 2, 0} {
				m.SetCursor(cursor)
				if cap(m.rowContent) > m.ViewportHeight() || cap(m.content) > m.ViewportHeight() {
					t.Fatalf("cached %d raw and %d styled rows with only %d visible", cap(m.rowContent), cap(m.content), m.ViewportHeight())
				}
				assertSingleLineTableSize(t, m)
			}
		})
	}
}

func TestTableUpdatePreservesPreviousView(t *testing.T) {
	m := cardinalityTable(cardinalityRows(75))
	m.SetCursor(21)
	before := m.View()
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
	if m.View() != before {
		t.Fatal("updating the copy changed the previous model's view")
	}
	if next.Cursor() != 22 || next.YOffset() != 1 {
		t.Fatal("copied model did not scroll to the next row")
	}
	nextView := next.View()
	if next.View() != nextView {
		t.Fatal("rendering changed the copied model's cache")
	}
}

func TestTableStylesOnlyVisibleRows(t *testing.T) {
	for name, action := range map[string]func(*Model){
		"cursor":          func(m *Model) { m.SetCursor(1) },
		"page":            func(m *Model) { m.MoveDown(10) },
		"bottom":          (*Model).GotoBottom,
		"vertical scroll": func(m *Model) { m.SetYOffset(50) },
		"horizontal":      (*Model).ScrollToEnd,
		"styles":          func(m *Model) { m.SetStyles(m.styles) },
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			styles := blankStyles()
			styles.Text = styles.Text.Transform(func(s string) string {
				calls++
				return s
			})
			m := cardinalityTable(cardinalityRows(1000))
			m.SetStyles(styles)
			calls = 0
			action(&m)
			if calls > m.ViewportHeight() {
				t.Fatalf("styled %d rows with only %d visible", calls, m.ViewportHeight())
			}
			calls = 0
			first := m.View()
			if second := m.View(); first != second {
				t.Fatal("repeated View changed the cached output")
			}
			if calls != 0 {
				t.Fatalf("unchanged View styled %d rows", calls)
			}
		})
	}
}

func TestTableOffscreenMetadata(t *testing.T) {
	for name, tc := range map[string]struct {
		cells []string
		full  map[int]string
	}{
		"column":      {cells: []string{"job", strings.Repeat("x", 512) + "TAIL"}},
		"full row":    {full: map[int]string{99: "\x1b[3m" + strings.Repeat("界", 512) + "TAIL\x1b[0m"}},
		"extra cells": {cells: []string{"job", "value", strings.Repeat("x", 512) + "TAIL"}},
	} {
		t.Run(name, func(t *testing.T) {
			rows := make([]Row, 100)
			for i := range rows {
				rows[i] = row(strconv.Itoa(i), "job", "value")
			}
			rows[99].Cells = tc.cells
			m := newTestTable(WithColumns([]Column{{Title: "Job", Width: 8}, {Title: "Value", Width: 12}}), WithWidth(24), WithHeight(5))
			m.SetRowsWithMeta(rows, tc.full, nil)
			if m.maxScrollOffset() < 500 {
				t.Fatal("offscreen metadata did not contribute to the horizontal range")
			}
			if strings.Contains(m.View(), "TAIL") {
				t.Fatal("offscreen row is visible")
			}
			m.GotoBottom()
			m.ScrollToEnd()
			if !strings.Contains(ansi.Strip(m.View()), "TAIL") {
				t.Fatal("cannot inspect offscreen metadata after scrolling")
			}
			assertSingleLineTableSize(t, m)
			assertANSIGrounded(t, m.View())
			m.SetSize(30, 10)
			m.ScrollToEnd()
			if !strings.Contains(ansi.Strip(m.View()), "TAIL") {
				t.Fatal("resizing lost the end of the metadata")
			}
			assertSingleLineTableSize(t, m)
		})
	}
}

func TestTableSelectionAcrossViewport(t *testing.T) {
	styles := blankStyles()
	styles.Selected = lipgloss.NewStyle().Reverse(true)
	m := New(WithColumns([]Column{{Title: "Value", Width: 20}}), WithStyles(styles), WithWidth(12), WithHeight(4))
	rows := make([]Row, 100)
	for i := range rows {
		rows[i] = row(strconv.Itoa(i), fmt.Sprintf("row%02d", i))
	}
	m.SetRowsWithMeta(rows, map[int]string{50: "\x1b[3mfull row50 tail\x1b[0m"}, map[int]SelectionSpan{50: {Start: 5, End: -1}})
	m.SetCursor(50)
	if !strings.Contains(m.View(), "\x1b[7mrow50") {
		t.Fatalf("selection span missing after scrolling into view: %q", m.View())
	}
	m.MoveDown(1)
	if !strings.Contains(m.View(), "\x1b[3mfull row50") || !strings.Contains(m.View(), "\x1b[7mrow51") {
		t.Fatalf("moving selection lost full-row or selected style: %q", m.View())
	}
	m.ScrollToEnd()
	assertSingleLineTableSize(t, m)
	assertANSIGrounded(t, m.View())
}
