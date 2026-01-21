package views

import (
	"fmt"
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

type testAction int

const (
	testActionNone testAction = iota
	testActionRun
)

func TestPendingConfirm(t *testing.T) {
	entry := testSortedEntry("abc123")

	tests := map[string]struct {
		setup      func(*pendingConfirm[testAction])
		msg        confirmdialog.ActionMsg
		enabled    bool
		wantOK     bool
		wantAction testAction
		wantEntry  *sidekiq.SortedEntry
		second     *struct {
			msg     confirmdialog.ActionMsg
			enabled bool
			wantOK  bool
		}
	}{
		"confirmed clears and returns action": {
			setup: func(p *pendingConfirm[testAction]) {
				p.SetForEntry(testActionRun, entry)
			},
			msg:        confirmdialog.ActionMsg{Confirmed: true, Target: entry.JID()},
			enabled:    true,
			wantOK:     true,
			wantAction: testActionRun,
			wantEntry:  entry,
			second: &struct {
				msg     confirmdialog.ActionMsg
				enabled bool
				wantOK  bool
			}{
				msg:     confirmdialog.ActionMsg{Confirmed: true, Target: entry.JID()},
				enabled: true,
				wantOK:  false,
			},
		},
		"cancel clears without action": {
			setup: func(p *pendingConfirm[testAction]) {
				p.SetForEntry(testActionRun, entry)
			},
			msg:        confirmdialog.ActionMsg{Confirmed: false, Target: entry.JID()},
			enabled:    true,
			wantOK:     false,
			wantAction: testActionNone,
			wantEntry:  nil,
			second: &struct {
				msg     confirmdialog.ActionMsg
				enabled bool
				wantOK  bool
			}{
				msg:     confirmdialog.ActionMsg{Confirmed: true, Target: entry.JID()},
				enabled: true,
				wantOK:  false,
			},
		},
		"target mismatch preserves pending": {
			setup: func(p *pendingConfirm[testAction]) {
				p.SetForEntry(testActionRun, entry)
			},
			msg:        confirmdialog.ActionMsg{Confirmed: true, Target: "other"},
			enabled:    true,
			wantOK:     false,
			wantAction: testActionNone,
			wantEntry:  nil,
			second: &struct {
				msg     confirmdialog.ActionMsg
				enabled bool
				wantOK  bool
			}{
				msg:     confirmdialog.ActionMsg{Confirmed: true, Target: entry.JID()},
				enabled: true,
				wantOK:  true,
			},
		},
		"disabled preserves pending": {
			setup: func(p *pendingConfirm[testAction]) {
				p.SetForEntry(testActionRun, entry)
			},
			msg:        confirmdialog.ActionMsg{Confirmed: true, Target: entry.JID()},
			enabled:    false,
			wantOK:     false,
			wantAction: testActionNone,
			wantEntry:  nil,
			second: &struct {
				msg     confirmdialog.ActionMsg
				enabled bool
				wantOK  bool
			}{
				msg:     confirmdialog.ActionMsg{Confirmed: true, Target: entry.JID()},
				enabled: true,
				wantOK:  true,
			},
		},
		"none action ignored": {
			msg:        confirmdialog.ActionMsg{Confirmed: true, Target: entry.JID()},
			enabled:    true,
			wantOK:     false,
			wantAction: testActionNone,
			wantEntry:  nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var pending pendingConfirm[testAction]
			if tc.setup != nil {
				tc.setup(&pending)
			}

			action, gotEntry, ok := pending.Confirm(tc.msg, tc.enabled, testActionNone)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tc.wantOK)
			}
			if action != tc.wantAction {
				t.Fatalf("action=%v, want %v", action, tc.wantAction)
			}
			if gotEntry != tc.wantEntry {
				t.Fatalf("entry=%v, want %v", gotEntry, tc.wantEntry)
			}

			if tc.second != nil {
				action2, gotEntry2, ok2 := pending.Confirm(tc.second.msg, tc.second.enabled, testActionNone)
				if ok2 != tc.second.wantOK {
					t.Fatalf("second ok=%v, want %v", ok2, tc.second.wantOK)
				}
				if ok2 && action2 != testActionRun {
					t.Fatalf("second action=%v, want %v", action2, testActionRun)
				}
				if ok2 && gotEntry2 != entry {
					t.Fatalf("second entry=%v, want %v", gotEntry2, entry)
				}
			}
		})
	}
}

func testSortedEntry(jid string) *sidekiq.SortedEntry {
	return &sidekiq.SortedEntry{
		JobRecord: sidekiq.NewJobRecord(fmt.Sprintf(`{"jid":"%s"}`, jid), ""),
	}
}
