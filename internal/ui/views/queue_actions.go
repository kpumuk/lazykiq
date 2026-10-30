package views

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

type queueAction struct {
	name   string
	verb   string
	target string
}

type queueActions struct {
	pending    *queueAction
	busy       bool
	generation uint64
}

// QueueActionResultMsg reports a completed queue mutation. Err is handled globally
// even when navigation has disposed the owning view in the meantime.
type QueueActionResultMsg struct {
	owner      *queueActions
	generation uint64
	Err        error
}

func (a *queueActions) prepare(name, verb string) (queueAction, bool) {
	if a.busy || a.pending != nil {
		return queueAction{}, false
	}
	a.generation++
	action := queueAction{name: name, verb: verb, target: fmt.Sprintf("queue:%p:%d:%s", a, a.generation, verb)}
	a.pending = &action
	return action, true
}

func (a *queueActions) reset() {
	a.pending = nil
	a.busy = false
	a.generation++
}

func (a *queueActions) pauseDialog(name string, paused bool, styles Styles, intents IntentScope) tea.Cmd {
	verb, title, consequence := "pause", "Pause queue", "Workers will stop fetching new jobs from this queue.\nJobs already running may finish; new jobs can still be enqueued."
	if paused {
		verb, title, consequence = "resume", "Resume queue", "Workers will resume fetching jobs from this queue."
	}
	action, ok := a.prepare(name, verb)
	if !ok {
		return nil
	}
	return intents.EmitIntent(dialogs.OpenDialogMsg{Model: newConfirmDialog(styles, title,
		fmt.Sprintf("%s %s?\n\n%s", title, styles.Text.Bold(true).Render(name), consequence), action.target, styles.NeutralAction)})
}

func (a *queueActions) confirm(parent context.Context, msg confirmdialog.ActionMsg, client sidekiq.API) tea.Cmd {
	if a.busy || a.pending == nil || msg.Target != a.pending.target {
		return nil
	}
	action := *a.pending
	a.pending = nil
	if !msg.Confirmed {
		return nil
	}
	a.busy = true
	a.generation++
	generation := a.generation
	ctx := devtools.WithTracker(parent, "queues.mutate")
	return func() tea.Msg {
		queue := client.NewQueue(action.name)
		var err error
		if action.verb == "delete" {
			err = queue.Clear(ctx)
		} else {
			err = queue.SetPaused(ctx, action.verb == "pause")
		}
		return QueueActionResultMsg{owner: a, generation: generation, Err: err}
	}
}

func (a *queueActions) complete(msg QueueActionResultMsg) bool {
	if !a.busy || msg.owner != a || msg.generation != a.generation {
		return false
	}
	a.busy = false
	return true
}

func queuePauseHelp(paused bool) string {
	if paused {
		return "resume queue"
	}
	return "pause queue"
}

func fetchQueueCapabilities(ctx context.Context, client sidekiq.API) (sidekiq.Capabilities, map[string]bool, error) {
	capabilities := client.GetCapabilities()
	if !capabilities.SupportsQueuePause() {
		return capabilities, nil, nil
	}
	paused, err := client.GetPausedQueues(ctx)
	return capabilities, paused, err
}
