package views

import (
	"context"
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type errorsSummaryFetchClient struct {
	sidekiq.API
	name string
}

func (c *errorsSummaryFetchClient) GetErrorSummary(_ context.Context, query string) ([]sidekiq.ErrorSummaryRow, sidekiq.ErrorSummaryMeta, error) {
	return []sidekiq.ErrorSummaryRow{{DisplayClass: c.name + query}}, sidekiq.ErrorSummaryMeta{}, nil
}

func TestErrorsSummaryFetchCapturesInputs(t *testing.T) {
	for name, mutate := range map[string]func(*ErrorsSummary){
		"filter": func(e *ErrorsSummary) { e.filter = "new" },
		"client": func(e *ErrorsSummary) { e.client = &errorsSummaryFetchClient{name: "replacement:"} },
	} {
		t.Run(name, func(t *testing.T) {
			e := NewErrorsSummary(&errorsSummaryFetchClient{name: "original:"})
			t.Cleanup(e.CancelRequests)
			e.filter = "old"
			cmd := e.fetchDataCmd(true)
			mutate(e)

			msg, ok := cmd().(errorsSummaryDataMsg)
			if !ok {
				t.Fatal("expected error summary data")
			}
			if got := msg.rows[0].DisplayClass; got != "original:old" {
				t.Fatalf("request = %q, want original:old", got)
			}
		})
	}
}

func TestErrorsSummaryFetchConcurrentFilterChanges(t *testing.T) {
	e := NewErrorsSummary(&errorsSummaryFetchClient{})
	t.Cleanup(e.CancelRequests)
	e.filter = "old"
	cmd := e.fetchDataCmd(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 20000 {
			e.filter = "new"
			e.filter = "old"
		}
	}()
	defer func() { <-done }()
	for range 20000 {
		msg, ok := cmd().(errorsSummaryDataMsg)
		if !ok {
			t.Fatal("expected error summary data")
		}
		if got := msg.rows[0].DisplayClass; got != "old" {
			t.Fatalf("request filter = %q, want old", got)
		}
	}
}
