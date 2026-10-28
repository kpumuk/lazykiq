package ui

import (
	"context"
	"fmt"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type queueNavigationHook struct {
	commands   []string
	batches    int
	roundTrips int
}

func (h *queueNavigationHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *queueNavigationHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.commands = append(h.commands, cmd.Name())
		h.roundTrips++
		return next(ctx, cmd)
	}
}

func (h *queueNavigationHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.batches++
		h.roundTrips++
		for _, cmd := range cmds {
			h.commands = append(h.commands, cmd.Name())
		}
		return next(ctx, cmds)
	}
}

func queueNavigationApp(client sidekiq.API) App {
	details := views.NewQueueDetails(client)
	details.SetSize(100, 29) // 25 visible job rows, with a 75-job fetch window.
	return App{
		keys:      DefaultKeyMap(),
		viewStack: []viewID{viewQueuesList},
		viewRegistry: map[viewID]views.View{
			viewQueuesList:   views.NewQueuesList(client),
			viewQueueDetails: details,
		},
		dialogs: stubDialogs{},
	}
}

func queueNavigationUpdate(t testing.TB, app App, msg tea.Msg) App {
	t.Helper()
	updated, cmd := app.Update(msg)
	app = updated.(App)
	for cmd != nil {
		msg = cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			// Run the fetch, leaving the spinner's recurring timer to Bubble Tea.
			cmd = batch[len(batch)-1]
			continue
		}
		data, ok := msg.(lazytable.DataMsg)
		if !ok || data.Err != nil {
			t.Fatalf("queue navigation fetch: %T %+v", msg, msg)
		}
		updated, cmd = app.Update(data)
		app = updated.(App)
	}
	return app
}

func TestAppQueueDetailsEntryReusesMetadata(t *testing.T) {
	mr := miniredis.RunT(t)
	_, _ = mr.SetAdd("queues", "alpha", "beta")
	for i := range 150 {
		_, _ = mr.Lpush("queue:beta", fmt.Sprintf(`{"jid":"job-%03d","class":"TestJob"}`, i))
	}
	client, err := sidekiq.NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Redis().Ping(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	hook := &queueNavigationHook{}
	client.AddHook(hook)
	app := queueNavigationUpdate(t, queueNavigationApp(client), views.ShowQueueDetailsMsg{QueueName: "beta"})
	if hook.batches != 1 {
		t.Fatalf("entry metadata batches = %d, want 1", hook.batches)
	}
	for _, key := range []string{"G", "g"} {
		*hook = queueNavigationHook{}
		app = queueNavigationUpdate(t, app, tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		if hook.batches != 0 || !slices.Equal(hook.commands, []string{"llen", "lrange"}) {
			t.Fatalf("%s refetched metadata: commands=%v batches=%d", key, hook.commands, hook.batches)
		}
		provider := app.viewRegistry[viewQueueDetails].(views.ContextProvider)
		if !slices.ContainsFunc(provider.ContextItems(), func(item views.ContextItem) bool {
			return item.Label == "Queue" && item.Value == "beta"
		}) {
			t.Fatal("navigation changed the selected queue")
		}
	}
}
