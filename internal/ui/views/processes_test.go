package views

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

type processDataClient struct {
	sidekiq.API
	fetch func(context.Context) ([]sidekiq.Process, error)
}

func (c processDataClient) GetProcessesData(ctx context.Context) ([]sidekiq.Process, error) {
	return c.fetch(ctx)
}

func TestProcessesListMetadataFilter(t *testing.T) {
	processes := []sidekiq.Process{
		{Identity: "a:1:nonce", Hostname: "Alpha", PID: 1, Tag: "Web", Capsules: map[string]sidekiq.Capsule{"default": {Weights: map[string]int{"Critical": 5}}}},
		{Identity: "b:2:nonce", Hostname: "Beta", PID: 2, Tag: "Jobs"},
	}
	for name, tc := range map[string]struct {
		filter string
		want   []sidekiq.Process
	}{
		"all":           {want: processes},
		"hostname":      {filter: "ALPHA:1", want: processes[:1]},
		"identity":      {filter: "A:1:NONCE", want: processes[:1]},
		"tag":           {filter: "WEB", want: processes[:1]},
		"capsule queue": {filter: "CRITICAL", want: processes[:1]},
		"no matches":    {filter: "missing", want: []sidekiq.Process{}},
	} {
		t.Run(name, func(t *testing.T) {
			client := processDataClient{fetch: func(context.Context) ([]sidekiq.Process, error) { return processes, nil }}
			view := NewProcessesList(client)
			view.filter = tc.filter
			t.Cleanup(view.CancelRequests)
			view.Update(view.fetchDataCmd()())
			if !reflect.DeepEqual(view.processes, tc.want) || !view.ready {
				t.Fatalf("filtered processes: %+v", view.processes)
			}
		})
	}
}

func TestProcessesListFetchErrors(t *testing.T) {
	failure := errors.New("metadata unavailable")
	for name, tc := range map[string]struct {
		err         error
		wantMessage bool
	}{
		"connection failure": {err: failure, wantMessage: true},
		"canceled":           {err: context.Canceled},
	} {
		t.Run(name, func(t *testing.T) {
			view := NewProcessesList(processDataClient{fetch: func(context.Context) ([]sidekiq.Process, error) { return nil, tc.err }})
			t.Cleanup(view.CancelRequests)
			msg := view.fetchDataCmd()()
			if tc.wantMessage {
				result, ok := msg.(ConnectionErrorMsg)
				if !ok || !errors.Is(result.Err, tc.err) {
					t.Fatalf("fetch error: %T %+v", msg, msg)
				}
			} else if msg != nil {
				t.Fatalf("canceled fetch returned %T", msg)
			}
		})
	}
}

func TestProcessesListRequestOwnership(t *testing.T) {
	for name, invalidate := range map[string]func(*ProcessesList){
		"refresh": func(v *ProcessesList) { v.Update(RefreshMsg{}) },
		"filter": func(v *ProcessesList) {
			v.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "new"})
		},
		"hide":    (*ProcessesList).CancelRequests,
		"dispose": (*ProcessesList).Dispose,
	} {
		t.Run(name, func(t *testing.T) {
			var requestContext context.Context
			client := processDataClient{fetch: func(ctx context.Context) ([]sidekiq.Process, error) {
				requestContext = ctx
				return []sidekiq.Process{{Identity: "old:1:nonce"}}, nil
			}}
			view := NewProcessesList(client)
			t.Cleanup(view.CancelRequests)
			stale := view.fetchDataCmd()()
			invalidate(view)
			if !errors.Is(requestContext.Err(), context.Canceled) {
				t.Fatal("invalidated request remains active")
			}
			view.Update(stale)
			if view.ready || len(view.processes) != 0 {
				t.Fatal("stale request installed process data")
			}
		})
	}
}

func TestProcessesListCanceledCommand(t *testing.T) {
	view := NewProcessesList(processDataClient{fetch: func(ctx context.Context) ([]sidekiq.Process, error) {
		return nil, ctx.Err()
	}})
	old := view.fetchDataCmd()
	current := view.fetchDataCmd()
	t.Cleanup(view.CancelRequests)
	if msg := old(); msg != nil {
		t.Fatalf("superseded command returned %T", msg)
	}
	view.CancelRequests()
	if msg := current(); msg != nil {
		t.Fatalf("hidden command returned %T", msg)
	}
}

func TestProcessesListCapturesFilter(t *testing.T) {
	view := NewProcessesList(processDataClient{fetch: func(context.Context) ([]sidekiq.Process, error) {
		return []sidekiq.Process{{Identity: "old:1:nonce"}, {Identity: "new:2:nonce"}}, nil
	}})
	t.Cleanup(view.CancelRequests)
	view.filter = "old"
	cmd := view.fetchDataCmd()
	view.filter = "new"
	msg := cmd().(processesListDataMsg)
	if len(msg.processes) != 1 || msg.processes[0].Identity != "old:1:nonce" {
		t.Fatalf("command read a later filter: %+v", msg.processes)
	}
}

func TestGoldenProcessesListMetadata(t *testing.T) {
	for name, tc := range map[string]struct {
		width  int
		filter string
	}{
		"wide":       {width: 160},
		"narrow":     {width: 80},
		"filtered":   {width: 160, filter: "CRITICAL"},
		"no matches": {width: 80, filter: "missing"},
	} {
		t.Run(name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			client, err := sidekiq.NewClient("redis://" + mr.Addr())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			_, _ = mr.SetAdd("processes", "alpha:1:nonce", "beta:2:nonce")
			mr.HSet("alpha:1:nonce", "info", `{"hostname":"alpha","pid":1,"tag":"web","concurrency":8,"version":"8.1.0","capsules":{"default":{"concurrency":8,"mode":"weighted","weights":{"critical":5,"default":1}}}}`, "busy", "3", "rss", "2048")
			mr.HSet("beta:2:nonce", "info", `{"hostname":"beta","pid":2,"concurrency":4,"queues":["mailers"],"version":"7.0.0"}`, "busy", "1", "quiet", "true")
			_, _ = mr.Lpush("alpha:1:nonce-signals", "TSTP")
			// A work hash of the wrong type makes any active-work read fail.
			_ = mr.Set("alpha:1:nonce:work", "unreadable work")
			view := NewProcessesList(client)
			view.SetStyles(Styles{})
			view.SetSize(tc.width, 12)
			view.filter = tc.filter
			t.Cleanup(view.CancelRequests)
			view.Update(view.Init()())
			if !view.ready {
				t.Fatal("process fetch did not complete")
			}
			output := ansi.Strip(view.View())
			if len(strings.Split(output, "\n")) != 12 {
				t.Fatalf("process view height changed:\n%s", output)
			}
			for line := range strings.SplitSeq(output, "\n") {
				if ansi.StringWidth(line) > tc.width {
					t.Fatalf("process row exceeds width %d: %q", tc.width, line)
				}
			}
			golden.RequireEqual(t, []byte(output))
		})
	}
}

func TestProcessesListDangerousActionsRequireConfirmation(t *testing.T) {
	cases := map[string]struct {
		key    string
		action string
	}{
		"pause": {
			key:    "p",
			action: processActionPause,
		},
		"stop": {
			key:    "s",
			action: processActionStop,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			view := NewProcessesList(nil)
			view.SetDangerousActionsEnabled(true)
			view.processes = []sidekiq.Process{{Identity: "worker:123:abc"}}
			view.updateTableRows()

			_, cmd := view.Update(tea.KeyPressMsg(tea.Key{Text: tc.key, Code: []rune(tc.key)[0]}))
			if cmd == nil {
				t.Fatal("dangerous action returned nil command, want confirmation dialog")
			}

			msg := cmd()
			open, ok := msg.(dialogs.OpenDialogMsg)
			if !ok {
				t.Fatalf("command returned %T, want dialogs.OpenDialogMsg", msg)
			}
			model, ok := open.Model.(*confirmdialog.Model)
			if !ok {
				t.Fatalf("dialog model = %T, want *confirm.Model", open.Model)
			}

			updated, actionCmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "y", Code: 'y'}))
			_ = updated
			if actionCmd == nil {
				t.Fatal("confirm yes returned nil command")
			}

			confirmed := collectConfirmAction(t, actionCmd)
			if !confirmed.Confirmed {
				t.Fatal("confirmation message was not confirmed")
			}
			wantTarget := tc.action + ":worker:123:abc"
			if confirmed.Target != wantTarget {
				t.Fatalf("target = %q, want %q", confirmed.Target, wantTarget)
			}
		})
	}
}

func collectConfirmAction(t *testing.T, cmd tea.Cmd) confirmdialog.ActionMsg {
	t.Helper()

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, subcmd := range batch {
			if confirmed, ok := subcmd().(confirmdialog.ActionMsg); ok {
				return confirmed
			}
		}
		t.Fatalf("batch did not contain confirm ActionMsg: %#v", msg)
	}

	confirmed, ok := msg.(confirmdialog.ActionMsg)
	if !ok {
		t.Fatalf("command returned %T, want confirm ActionMsg", msg)
	}
	return confirmed
}
