package views

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

func TestPeriodicReadLifetimes(t *testing.T) {
	for name, detail := range map[string]bool{"list": false, "detail": true} {
		t.Run(name, func(t *testing.T) {
			_, client := periodicTestClient(t)
			p := NewPeriodic(client)
			if detail {
				p = NewPeriodicDetail(client)
				p.SetPeriodicJob(periodicFixture().job.LID)
			}
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			p.SetContext(parent)
			t.Cleanup(p.CancelRequests)
			first := p.fetch()().(ReadResultMsg)
			second := p.fetch()().(ReadResultMsg)
			if first.Current() || !second.Current() || parent.Err() != nil {
				t.Fatal("replacement did not preserve application lifetime")
			}
			p.CancelRequests()
			if second.Current() || parent.Err() != nil {
				t.Fatal("hiding did not cancel only the read")
			}
			p.Dispose()
			if detail {
				p.SetPeriodicJob(periodicFixture().job.LID)
			}
			reopened := p.Init()().(ReadResultMsg)
			if !reopened.Current() {
				t.Fatal("reopened view lost its lifetime")
			}
			cancel()
			p.Update(reopened)
			if reopened.Current() || p.ready {
				t.Fatal("completed read survived application cancellation")
			}
		})
	}
}

func TestPeriodicMutationLifetimes(t *testing.T) {
	for name, detail := range map[string]bool{"list": false, "detail": true} {
		for action, key := range map[string]string{"pause": "p", "enqueue": "e"} {
			for timing, queued := range map[string]bool{"running": false, "queued": true} {
				t.Run(name+"/"+action+"/"+timing, func(t *testing.T) {
					mr, client := periodicTestClient(t)
					p := NewPeriodic(client)
					if detail {
						p = NewPeriodicDetail(client)
						p.SetPeriodicJob(periodicFixture().job.LID)
					}
					parent, cancel := context.WithCancel(t.Context())
					defer cancel()
					p.SetContext(parent)
					p.SetDangerousActionsEnabled(true)
					p.Update(p.Init()())
					p.Update(periodicKey(key))
					_, cmd := p.Update(confirmdialog.ActionMsg{Target: p.actions.pending.target, Confirmed: true})
					if cmd == nil {
						t.Fatal("missing confirmed mutation")
					}
					p.CancelRequests()
					p.Dispose()
					hook := lifetimeRedisHook{started: make(chan context.Context, 1)}
					client.AddHook(hook)
					if queued {
						cancel()
					}
					done := make(chan tea.Msg, 1)
					go func() { done <- cmd() }()
					select {
					case request := <-hook.started:
						if !queued && request.Err() != nil {
							t.Fatal("navigation canceled a confirmed mutation")
						}
						if _, ok := request.Deadline(); !ok {
							t.Fatal("mutation lost its timeout")
						}
						cancel()
						if !errors.Is(request.Err(), context.Canceled) {
							t.Fatal("mutation survived application cancellation")
						}
					case <-time.After(5 * time.Second):
						t.Fatal("mutation did not reach Redis")
					}
					select {
					case msg := <-done:
						if !errors.Is(msg.(PeriodicActionResultMsg).Err, context.Canceled) {
							t.Fatal("mutation did not return cancellation")
						}
					case <-time.After(5 * time.Second):
						t.Fatal("mutation did not finish")
					}
					if mr.HGet("loop-"+periodicFixture().job.LID, "paused") != "" || mr.Exists("queue:low") {
						t.Fatal("canceled mutation wrote to Redis")
					}
				})
			}
		}
	}
}
