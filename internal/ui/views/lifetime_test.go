package views

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/redis/go-redis/v9"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type lifetimeRedisHook struct{ started chan context.Context }

func (h lifetimeRedisHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h lifetimeRedisHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, _ redis.Cmder) error { return h.wait(ctx) }
}

func (h lifetimeRedisHook) ProcessPipelineHook(redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, _ []redis.Cmder) error { return h.wait(ctx) }
}

func (h lifetimeRedisHook) wait(ctx context.Context) error {
	h.started <- ctx
	<-ctx.Done()
	return ctx.Err()
}

func TestMutationLifetimes(t *testing.T) {
	for name, build := range map[string]func(*sidekiq.Client, context.Context) (RequestCanceler, tea.Cmd){
		"retry delete": func(c *sidekiq.Client, ctx context.Context) (RequestCanceler, tea.Cmd) {
			v := NewRetries(c)
			v.SetContext(ctx)
			return v, v.deleteAllCmd()
		},
		"dead delete": func(c *sidekiq.Client, ctx context.Context) (RequestCanceler, tea.Cmd) {
			v := NewDead(c)
			v.SetContext(ctx)
			return v, v.deleteAllCmd()
		},
		"scheduled delete": func(c *sidekiq.Client, ctx context.Context) (RequestCanceler, tea.Cmd) {
			v := NewScheduled(c)
			v.SetContext(ctx)
			return v, v.deleteAllCmd()
		},
		"process pause": func(c *sidekiq.Client, ctx context.Context) (RequestCanceler, tea.Cmd) {
			v := NewProcessesList(c)
			v.SetContext(ctx)
			return v, v.pauseProcessCmd("worker")
		},
		"process stop": func(c *sidekiq.Client, ctx context.Context) (RequestCanceler, tea.Cmd) {
			v := NewProcessesList(c)
			v.SetContext(ctx)
			return v, v.stopProcessCmd("worker")
		},
		"queue list": func(c *sidekiq.Client, ctx context.Context) (RequestCanceler, tea.Cmd) {
			v := NewQueuesList(c)
			v.SetContext(ctx)
			v.actions.prepare("default", "delete")
			return v, v.actions.confirm(v.Context(), queueConfirmation(&v.actions, true), c)
		},
		"queue details": func(c *sidekiq.Client, ctx context.Context) (RequestCanceler, tea.Cmd) {
			v := NewQueueDetails(c)
			v.SetContext(ctx)
			v.actions.prepare("default", "pause")
			return v, v.actions.confirm(v.Context(), queueConfirmation(&v.actions, true), c)
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, client := queueTestClient(t)
			hook := lifetimeRedisHook{started: make(chan context.Context, 1)}
			client.AddHook(hook)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			view, cmd := build(client, ctx)
			// Navigation cancels reads; a confirmed mutation keeps the app lifetime.
			view.CancelRequests()
			done := make(chan tea.Msg, 1)
			go func() { done <- cmd() }()
			select {
			case request := <-hook.started:
				if request.Err() != nil {
					t.Fatal("hiding canceled a confirmed mutation")
				}
				cancel()
				if request.Err() == nil {
					t.Fatal("mutation survived application cancellation")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mutation did not reach Redis")
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("mutation worker did not finish")
			}
		})
	}
}
