package views

import (
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func sortedJobsAction(t *testing.T, view *SortedJobs, key string) *sortedJobAction {
	t.Helper()
	for i := range view.spec.actions {
		if view.spec.actions[i].key == key {
			return &view.spec.actions[i]
		}
	}
	t.Fatalf("%s has no %q action", view.Name(), key)
	return nil
}

func TestRetriesKillMissingJob(t *testing.T) {
	mr, client := queueTestClient(t)
	view := NewRetries(client)
	entry := sidekiq.NewSortedEntry(`{"jid":"gone","class":"Good","queue":"default"}`, 1)
	msg := view.actionCmd(sortedJobsAction(t, view, "K"), entry)()
	failure, ok := msg.(ActionErrorMsg)
	if !ok || failure.Err == nil || failure.Err.Error() != "Kill job failed for JID gone: job not found" {
		t.Fatalf("kill result = %#v, want job not found error", msg)
	}
	if mr.Exists("dead") {
		t.Fatal("stale kill created a dead job")
	}
}
