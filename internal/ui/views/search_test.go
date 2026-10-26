package views

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

type searchView interface {
	View
	CancelRequests()
}

type searchHarness struct {
	view searchView
	list *detailListView
}

func searchViews() map[string]func() searchHarness {
	return map[string]func() searchHarness{
		"queues": func() searchHarness {
			v := NewQueueDetails(nil)
			return searchHarness{v, &v.detailListView}
		},
		"retries": func() searchHarness {
			v := NewRetries(nil)
			return searchHarness{v, &v.detailListView}
		},
		"scheduled": func() searchHarness {
			v := NewScheduled(nil)
			return searchHarness{v, &v.detailListView}
		},
		"dead": func() searchHarness {
			v := NewDead(nil)
			return searchHarness{v, &v.detailListView}
		},
		"error details": func() searchHarness {
			v := NewErrorsDetails(nil)
			return searchHarness{v, &v.detailListView}
		},
	}
}

func TestSearchViewsRemainResponsive(t *testing.T) {
	for name, build := range searchViews() {
		t.Run(name, func(t *testing.T) {
			h := build()
			h.view.SetSize(100, 12)
			h.view.SetStyles(Styles{})
			h.list.ready = true
			setupLazyRows(&h.list.lazy)
			started := make(chan context.Context, 1)
			h.list.lazy.SetFetcher(func(ctx context.Context, _, _ int, _ lazytable.CursorIntent) (lazytable.FetchResult, error) {
				started <- ctx
				<-ctx.Done()
				return lazytable.FetchResult{}, ctx.Err()
			})
			t.Cleanup(h.view.CancelRequests)
			_, cmd := h.view.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "needle"})
			batch := cmd().(tea.BatchMsg)
			done := make(chan tea.Msg, 1)
			go func() { done <- batch[1]() }()
			var ctx context.Context
			select {
			case ctx = <-started:
			case <-time.After(time.Second):
				t.Fatal("search did not start")
			}
			before := h.view.View()
			if !strings.Contains(ansi.Strip(before), "Searching...") {
				t.Fatal("search status is missing")
			}
			_, tick := h.view.Update(batch[0]())
			if tick == nil || before == h.view.View() {
				t.Fatal("search spinner did not animate")
			}
			h.view.Update(tea.KeyPressMsg(tea.Key{Code: 'j'}))
			if h.list.lazy.Table().Cursor() != 1 {
				t.Fatal("navigation did not respond while searching")
			}
			if _, cmd := h.view.Update(RefreshMsg{}); cmd != nil {
				t.Fatal("refresh restarted the active search")
			}
			h.view.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("clearing the filter did not cancel the scan")
			}
			select {
			case msg := <-done:
				if msg != nil {
					t.Fatalf("canceled search returned %T", msg)
				}
			case <-time.After(time.Second):
				t.Fatal("search did not stop")
			}
			h.view.CancelRequests()
			if strings.Contains(h.view.View(), "Searching...") {
				t.Fatal("search status remained after cancellation")
			}
			if _, cmd := h.view.Update(spinner.TickMsg{}); cmd != nil {
				t.Fatal("spinner kept ticking after cancellation")
			}
		})
	}
}

func TestSearchFailurePreservesResults(t *testing.T) {
	v := NewRetries(nil)
	v.ready = true
	setupLazyRows(&v.lazy)
	v.lazy.SetFetcher(func(context.Context, int, int, lazytable.CursorIntent) (lazytable.FetchResult, error) {
		return lazytable.FetchResult{}, nil
	})
	v.lazy.RequestWindow(0, lazytable.CursorStart)
	_, cmd := v.Update(lazytable.DataMsg{RequestID: v.lazy.RequestID(), Err: errors.New("scan failed")})
	if !v.ready || v.lazy.Loading() || len(v.lazy.Table().Rows()) != 20 || cmd == nil {
		t.Fatal("failed scan changed results or left loading active")
	}
}

func TestGoldenSearchStatus(t *testing.T) {
	for name, size := range map[string][2]int{"wide": {120, 12}, "narrow": {45, 8}} {
		for state, ready := range map[string]bool{"initial": false, "existing rows": true} {
			t.Run(name+"/"+state, func(t *testing.T) {
				v := NewRetries(nil)
				v.SetSize(size[0], size[1])
				v.SetStyles(Styles{})
				v.ready = ready
				v.filter = "needle"
				if ready {
					setupLazyRows(&v.lazy)
				}
				v.SetSize(size[0], size[1])
				v.lazy.RequestWindow(0, lazytable.CursorStart)
				defer v.CancelRequests()
				output := ansi.Strip(v.View())
				if !strings.Contains(output, "Searching...") {
					t.Fatal("missing search status")
				}
				if got := len(strings.Split(output, "\n")); got != size[1] {
					t.Fatalf("height = %d, want %d", got, size[1])
				}
				for line := range strings.SplitSeq(output, "\n") {
					if ansi.StringWidth(line) > size[0] {
						t.Fatalf("line exceeds available width: %q", line)
					}
				}
				golden.RequireEqual(t, []byte(output))
			})
		}
	}
}
