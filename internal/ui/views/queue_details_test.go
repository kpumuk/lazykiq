package views

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
)

func TestQueueDetailsFetchWindow_FilteredJobs(t *testing.T) {
	mr := miniredis.RunT(t)
	client, err := sidekiq.NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	_, _ = mr.SetAdd("queues", "default")

	entries := []string{
		`{"jid":"job1","class":"TestJob","args":["skip"]}`,
		`{"jid":"job2","class":"TestJob","args":["match older"]}`,
		`{"jid":"job3","class":"TestJob","args":["skip"]}`,
		`{"jid":"job4","class":"TestJob","args":["match middle"]}`,
		`{"jid":"job5","class":"TestJob","args":["match newest"]}`,
	}
	for _, entry := range entries {
		_, _ = mr.Lpush("queue:default", entry)
	}

	view := NewQueueDetails(client)
	view.filter = "match"

	result, err := view.fetchWindow(context.Background(), 1, 1, lazytable.CursorStart)
	if err != nil {
		t.Fatalf("fetchWindow failed: %v", err)
	}

	if result.Total != 3 {
		t.Fatalf("result.Total = %d, want 3", result.Total)
	}
	if result.WindowStart != 1 {
		t.Fatalf("result.WindowStart = %d, want 1", result.WindowStart)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("len(result.Rows) = %d, want 1", len(result.Rows))
	}
	if result.Rows[0].ID != "job4" {
		t.Fatalf("result.Rows[0].ID = %q, want %q", result.Rows[0].ID, "job4")
	}

	payload, ok := result.Payload.(queueDetailsPayload)
	if !ok {
		t.Fatalf("result.Payload type = %T, want queueDetailsPayload", result.Payload)
	}
	if len(payload.jobs) != 1 {
		t.Fatalf("len(payload.jobs) = %d, want 1", len(payload.jobs))
	}
	if got := payload.jobs[0].JID(); got != "job4" {
		t.Fatalf("payload.jobs[0].JID() = %q, want %q", got, "job4")
	}
	if got := payload.jobs[0].Position; got != 4 {
		t.Fatalf("payload.jobs[0].Position = %d, want %d", got, 4)
	}
}
