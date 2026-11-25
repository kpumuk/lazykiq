package views

import (
	"time"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

// NewDead creates the view of dead (morgue) jobs.
func NewDead(client sidekiq.API) *SortedJobs {
	return newSortedJobs(client, &deadSpec)
}

var deadSpec = sortedJobsSpec{
	name:             "Dead",
	title:            "Dead Jobs",
	tracker:          "dead",
	kind:             sidekiq.SortedSetDead,
	columns:          deadJobColumns,
	emptyMessage:     "No dead jobs",
	windowPages:      3,
	fallbackPageSize: 25,
	cells: func(styles Styles, now time.Time, job *sidekiq.SortedEntry) []string {
		return []string{
			entryAge(now, job),
			styles.QueueText.Render(job.Queue()),
			job.DisplayClass(),
			display.ArgsPreview(job.DisplayArgs()),
			errorDisplay(job),
		}
	},
	contextItems: func(now time.Time, first, last *sidekiq.SortedEntry) []ContextItem {
		return []ContextItem{
			{Label: "Last failed", Value: entryAge(now, last)},
			{Label: "Oldest failed", Value: entryAge(now, first)},
		}
	},
	actions: []sortedJobAction{
		sortedDeleteJobAction,
		sortedRetryJobAction,
		{
			key: "ctrl+d", helpKey: "ctrl+d", help: "delete all", op: "deleteAllCmd",
			title:   "Delete all dead",
			message: "Are you sure you want to delete all dead jobs?\n\nThis action is not recoverable.",
			runAll:  sidekiq.API.DeleteAllSortedEntries,
		},
		{
			key: "ctrl+r", helpKey: "ctrl+r", help: "retry all", op: "retryAllCmd",
			title:   "Retry all dead",
			message: "Retry all dead jobs now?\n\nThis will enqueue them immediately.",
			runAll:  sidekiq.API.EnqueueAllSortedEntries,
		},
	},
}

// Table columns for dead job list.
var deadJobColumns = []table.Column{
	{Title: "Last Retry", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}
