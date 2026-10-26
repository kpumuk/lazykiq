package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type failedSearchClient struct {
	sidekiq.API
	err error
}

func (c failedSearchClient) GetQueues(context.Context) ([]*sidekiq.Queue, error) {
	return nil, c.err
}

func (c failedSearchClient) ScanSortedEntriesWindow(context.Context, sidekiq.SortedSetKind, string, int, int) (sidekiq.SortedEntriesWindow, error) {
	return sidekiq.SortedEntriesWindow{}, c.err
}

func (c failedSearchClient) GetErrorGroupWindow(context.Context, sidekiq.ErrorGroupKey, string, int, int) (sidekiq.ErrorGroupWindow, error) {
	return sidekiq.ErrorGroupWindow{}, c.err
}

func TestAppRejectsDeferredSearchErrors(t *testing.T) {
	for name, id := range map[string]viewID{
		"queues": viewQueueDetails, "retries": viewRetries, "scheduled": viewScheduled,
		"dead": viewDead, "error details": viewErrorsDetails,
	} {
		t.Run(name, func(t *testing.T) {
			otherViewKey := '4'
			if id == viewRetries {
				otherViewKey = '5'
			}
			for name, tc := range map[string]struct {
				action    tea.Msg
				wantError bool
			}{
				"current request": {wantError: true},
				"clear filter":    {action: tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl})},
				"replace filter":  {action: filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "new query"}},
				"refresh":         {action: views.RefreshMsg{}},
				"switch view":     {action: tea.KeyPressMsg(tea.Key{Code: otherViewKey})},
				"open detail":     {action: views.ShowJobDetailMsg{}},
			} {
				for when, produceBeforeAction := range map[string]bool{
					"before error command runs": false,
					"after error command runs":  true,
				} {
					t.Run(name+"/"+when, func(t *testing.T) {
						scanError := errors.New("old scan failed")
						app := New(failedSearchClient{err: scanError}, "", false, nil)
						app.setActiveView(id)
						t.Cleanup(func() {
							for id := range app.viewRegistry {
								app.cancelViewRequests(id)
							}
						})
						updated, start := app.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "old query"})
						app = updated.(App)
						fetch := start().(tea.BatchMsg)[1]
						updated, delivery := app.Update(fetch())
						app = updated.(App)
						if app.connectionError != nil || delivery == nil {
							t.Fatal("expected error to await final delivery")
						}

						var errorMsg tea.Msg
						if produceBeforeAction {
							errorMsg = delivery()
						}
						if tc.action != nil {
							updated, _ = app.Update(tc.action)
							app = updated.(App)
						}
						if !produceBeforeAction {
							errorMsg = delivery()
						}
						updated, _ = app.Update(errorMsg)
						app = updated.(App)
						if got := errors.Is(app.connectionError, scanError); got != tc.wantError {
							t.Fatalf("search error displayed = %v, want %v", got, tc.wantError)
						}
					})
				}
			}
		})
	}
}

type blockedSearchClient struct {
	sidekiq.API
	started chan context.Context
	release chan struct{}
	jobs    []*sidekiq.SortedEntry
}

func (c blockedSearchClient) GetSortedEntries(_ context.Context, kind sidekiq.SortedSetKind, _, _ int) ([]*sidekiq.SortedEntry, int64, error) {
	if kind == sidekiq.SortedSetRetry {
		return c.jobs, int64(len(c.jobs)), nil
	}
	return nil, 0, nil
}

func (c blockedSearchClient) GetSortedEntryBounds(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
	return c.jobs[0], c.jobs[len(c.jobs)-1], nil
}

func (c blockedSearchClient) ScanSortedEntriesWindow(ctx context.Context, _ sidekiq.SortedSetKind, _ string, _, _ int) (sidekiq.SortedEntriesWindow, error) {
	c.started <- ctx
	select {
	case <-ctx.Done():
		return sidekiq.SortedEntriesWindow{}, ctx.Err()
	case <-c.release:
		return sidekiq.SortedEntriesWindow{}, nil
	}
}

type searchProgramModel struct {
	app     App
	initial tea.Cmd
	frames  chan string
}

func (m searchProgramModel) Init() tea.Cmd { return m.initial }

func (m searchProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.app.Update(msg)
	m.app = updated.(App)
	select {
	case m.frames <- ansi.Strip(m.app.View().Content):
	default:
	}
	return m, cmd
}

func (m searchProgramModel) View() tea.View { return m.app.View() }

func TestAppCanSwitchViewsAndQuitDuringSearch(t *testing.T) {
	client := blockedSearchClient{started: make(chan context.Context, 1), release: make(chan struct{})}
	defer close(client.release)
	for range 75 {
		client.jobs = append(client.jobs, sidekiq.NewSortedEntry(
			`{"jid":"wide","class":"WideJob","queue":"default","args":["`+strings.Repeat("x", 64*1024)+`"]}`, 1))
	}
	app := New(client, "", false, nil)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app = updated.(App)
	initial := app.setActiveView(viewRetries)
	batch := initial().(tea.BatchMsg)
	updated, _ = app.Update(batch[1]())
	app = updated.(App)
	updated, cmd := app.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "needle"})
	frames := make(chan string, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := tea.NewProgram(searchProgramModel{app: updated.(App), initial: cmd, frames: frames},
		tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(),
		tea.WithWindowSize(120, 30), tea.WithContext(ctx), tea.WithoutSignalHandler())
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()
	defer p.Kill()

	var scanContext context.Context
	select {
	case scanContext = <-client.started:
	case err := <-done:
		t.Fatalf("program exited before searching: %v", err)
	case <-ctx.Done():
		t.Fatal("search did not start")
	}

	// Observe two different rendered frames while the Redis request is blocked.
	first := ""
framesLoop:
	for {
		select {
		case frame := <-frames:
			if !strings.Contains(frame, "Searching...") {
				continue
			}
			if first != "" && first != frame {
				break framesLoop
			}
			first = frame
		case <-ctx.Done():
			t.Fatal("search indicator did not animate while the request was blocked")
		}
	}

	p.Send(tea.KeyPressMsg(tea.Key{Code: '5'}))
	select {
	case <-scanContext.Done():
	case <-ctx.Done():
		t.Fatal("switching views did not cancel the active search")
	}
	p.Send(tea.KeyPressMsg(tea.Key{Code: 'q'}))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("quit did not respond")
	}
}
