package views

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

func queueTestClient(t *testing.T) (*miniredis.Miniredis, *sidekiq.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	features := queueTestFeatures(t, "pro")
	client, err := sidekiq.NewClient("redis://"+mr.Addr(), sidekiq.WithFeatures(features))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return mr, client
}

func queuePauseKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'p', Text: "p"} }

func queueConfirmation(actions *queueActions, confirmed bool) confirmdialog.ActionMsg {
	target := ""
	if actions.pending != nil {
		target = actions.pending.target
	}
	return confirmdialog.ActionMsg{Target: target, Confirmed: confirmed}
}

func TestQueuePauseGates(t *testing.T) {
	for name, test := range map[string]struct{ enabled, featureEnabled bool }{
		"readonly enabled feature":   {false, true},
		"disabled feature dangerous": {true, false},
		"disabled feature readonly":  {false, false},
	} {
		t.Run(name, func(t *testing.T) {
			capabilities := sidekiq.Capabilities{}
			if test.featureEnabled {
				capabilities = queueTestFeatures(t, "pro")
			}
			list := NewQueuesList(nil)
			list.capabilities = capabilities
			list.queues = []*QueuesListInfo{{Name: "default"}}
			list.SetDangerousActionsEnabled(test.enabled)
			details := NewQueueDetails(nil)
			details.capabilities = capabilities
			details.queues = []*QueueInfo{{Name: "default"}}
			details.SetDangerousActionsEnabled(test.enabled)
			for _, view := range []View{list, details} {
				if _, cmd := view.Update(queuePauseKey()); cmd != nil {
					t.Fatalf("%T offered pause without both gates", view)
				}
			}
		})
	}
}

func TestQueuesListPauseConfirmationAndRefresh(t *testing.T) {
	mr, client := queueTestClient(t)
	_, _ = mr.SetAdd("queues", "default", "other")
	view := NewQueuesList(client)
	view.SetDangerousActionsEnabled(true)
	view.Update(view.fetchDataCmd()())
	_, cmd := view.Update(queuePauseKey())
	if cmd == nil {
		t.Fatal("pause did not open confirmation")
	}
	opened := cmd().(dialogs.OpenDialogMsg)
	opened.Model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if output := ansi.Strip(opened.Model.View()); !strings.Contains(output, "Pause queue") {
		t.Fatalf("wrong dialog: %s", output)
	}
	if mr.Exists("paused") {
		t.Fatal("opening confirmation wrote state")
	}
	view.Update(queueConfirmation(&view.actions, false))
	if mr.Exists("paused") {
		t.Fatal("cancel wrote state")
	}
	view.Update(queuePauseKey())
	if _, cmd := view.Update(confirmdialog.ActionMsg{Target: "other", Confirmed: true}); cmd != nil {
		t.Fatal("wrong target accepted")
	}
	stale := view.fetchDataCmd()()
	view.table.SetCursor(1)
	_, cmd = view.Update(queueConfirmation(&view.actions, true))
	if cmd == nil {
		t.Fatal("confirmation ignored")
	}
	view.Update(stale)
	if _, duplicate := view.Update(queuePauseKey()); duplicate != nil {
		t.Fatal("duplicate mutation allowed")
	}
	_, refresh := view.Update(cmd())
	if refresh == nil {
		t.Fatal("mutation did not request readback")
	}
	view.Update(refresh())
	view.Update(stale)
	if !view.queues[0].Paused || view.queues[1].Paused {
		t.Fatalf("changed wrong queue or accepted stale result: %+v", view.queues)
	}
	if got, _ := mr.SIsMember("queues", "default"); !got {
		t.Fatal("pause deleted queue")
	}
	view.table.SetCursor(0)
	view.Update(queuePauseKey())
	_, cmd = view.Update(queueConfirmation(&view.actions, true))
	_, refresh = view.Update(cmd())
	view.Update(refresh())
	if view.queues[0].Paused {
		t.Fatal("resume did not refresh state")
	}
}

func TestQueueDetailsPauseRejectsStaleWindow(t *testing.T) {
	_, client := queueTestClient(t)
	view := NewQueueDetails(client)
	view.SetDangerousActionsEnabled(true)
	view.capabilities = queueTestFeatures(t, "ent")
	view.queues = []*QueueInfo{{Name: "default"}, {Name: "other"}}
	stale := lazytable.DataMsg{RequestID: view.lazy.RequestID(), Result: lazytable.FetchResult{Payload: queueDetailsPayload{queues: []*QueueInfo{{Name: "stale"}}}}}
	view.Update(queuePauseKey())
	view.selectedQueue = 1
	_, cmd := view.Update(queueConfirmation(&view.actions, true))
	if cmd == nil {
		t.Fatal("pause confirmation ignored")
	}
	view.Update(stale)
	_, refresh := view.Update(cmd())
	if refresh == nil {
		t.Fatal("missing readback")
	}
	view.Update(stale)
	if view.queues[0].Name != "default" {
		t.Fatal("pre-action fetch accepted")
	}
	paused, err := client.NewQueue("default").Paused(context.Background())
	if err != nil || !paused {
		t.Fatalf("original queue not paused: %v, %v", paused, err)
	}
	if _, duplicate := view.Update(queueConfirmation(&view.actions, true)); duplicate != nil {
		t.Fatal("duplicate confirmation accepted")
	}
}

func TestQueuePauseActionFailureAndDisposal(t *testing.T) {
	_, client := queueTestClient(t)
	view := NewQueuesList(client)
	view.SetDangerousActionsEnabled(true)
	view.capabilities = queueTestFeatures(t, "pro")
	view.queues = []*QueuesListInfo{{Name: "default"}}
	view.Update(queuePauseKey())
	_, cmd := view.Update(queueConfirmation(&view.actions, true))
	view.CancelRequests()
	if _, refresh := view.Update(cmd()); refresh != nil {
		t.Fatal("disposed action completion accepted")
	}
	if view.actions.busy {
		t.Fatal("hidden view remained busy")
	}
	view.Update(queuePauseKey())
	_, cmd = view.Update(queueConfirmation(&view.actions, true))
	_ = client.Redis().Set(context.Background(), "paused", "wrong-type", 0).Err()
	result := cmd().(QueueActionResultMsg)
	if result.Err == nil {
		t.Fatal("mutation error lost")
	}
	view.Update(result)
	if view.actions.busy {
		t.Fatal("failed mutation remained busy")
	}
}

func TestGoldenPausedQueuesList(t *testing.T) {
	for name, width := range map[string]int{"wide": 140, "narrow": 80} {
		t.Run(name, func(t *testing.T) {
			view := NewQueuesList(nil)
			view.SetSize(width, 12)
			view.SetStyles(Styles{})
			view.ready = true
			view.capabilities = queueTestFeatures(t, "pro")
			view.queues = []*QueuesListInfo{{Name: "critical", Size: 125, Latency: 15, Paused: true}, {Name: "default", Size: 20}}
			view.updateTableRows()
			golden.RequireEqual(t, []byte(ansi.Strip(view.View())))
		})
	}
}

func TestGoldenPausedQueueDetails(t *testing.T) {
	for name, width := range map[string]int{"wide": 140, "narrow": 60} {
		t.Run(name, func(t *testing.T) {
			view := NewQueueDetails(nil)
			view.SetSize(width, 12)
			view.SetStyles(Styles{})
			view.ready = true
			view.queues = []*QueueInfo{{Name: "critical", Size: 100, Paused: true}, {Name: "default", Size: 90}, {Name: "mailers", Size: 80}, {Name: "reports", Size: 70}, {Name: "exports", Size: 60}, {Name: "a-very-long-selected-queue-outside-the-top-five-queue-shortcuts", Paused: true}}
			view.selectedQueue = 5
			header := view.HeaderLines()
			if len(header) != 5 {
				t.Fatalf("header height = %d", len(header))
			}
			if width == 140 && !strings.Contains(ansi.Strip(view.View()), " · Paused") {
				t.Fatal("selected paused state hidden")
			}
			golden.RequireEqual(t, []byte(ansi.Strip(strings.Join(header, "\n")+"\n"+view.View())))
		})
	}
}

func TestQueueConfirmationPreservesFirstAction(t *testing.T) {
	for name, keys := range map[string]string{"pause twice": "pp", "delete twice": "dd", "pause then delete": "pd", "delete then pause": "dp"} {
		t.Run(name, func(t *testing.T) {
			mr, client := queueTestClient(t)
			_, _ = mr.SetAdd("queues", "default", "other")
			_, _ = mr.Lpush("queue:default", `{"jid":"original","class":"TestJob","args":[]}`)
			_, _ = mr.Lpush("queue:other", `{"jid":"other","class":"TestJob","args":[]}`)
			view := NewQueuesList(client)
			view.SetDangerousActionsEnabled(true)
			view.Update(view.fetchDataCmd()())
			first := rune(keys[0])
			_, open := view.Update(tea.KeyPressMsg{Code: first, Text: string(first)})
			if open == nil {
				t.Fatal("first key did not schedule a dialog")
			}
			original := *view.actions.pending
			view.table.SetCursor(1)
			second := rune(keys[1])
			if _, duplicate := view.Update(tea.KeyPressMsg{Code: second, Text: string(second)}); duplicate != nil {
				t.Fatal("pending action allowed a second dialog")
			}
			if *view.actions.pending != original {
				t.Fatal("second key replaced the first action")
			}
			manager := dialogs.NewDialogCmp()
			manager, _ = manager.Update(open())
			manager, confirm := manager.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
			mutations := 0
			for _, command := range confirm().(tea.BatchMsg) {
				switch msg := command().(type) {
				case confirmdialog.ActionMsg:
					_, mutation := view.Update(msg)
					if mutation == nil {
						t.Fatal("visible confirmation did not execute original action")
					}
					result := mutation().(QueueActionResultMsg)
					if result.Err != nil {
						t.Fatal(result.Err)
					}
					view.Update(result)
					mutations++
				case dialogs.CloseDialogMsg:
					manager, _ = manager.Update(msg)
				}
			}
			if mutations != 1 || manager.HasDialogs() {
				t.Fatal("confirmation did not finish exactly once")
			}
			if !mr.Exists("queue:other") {
				t.Fatal("confirmation mutated the new selection")
			}
			if first == 'p' {
				if !mr.Exists("queue:default") {
					t.Fatal("pause deleted the original queue")
				}
				paused, err := client.NewQueue("default").Paused(context.Background())
				if err != nil || !paused {
					t.Fatalf("original queue not paused: %v, %v", paused, err)
				}
			} else if mr.Exists("queue:default") {
				t.Fatal("original queue was not deleted")
			}
		})
	}
}

// escapeReopenModel runs the dialog lifecycle through Bubble Tea so ordered
// commands are exercised by the real runtime, including its private sequenceMsg.
type escapeReopenModel struct {
	manager   dialogs.DialogCmp
	escape    tea.Cmd
	onAction  func(confirmdialog.ActionMsg) tea.Cmd
	reopen    func() tea.Cmd
	canceled  bool
	reopened  bool
	confirmed bool
	failure   string
}

func (m *escapeReopenModel) Init() tea.Cmd  { return m.escape }
func (m *escapeReopenModel) View() tea.View { return tea.NewView("") }
func (m *escapeReopenModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case confirmdialog.ActionMsg:
		command := m.onAction(msg)
		if msg.Confirmed {
			m.confirmed = command != nil
			return m, tea.Quit
		}
		m.canceled = true
	case dialogs.CloseDialogMsg:
		if m.reopened {
			return m, nil
		}
		if !m.canceled {
			m.failure = "dialog closed before cancellation reached its owner"
			return m, tea.Quit
		}
		m.manager, _ = m.manager.Update(msg)
		open := m.reopen()
		if open == nil {
			m.failure = "Escape left the original action pending"
			return m, tea.Quit
		}
		m.manager, _ = m.manager.Update(open())
		m.reopened = true
		var command tea.Cmd
		m.manager, command = m.manager.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
		return m, command
	}
	return m, nil
}

func TestConfirmationEscapeCancelsBeforeReopening(t *testing.T) {
	for _, name := range []string{"queue token", "same sorted job target"} {
		t.Run(name, func(t *testing.T) {
			var open tea.Cmd
			model := &escapeReopenModel{manager: dialogs.NewDialogCmp()}
			if name == "queue token" {
				view := NewQueuesList(nil)
				view.SetDangerousActionsEnabled(true)
				view.capabilities = queueTestFeatures(t, "pro")
				view.queues = []*QueuesListInfo{{Name: "default"}}
				model.reopen = func() tea.Cmd { _, command := view.Update(queuePauseKey()); return command }
				model.onAction = func(msg confirmdialog.ActionMsg) tea.Cmd { _, command := view.Update(msg); return command }
			} else {
				var pending pendingConfirm[retriesJobAction]
				model.reopen = func() tea.Cmd {
					pending.Set(retriesJobActionDelete, nil, "same-job-jid")
					return func() tea.Msg {
						return dialogs.OpenDialogMsg{Model: confirmdialog.New(confirmdialog.WithTarget("same-job-jid"))}
					}
				}
				model.onAction = func(msg confirmdialog.ActionMsg) tea.Cmd {
					_, _, accepted := pending.Confirm(msg, true, retriesJobActionNone)
					if accepted {
						return func() tea.Msg { return nil }
					}
					return nil
				}
			}
			open = model.reopen()
			model.manager, _ = model.manager.Update(open())
			model.manager, model.escape = model.manager.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			program := tea.NewProgram(model, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithContext(ctx))
			if _, err := program.Run(); err != nil {
				t.Fatal(err)
			}
			if model.failure != "" {
				t.Fatal(model.failure)
			}
			if !model.canceled || !model.reopened || !model.confirmed {
				t.Fatalf("dialog lifecycle incomplete: canceled=%v reopened=%v confirmed=%v", model.canceled, model.reopened, model.confirmed)
			}
		})
	}
}

func queueTestFeatures(t *testing.T, names ...string) sidekiq.Capabilities {
	t.Helper()
	features, err := sidekiq.ResolveFeatures(names)
	if err != nil {
		t.Fatal(err)
	}
	return features
}

func TestQueueHeaderSeparatesShortcutAndPausedStatus(t *testing.T) {
	view := NewQueueDetails(nil)
	view.SetStyles(Styles{})
	view.queues = []*QueueInfo{{Name: "mailers", Size: 1, Paused: true}}
	header := ansi.Strip(strings.Join(view.HeaderLines(), "\n"))
	if !strings.Contains(header, "ctrl+1 mailers") {
		t.Fatalf("shortcut touches queue name: %q", header)
	}
	if !strings.Contains(header, "  Paused") {
		t.Fatalf("paused status touches statistics: %q", header)
	}
}
