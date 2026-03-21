package views

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

func TestFilteredViews_PageAliasStillPages(t *testing.T) {
	type harness struct {
		view   View
		cursor func(View) int
	}

	cases := map[string]func() harness{
		"queue details": func() harness {
			view := NewQueueDetails(nil)
			view.filter = "match"
			setupLazyRows(&view.lazy)
			return harness{
				view: view,
				cursor: func(v View) int {
					return v.(*QueueDetails).lazy.Table().Cursor()
				},
			}
		},
		"dead": func() harness {
			view := NewDead(nil)
			view.filter = "match"
			setupLazyRows(&view.lazy)
			return harness{
				view: view,
				cursor: func(v View) int {
					return v.(*Dead).lazy.Table().Cursor()
				},
			}
		},
		"retries": func() harness {
			view := NewRetries(nil)
			view.filter = "match"
			setupLazyRows(&view.lazy)
			return harness{
				view: view,
				cursor: func(v View) int {
					return v.(*Retries).lazy.Table().Cursor()
				},
			}
		},
		"scheduled": func() harness {
			view := NewScheduled(nil)
			view.filter = "match"
			setupLazyRows(&view.lazy)
			return harness{
				view: view,
				cursor: func(v View) int {
					return v.(*Scheduled).lazy.Table().Cursor()
				},
			}
		},
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			h := build()

			updated, _ := h.view.Update(tea.KeyPressMsg(tea.Key{Code: ']'}))
			if got := h.cursor(updated); got <= 0 {
				t.Fatalf("cursor after ] = %d, want > 0", got)
			}
		})
	}
}

func setupLazyRows(lazy *lazytable.Model) {
	lazy.SetSize(80, 10)

	rows := make([]table.Row, 20)
	for i := range rows {
		rows[i] = table.Row{
			ID:    fmt.Sprintf("row-%02d", i),
			Cells: []string{fmt.Sprintf("row %02d", i)},
		}
	}

	lazy.Table().SetRows(rows)
	lazy.Table().SetCursor(0)
}
