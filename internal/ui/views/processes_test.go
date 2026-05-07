package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

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
