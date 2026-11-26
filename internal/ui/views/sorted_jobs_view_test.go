package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

func sortedJobsKey(key string) tea.KeyPressMsg {
	if letter, ok := strings.CutPrefix(key, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(letter[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
}

// TestSortedJobsDangerGates checks that the Dead, Scheduled, and Retries
// screens offer and run their mutations only with dangerous actions enabled.
func TestSortedJobsDangerGates(t *testing.T) {
	screens := map[string]struct {
		build func(sidekiq.API) *SortedJobs
		key   string
	}{
		"dead":      {build: NewDead, key: "dead"},
		"scheduled": {build: NewScheduled, key: "schedule"},
		"retries":   {build: NewRetries, key: "retry"},
	}
	modes := map[string]struct {
		enabled      bool // dangerous actions enabled when the key is pressed
		disableAfter bool // dangerous actions disabled while the dialog is open
	}{
		"disabled":              {},
		"disabled after prompt": {enabled: true, disableAfter: true},
		"enabled":               {enabled: true},
	}
	const payload = `{"jid":"abc123","class":"ExampleJob","queue":"default","args":[]}`

	for screenName, screen := range screens {
		for _, action := range screen.build(nil).spec.actions {
			for modeName, mode := range modes {
				t.Run(screenName+"/"+action.key+"/"+modeName, func(t *testing.T) {
					mr, client := queueTestClient(t)
					if _, err := mr.ZAdd(screen.key, 1700000000, payload); err != nil {
						t.Fatal(err)
					}
					view := screen.build(client)
					view.jobs = []*sidekiq.SortedEntry{sidekiq.NewSortedEntry(payload, 1700000000)}
					view.lazy.Table().SetRows([]table.Row{{ID: "abc123"}})
					view.SetDangerousActionsEnabled(mode.enabled)
					wantHints := 0
					if mode.enabled {
						wantHints = len(view.spec.actions)
					}
					if got := len(view.MutationBindings()); got != wantHints {
						t.Fatalf("mutation hints = %d, want %d", got, wantHints)
					}
					before := mr.Dump()

					_, cmd := view.Update(sortedJobsKey(action.key))
					var opened bool
					if cmd != nil {
						_, opened = cmd().(dialogs.OpenDialogMsg)
					}
					if opened != mode.enabled {
						t.Fatalf("confirmation opened = %v with dangerous actions %v", opened, mode.enabled)
					}

					// A forged confirmation must not mutate while the gate is closed.
					target := view.pendingConfirm.target
					if mode.disableAfter {
						view.SetDangerousActionsEnabled(false)
					}
					_, mutate := view.Update(confirmdialog.ActionMsg{Target: target, Confirmed: true})
					wantMutation := mode.enabled && !mode.disableAfter
					if (mutate != nil) != wantMutation {
						t.Fatalf("confirmed mutation = %v, want %v", mutate != nil, wantMutation)
					}
					if !wantMutation {
						if after := mr.Dump(); after != before {
							t.Fatalf("redis changed without dangerous actions:\n%s", after)
						}
						return
					}
					if msg := mutate(); msg != (RefreshMsg{}) {
						t.Fatalf("mutation result = %#v, want RefreshMsg", msg)
					}
					if mr.Dump() == before {
						t.Fatal("confirmed mutation left redis unchanged")
					}
				})
			}
		}
	}
}
