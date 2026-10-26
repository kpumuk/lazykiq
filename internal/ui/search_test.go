package ui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

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
