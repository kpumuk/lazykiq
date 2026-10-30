package ui

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

type lifetimeRequest struct {
	name string
	ctx  context.Context
}

type lifetimeClient struct {
	sidekiq.API
	started  chan lifetimeRequest
	finished chan string
	release  chan struct{}
}

func (c *lifetimeClient) DisplayRedisURL() string { return "redis://localhost:6379/0" }

func (c *lifetimeClient) GetStats(ctx context.Context) (sidekiq.Stats, error) {
	return sidekiq.Stats{}, c.block(ctx, "stats")
}

func (c *lifetimeClient) GetRedisInfo(ctx context.Context) (sidekiq.RedisInfo, error) {
	return sidekiq.RedisInfo{}, c.block(ctx, "redis info")
}

func (c *lifetimeClient) GetStatsHistory(ctx context.Context, _ int) (sidekiq.StatsHistory, error) {
	return sidekiq.StatsHistory{}, c.block(ctx, "history")
}

func (c *lifetimeClient) DeleteAllSortedEntries(ctx context.Context, _ sidekiq.SortedSetKind) error {
	return c.block(ctx, "mutation")
}

func (c *lifetimeClient) GetSortedEntries(ctx context.Context, _ sidekiq.SortedSetKind, _, _ int) ([]*sidekiq.SortedEntry, int64, error) {
	return nil, 0, c.block(ctx, "paged read")
}

func (c *lifetimeClient) Do(ctx context.Context, _ ...any) (any, error) {
	return nil, c.block(ctx, "console")
}

func (c *lifetimeClient) block(ctx context.Context, name string) error {
	c.started <- lifetimeRequest{name, ctx}
	defer func() { c.finished <- name }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return nil
	}
}

func awaitLifetime[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for lifecycle event")
		var zero T
		return zero
	}
}

func newLifetimeClient(t *testing.T) *lifetimeClient {
	t.Helper()
	c := &lifetimeClient{
		started: make(chan lifetimeRequest, 8), finished: make(chan string, 8),
		release: make(chan struct{}),
	}
	t.Cleanup(func() { close(c.release) })
	return c
}

func TestAppRunCancelsRequests(t *testing.T) {
	inputError := errors.New("test input failed")
	for name, test := range map[string]struct {
		input string
		err   error
	}{
		"quit":             {input: "q"},
		"ctrl c":           {input: "\x03"},
		"input error":      {err: inputError},
		"external context": {err: context.Canceled},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			client := newLifetimeClient(t)
			app := NewWithContext(ctx, client, "", true, devtools.NewTracker())
			defer app.cancel()
			// Prepare a confirmed mutation and console command before Run. They
			// remain owned by the app even while their views are hidden.
			retries := app.viewRegistry[viewRetries]
			retries.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
			_, mutation := retries.Update(confirmdialog.ActionMsg{Target: "retries.delete_all", Confirmed: true})
			if mutation == nil {
				t.Fatal("missing confirmed mutation command")
			}
			console := app.toggleDevToolsDialog()().(dialogs.OpenDialogMsg).Model
			console.Init()
			console.Update(tea.PasteMsg{Content: "PING"})
			_, consoleCmd := console.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if consoleCmd == nil {
				t.Fatal("missing console command")
			}
			paged := app.viewRegistry[viewDead].Init()
			scheduled := false
			reader, writer := io.Pipe()
			defer func() { _ = reader.Close() }()
			defer func() { _ = writer.Close() }()
			done := make(chan error, 1)
			go func() {
				_, err := app.Run(tea.WithInput(reader), tea.WithOutput(io.Discard),
					tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithWindowSize(80, 24),
					tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
						if _, ok := msg.(tea.WindowSizeMsg); ok && !scheduled {
							scheduled = true
							return tea.BatchMsg{mutation, consoleCmd, paged, func() tea.Msg { return msg }}
						}
						return msg
					}))
				done <- err
			}()

			requests := map[string]context.Context{}
			for range 6 {
				request := awaitLifetime(t, client.started)
				requests[request.name] = request.ctx
			}
			if len(requests) != 6 {
				t.Fatalf("started requests = %v", requests)
			}
			for name, request := range requests {
				if request.Err() != nil {
					t.Fatalf("%s canceled before shutdown", name)
				}
			}

			triggered := make(chan error, 1)
			go func() {
				var err error
				switch {
				case test.input != "":
					_, err = io.WriteString(writer, test.input)
				case errors.Is(test.err, inputError):
					err = writer.CloseWithError(inputError)
				default:
					cancel()
				}
				triggered <- err
			}()
			if err := awaitLifetime(t, done); !errors.Is(err, test.err) {
				t.Fatalf("Run error = %v, want %v", err, test.err)
			}
			if err := awaitLifetime(t, triggered); err != nil {
				t.Fatal(err)
			}
			for name, request := range requests {
				if !errors.Is(request.Err(), context.Canceled) {
					t.Fatalf("%s context remains live after Run", name)
				}
			}
			for range 6 {
				name := awaitLifetime(t, client.finished)
				if _, ok := requests[name]; !ok {
					t.Fatalf("unexpected worker completion: %s", name)
				}
				delete(requests, name)
			}
		})
	}
}

func TestAppInitialStatsReplacement(t *testing.T) {
	client := newLifetimeClient(t)
	app := New(client, "", false, nil)
	defer app.cancel()
	// Init's value copy starts stats alongside the dashboard and timer.
	commands := app.Init()().(tea.BatchMsg)
	finished := make(chan tea.Msg, 1)
	go func() { finished <- commands[2]() }()
	first := awaitLifetime(t, client.started)
	if first.name != "stats" {
		t.Fatalf("started %s, want stats", first.name)
	}
	app.fetchStatsCmd()
	if !errors.Is(first.ctx.Err(), context.Canceled) {
		t.Fatal("replacement did not cancel initial stats")
	}
	awaitLifetime(t, finished)
	awaitLifetime(t, client.finished)
}
