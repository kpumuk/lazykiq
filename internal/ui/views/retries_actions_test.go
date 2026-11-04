package views

import (
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func TestRetriesKillMissingJob(t *testing.T) {
	mr, client := queueTestClient(t)
	view := NewRetries(client)
	entry := sidekiq.NewSortedEntry(`{"jid":"gone","class":"Good","queue":"default"}`, 1)
	msg := view.killJobCmd(entry)()
	failure, ok := msg.(ConnectionErrorMsg)
	if !ok || failure.Err == nil || failure.Err.Error() != "job not found" {
		t.Fatalf("kill result = %#v, want job not found error", msg)
	}
	if mr.Exists("dead") {
		t.Fatal("stale kill created a dead job")
	}
}
