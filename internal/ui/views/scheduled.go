package views

import (
	"time"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

// NewScheduled creates the view of jobs scheduled for future execution.
func NewScheduled(client sidekiq.API) *SortedJobs {
	return newSortedJobs(client, &scheduledSpec)
}

var scheduledSpec = sortedJobsSpec{
	name:             "Scheduled",
	title:            "Scheduled",
	tracker:          "scheduled",
	kind:             sidekiq.SortedSetScheduled,
	columns:          scheduledJobColumns,
	emptyMessage:     "No scheduled jobs",
	windowPages:      3,
	fallbackPageSize: 25,
	cells: func(styles Styles, now time.Time, job *sidekiq.SortedEntry) []string {
		return []string{
			entryWait(now, job),
			styles.QueueText.Render(job.Queue()),
			job.DisplayClass(),
			display.ArgsPreview(job.DisplayArgs()),
		}
	},
	contextItems: func(now time.Time, first, last *sidekiq.SortedEntry) []ContextItem {
		return []ContextItem{
			{Label: "Next scheduled in", Value: entryWait(now, first)},
			{Label: "Latest scheduled in", Value: entryWait(now, last)},
		}
	},
	actions: []sortedJobAction{
		sortedDeleteJobAction,
		{
			key: "R", helpKey: "shift+r", help: "add to queue", op: "addToQueueJobCmd",
			title:    "Add to queue",
			message:  "Add the %s job to the queue now?\n\nThis will enqueue it immediately.",
			runEntry: sidekiq.API.EnqueueSortedEntry,
		},
		{
			key: "ctrl+d", helpKey: "ctrl+d", help: "delete all", op: "deleteAllCmd",
			title:   "Delete all scheduled",
			message: "Are you sure you want to delete all scheduled jobs?\n\nThis action is not recoverable.",
			runAll:  sidekiq.API.DeleteAllSortedEntries,
		},
		{
			key: "ctrl+r", helpKey: "ctrl+r", help: "add all to queue", op: "addAllToQueueCmd",
			title:   "Add all to queue",
			message: "Add all scheduled jobs to the queue now?\n\nThis will enqueue them immediately.",
			runAll:  sidekiq.API.EnqueueAllSortedEntries,
		},
	},
}

// Table columns for scheduled job list.
var scheduledJobColumns = []table.Column{
	{Title: "When", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 60},
}
