package views

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

type busyPollingClient struct {
	sidekiq.API
	fetch func(context.Context, string) (sidekiq.BusyData, error)
}

func (c *busyPollingClient) GetBusyData(ctx context.Context, query string) (sidekiq.BusyData, error) {
	return c.fetch(ctx, query)
}

func TestBusyPollCoalescesUntilCompletion(t *testing.T) {
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	calls := 0
	b := NewBusy(&busyPollingClient{fetch: func(ctx context.Context, _ string) (sidekiq.BusyData, error) {
		calls++
		started <- ctx
		select {
		case <-release:
			return busySelectionData([]string{"worker"}), nil
		case <-ctx.Done():
			return sidekiq.BusyData{}, ctx.Err()
		}
	}})
	defer b.CancelRequests()
	done := make(chan tea.Msg, 1)
	cmd := b.Init()
	go func() { done <- cmd() }()
	ctx := <-started
	for range 3 {
		if _, cmd := b.Update(RefreshMsg{}); cmd != nil || ctx.Err() != nil {
			t.Fatal("poll replaced outstanding read")
		}
	}
	close(release)
	reply := <-done
	// Also coalesce while a completed reply waits in the event queue.
	if _, cmd := b.Update(RefreshMsg{}); cmd != nil {
		t.Fatal("poll replaced undelivered completion")
	}
	b.Update(reply)
	if !b.ready || len(b.data.Processes) != 1 {
		t.Fatal("coalesced read did not publish")
	}
	_, next := b.Update(RefreshMsg{})
	if next == nil {
		t.Fatal("next poll did not fetch")
	}
	b.Update(next())
	if calls != 2 {
		t.Fatalf("fetches = %d, want 2", calls)
	}
}

func TestBusyFailedPollAllowsRetry(t *testing.T) {
	for name, failure := range map[string]error{
		"read error":   errors.New("offline"),
		"deadline":     context.DeadlineExceeded,
		"cancellation": context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			c := &readClient{err: failure}
			b := NewBusy(c)
			defer b.CancelRequests()
			b.Update(b.Init()())
			if b.ready {
				t.Fatal("failure published data")
			}
			c.err = nil
			_, retry := b.Update(RefreshMsg{})
			if retry == nil {
				t.Fatal("failure blocked retry")
			}
			b.Update(retry())
			if !b.ready {
				t.Fatal("retry did not publish")
			}
		})
	}
}

func TestBusyReplacementKeepsNewPollPending(t *testing.T) {
	for name, replace := range map[string]func(*Busy) tea.Cmd{
		"filter": func(b *Busy) tea.Cmd {
			_, cmd := b.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "new"})
			return cmd
		},
		"clear filter":       func(b *Busy) tea.Cmd { _, cmd := b.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}); return cmd },
		"hide and return":    func(b *Busy) tea.Cmd { b.CancelRequests(); _, cmd := b.Update(RefreshMsg{}); return cmd },
		"dispose and reopen": func(b *Busy) tea.Cmd { b.Dispose(); return b.Init() },
	} {
		for outcome, failure := range map[string]error{"success": nil, "failure": errors.New("old error")} {
			t.Run(name+"/"+outcome, func(t *testing.T) {
				c := &readClient{value: "old", err: failure}
				b := NewBusy(c)
				defer b.CancelRequests()
				b.filter = "old"
				old := b.fetchDataCmd()().(ReadResultMsg)
				current := replace(b)
				if current == nil || old.Current() {
					t.Fatal("replacement did not invalidate read immediately")
				}
				b.Update(old)
				// A different Busy instance's live result must not release the gate either.
				other := NewBusy(c)
				defer other.CancelRequests()
				b.Update(other.Init()())
				if _, cmd := b.Update(RefreshMsg{}); cmd != nil {
					t.Fatal("stale/foreign reply released current read")
				}
				c.err, c.value = nil, "new"
				b.Update(current())
				if !b.ready || b.data.Processes[0].Identity != "new" {
					t.Fatal("replacement failed to publish")
				}
			})
		}
	}
}

func TestBusyReadDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBusy(&busyPollingClient{fetch: func(ctx context.Context, _ string) (sidekiq.BusyData, error) {
			<-ctx.Done()
			return sidekiq.BusyData{}, ctx.Err()
		}})
		defer b.CancelRequests()
		start := time.Now()
		reply := b.Init()().(ReadResultMsg)
		if time.Since(start) != busyFetchTimeout || !errors.Is(reply.Err, context.DeadlineExceeded) || !reply.Current() {
			t.Fatalf("deadline completion after %s: %v, current=%t", time.Since(start), reply.Err, reply.Current())
		}
		b.Update(reply)
		if _, retry := b.Update(RefreshMsg{}); retry == nil {
			t.Fatal("deadline blocked retry")
		}
	})
}

func TestBusyFastFilterReplacementCancelsReads(t *testing.T) {
	type request struct {
		ctx   context.Context
		query string
	}
	started := make(chan request, 1)
	b := NewBusy(&busyPollingClient{fetch: func(ctx context.Context, query string) (sidekiq.BusyData, error) {
		started <- request{ctx, query}
		<-ctx.Done()
		return sidekiq.BusyData{}, ctx.Err()
	}})
	defer b.CancelRequests()
	run := func(cmd tea.Cmd) <-chan tea.Msg {
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		return done
	}
	old := run(b.Init())
	first := <-started
	_, filtered := b.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "new"})
	if first.ctx.Err() != context.Canceled {
		t.Fatal("filter did not cancel immediately")
	}
	pending := run(filtered)
	second := <-started
	if second.query != "new" {
		t.Fatalf("query = %q", second.query)
	}
	_, cleared := b.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if second.ctx.Err() != context.Canceled {
		t.Fatal("clear did not cancel immediately")
	}
	// Deliver canceled requests in reverse order while the clear is pending.
	b.Update(<-pending)
	b.Update(<-old)
	if _, cmd := b.Update(RefreshMsg{}); cmd != nil {
		t.Fatal("old cancellation cleared current operation")
	}
	// The replacement captured the cleared filter and original service handle.
	b.client = &busyFetchClient{name: "wrong:"}
	latest := run(cleared)
	third := <-started
	if third.query != "" {
		t.Fatalf("clear query = %q", third.query)
	}
	b.CancelRequests()
	<-latest
}
