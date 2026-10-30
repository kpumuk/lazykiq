package views

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

type periodicAction struct {
	job    sidekiq.PeriodicJob
	verb   string
	target string
}

type periodicActions struct {
	pending    *periodicAction
	busy       bool
	generation uint64
}

// PeriodicActionResultMsg reports mutation errors even after navigation.
type PeriodicActionResultMsg struct {
	owner      *periodicActions
	generation uint64
	Err        error
}

func (a *periodicActions) dialog(job sidekiq.PeriodicJob, enqueue bool, styles Styles, intents IntentScope) tea.Cmd {
	if a.busy || a.pending != nil {
		return nil
	}
	verb, title, consequence := "pause", "Pause periodic job", "Future recurring enqueues will stop.\nJobs already queued or running are unaffected."
	if job.Paused {
		verb, title, consequence = "resume", "Resume periodic job", "Future jobs will be enqueued on this schedule again."
	}
	if enqueue {
		verb, title, consequence = "enqueue", "Enqueue periodic job", "One job will be added to its queue immediately.\nThe recurring schedule and paused state are unaffected."
	}
	a.generation++
	action := periodicAction{job: job, verb: verb, target: fmt.Sprintf("periodic:%p:%d:%s", a, a.generation, job.LID)}
	a.pending = &action
	return intents.EmitIntent(dialogs.OpenDialogMsg{Model: newConfirmDialog(styles, title,
		fmt.Sprintf("%s %s?\n\nSchedule: %s\nQueue: %s\n\n%s", title, styles.Text.Bold(true).Render(job.Class), job.Schedule, job.Queue(), consequence),
		action.target, styles.DangerAction)})
}

func (a *periodicActions) confirm(parent context.Context, msg confirmdialog.ActionMsg, client sidekiq.API) tea.Cmd {
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
	ctx := devtools.WithTracker(parent, "periodic.mutate")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var err error
		if action.verb == "enqueue" {
			_, err = client.EnqueuePeriodicJob(ctx, action.job)
		} else {
			err = client.SetPeriodicJobPaused(ctx, action.job, action.verb == "pause")
		}
		return PeriodicActionResultMsg{owner: a, generation: generation, Err: err}
	}
}

func (a *periodicActions) complete(msg PeriodicActionResultMsg) bool {
	if !a.busy || msg.owner != a || msg.generation != a.generation {
		return false
	}
	a.busy = false
	return true
}

func (a *periodicActions) reset() {
	a.pending = nil
	a.busy = false
	a.generation++
}
