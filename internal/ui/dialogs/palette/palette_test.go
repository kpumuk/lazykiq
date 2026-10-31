package palette

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

func testModel() *Model {
	items := []Item{{Label: "Dashboard", ID: 0}, {Label: "Busy", ID: 1}, {Label: "Queue jobs", ID: 2}, {Label: "Retries", ID: 3}, {Label: "Scheduled", ID: 4}, {Label: "Dead", ID: 5}, {Label: "Errors", ID: 6}, {Label: "Metrics", ID: 7}, {Label: "Busy: processes", ID: 8}}
	m := New(items)
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func TestPaletteSearchAndSelection(t *testing.T) {
	m := testModel()
	for range 20 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.selected != 8 || m.offset != 1 {
		t.Fatalf("selection/offset = %d/%d", m.selected, m.offset)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := cmd().(dialogs.CloseDialogMsg).Action.(ActionMsg).ID; got != 4 {
		t.Fatalf("destination = %d", got)
	}
	m.Update(tea.PasteMsg{Content: "bUs"})
	if len(m.matches) != 2 || m.selected != 0 || m.offset != 0 {
		t.Fatalf("search state = %#v", m)
	}
	m.Update(tea.PasteMsg{Content: "nope"})
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(m.View(), "No matches") {
		t.Fatal("empty search navigated or lacked feedback")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd().(dialogs.CloseDialogMsg).Action != nil {
		t.Fatal("escape navigated")
	}
}

func TestPaletteFits(t *testing.T) {
	for w := range 61 {
		for h := range 27 {
			m := testModel()
			for range 8 {
				m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			output := m.View()
			if lipgloss.Width(output) > w || (output != "" && lipgloss.Height(output) > h) {
				t.Fatalf("%dx%d overflow: %q", w, h, output)
			}
			row, col := m.Position()
			if row < 0 || col < 0 {
				t.Fatal("negative origin")
			}
		}
	}
}

func TestGoldenPalette(t *testing.T) {
	for name, tc := range map[string]struct {
		w, h, down int
		query      string
	}{
		"normal": {80, 24, 0, ""}, "five_rows": {80, 20, 0, ""}, "eight_rows_threshold": {80, 21, 0, ""}, "scrolled": {80, 24, 8, ""}, "small": {22, 6, 8, ""}, "no_matches": {80, 24, 0, "xyz"}, "filtered": {80, 24, 0, "BUS"},
	} {
		t.Run(name, func(t *testing.T) {
			m := testModel()
			m.input.Blur()
			m.Update(tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
			m.input.SetValue(tc.query)
			m.filter()
			for range tc.down {
				m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
		})
	}
}

func TestPaletteSortsWithoutChangingCaller(t *testing.T) {
	items := []Item{{Label: "zebra", ID: 1}, {Label: "Alpha", ID: 2}, {Label: "busy", ID: 3}}
	m := New(items)
	if m.matches[0].ID != 2 || m.matches[1].ID != 3 || m.matches[2].ID != 1 {
		t.Fatalf("unexpected order: %#v", m.matches)
	}
	if items[0].ID != 1 {
		t.Fatal("sorting changed caller's items")
	}
}

func TestPaletteBackgroundItemsPreserveSearchAndSelection(t *testing.T) {
	m := testModel()
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selected := m.matches[m.selected]
	m.loading = true
	m.Update(loadedMsg{owner: m, items: []Item{{Label: "Queue: batches", ID: 20, Queue: "batches"}, {Label: "A queue", ID: 21, Queue: "a"}}})
	if m.matches[m.selected] != selected {
		t.Fatal("background completion moved selection")
	}
	for i := 1; i < len(m.matches); i++ {
		if strings.ToLower(m.matches[i-1].Label) > strings.ToLower(m.matches[i].Label) {
			t.Fatal("merged items are unsorted")
		}
	}
	m.Update(tea.PasteMsg{Content: "Queue: bat"})
	if len(m.matches) != 1 {
		t.Fatal("queue name is not searchable")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action := cmd().(dialogs.CloseDialogMsg).Action.(ActionMsg); action.Queue != "batches" {
		t.Fatalf("action=%#v", action)
	}
	// A query entered before discovery completes is applied to arriving queues.
	m = testModel()
	m.Update(tea.PasteMsg{Content: "batches"})
	m.loading = true
	m.Update(loadedMsg{owner: m, items: []Item{{Label: "Queue: batches", ID: 20, Queue: "batches"}}})
	if m.input.Value() != "batches" || len(m.matches) != 1 {
		t.Fatal("background completion lost query")
	}
}

func TestPaletteLoaderCancellationAndStaleResults(t *testing.T) {
	var ctx context.Context
	calls := 0
	loader := func(c context.Context) ([]Item, error) { ctx = c; calls++; return nil, c.Err() }
	m := New([]Item{{Label: "Dashboard"}}, WithLoader(context.Background(), loader))
	batch := m.Init()().(tea.BatchMsg)
	if calls != 0 {
		t.Fatal("Init blocked on loader")
	}
	m.Close()
	reply := batch[1]()
	if calls != 1 || ctx.Err() != context.Canceled {
		t.Fatal("close did not cancel discovery")
	}
	reopened := New([]Item{{Label: "Busy"}}, WithLoader(context.Background(), loader))
	reopened.Init()
	defer func() { reopened.Close() }()
	reopened.Update(reply)
	if !reopened.loading || len(reopened.items) != 1 {
		t.Fatal("stale result affected reopened palette")
	}
	reopened.Update(loadedMsg{owner: reopened, err: errors.New("offline")})
	reopened.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(reopened.View(), "queues unavailable") || !strings.Contains(reopened.View(), "Busy") {
		t.Fatal("queue failure hid navigation or lacked feedback")
	}
}

func TestGoldenPaletteQueueDiscovery(t *testing.T) {
	for _, state := range []string{"loading", "failed", "loaded"} {
		t.Run(state, func(t *testing.T) {
			m := testModel()
			m.input.Blur()
			m.loading = true
			switch state {
			case "failed":
				m.Update(loadedMsg{owner: m, err: errors.New("offline")})
			case "loaded":
				m.Update(loadedMsg{owner: m, items: []Item{{Label: "Queue: batches", ID: 20, Queue: "batches"}}})
				m.input.SetValue("queue")
				m.filter()
			}
			golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
		})
	}
}

func TestPaletteHeightBreakpoints(t *testing.T) {
	m := testModel()
	for range 8 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	for _, tc := range []struct{ height, rows int }{{30, 8}, {21, 8}, {20, 5}, {18, 5}, {8, 5}, {7, 4}, {4, 1}, {3, 0}, {21, 8}} {
		m.Update(tea.WindowSizeMsg{Width: 80, Height: tc.height})
		if m.rows != tc.rows {
			t.Fatalf("height %d: rows=%d, want %d", tc.height, m.rows, tc.rows)
		}
		if m.selected != 8 || (m.rows > 0 && (m.selected < m.offset || m.selected >= m.offset+m.rows)) {
			t.Fatalf("height %d: selection no longer visible", tc.height)
		}
		if tc.rows == 8 && (m.row < 5 || tc.height-m.row-m.height < 5) {
			t.Fatal("eight rows leave insufficient space above or below")
		}
	}
}

func TestPaletteResizePreservesQueryCursor(t *testing.T) {
	for name, atEnd := range map[string]bool{"end": true, "middle": false} {
		t.Run(name, func(t *testing.T) {
			m := testModel()
			query := strings.Repeat("a", 60) + "TAIL"
			m.Update(tea.PasteMsg{Content: query})
			if !atEnd {
				m.input.SetCursor(35)
			}
			position := m.input.Position()
			for _, width := range []int{25, 60, 15, 80} {
				m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
				before := m.View()
				m.input.CursorEnd()
				m.input.SetCursor(position)
				if before != m.View() || m.input.Value() != query || m.input.Position() != position {
					t.Fatalf("width %d: resize left stale input scrolling or changed input state", width)
				}
				if atEnd && !strings.Contains(ansi.Strip(before), "TAIL") {
					t.Fatal("query tail hidden after resize")
				}
			}
		})
	}
}

func TestGoldenPaletteResizedQuery(t *testing.T) {
	m := testModel()
	m.Update(tea.PasteMsg{Content: strings.Repeat("a", 60) + "TAIL"})
	m.Update(tea.WindowSizeMsg{Width: 25, Height: 18})
	m.input.Blur()
	golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
}
