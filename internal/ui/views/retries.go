package views

import (
	"strconv"
	"time"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

// NewRetries creates the view of failed jobs pending retry.
func NewRetries(client sidekiq.API) *SortedJobs {
	return newSortedJobs(client, &retriesSpec)
}

var retriesSpec = sortedJobsSpec{
	name:             "Retries",
	title:            "Retries",
	tracker:          "retries",
	kind:             sidekiq.SortedSetRetry,
	columns:          retryJobColumns,
	emptyMessage:     "No retries",
	windowPages:      3,
	fallbackPageSize: 25,
	cells: func(styles Styles, now time.Time, job *sidekiq.SortedEntry) []string {
		return []string{
			entryAge(now, job),
			strconv.Itoa(job.RetryCount()),
			styles.QueueText.Render(job.Queue()),
			job.DisplayClass(),
			display.ArgsPreview(job.DisplayArgs()),
			errorDisplay(job),
		}
	},
	contextItems: func(now time.Time, first, last *sidekiq.SortedEntry) []ContextItem {
		return []ContextItem{
			{Label: "Next retry in", Value: entryWait(now, first)},
			{Label: "Latest retry in", Value: entryWait(now, last)},
		}
	},
	actions: []sortedJobAction{
		sortedDeleteJobAction,
		{
			key: "K", helpKey: "shift+k", help: "kill job", op: "killJobCmd",
			title:    "Kill job",
			message:  "Are you sure you want to kill the %s job?\n\nThis will move the job to the dead queue.",
			runEntry: sidekiq.API.MoveSortedEntryToDead,
		},
		sortedRetryJobAction,
		{
			key: "ctrl+d", helpKey: "ctrl+d", help: "delete all", op: "deleteAllCmd",
			title:   "Delete all retries",
			message: "Are you sure you want to delete all retry jobs?\n\nThis action is not recoverable.",
			runAll:  sidekiq.API.DeleteAllSortedEntries,
		},
		{
			key: "ctrl+k", helpKey: "ctrl+k", help: "kill all", op: "killAllCmd",
			title:   "Kill all retries",
			message: "Are you sure you want to kill all retry jobs?\n\nThis will move them to the dead queue.",
			runAll:  sidekiq.API.MoveAllSortedEntriesToDead,
		},
		{
			key: "ctrl+r", helpKey: "ctrl+r", help: "retry all", op: "retryAllCmd",
			title:   "Retry all retries",
			message: "Retry all retry jobs now?\n\nThis will enqueue them immediately.",
			runAll:  sidekiq.API.EnqueueAllSortedEntries,
		},
	},
}

// Table columns for retry job list.
var retryJobColumns = []table.Column{
	{Title: "Next Retry", Width: 12},
	{Title: "Retries", Width: 7},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}
