package views

import (
	"context"
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type busyFetchClient struct {
	sidekiq.API
	name string
}

func (c *busyFetchClient) GetBusyData(_ context.Context, query string) (sidekiq.BusyData, error) {
	return sidekiq.BusyData{Processes: []sidekiq.Process{{Identity: c.name + query}}}, nil
}

func TestBusyFetchCapturesInputs(t *testing.T) {
	for name, mutate := range map[string]func(*Busy){
		"filter": func(b *Busy) { b.filter = "new" },
		"client": func(b *Busy) { b.client = &busyFetchClient{name: "replacement:"} },
	} {
		t.Run(name, func(t *testing.T) {
			b := NewBusy(&busyFetchClient{name: "original:"})
			t.Cleanup(b.CancelRequests)
			b.filter = "old"
			cmd := b.fetchDataCmd()
			mutate(b)

			msg, ok := cmd().(busyDataMsg)
			if !ok {
				t.Fatal("expected busy data")
			}
			if got := msg.data.Processes[0].Identity; got != "original:old" {
				t.Fatalf("request = %q, want original:old", got)
			}
		})
	}
}
