package views

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

type sortedJobsView struct {
	detailListView
	jobs       []*sidekiq.SortedEntry
	firstEntry *sidekiq.SortedEntry
	lastEntry  *sidekiq.SortedEntry
}

func newSortedJobsView(
	title string,
	columns []table.Column,
	emptyMessage string,
	windowPages int,
	fallbackPageSize int,
) sortedJobsView {
	return sortedJobsView{
		detailListView: newDetailListView(
			title,
			columns,
			emptyMessage,
			windowPages,
			fallbackPageSize,
		),
	}
}

func (v *sortedJobsView) handleSortedEntriesData(msg lazytable.DataMsg) (bool, tea.Cmd) {
	return v.handleData(msg, func(result lazytable.FetchResult) {
		if payload, ok := result.Payload.(sortedEntriesPayload); ok {
			v.jobs = payload.jobs
			v.firstEntry = payload.firstEntry
			v.lastEntry = payload.lastEntry
		}
	})
}

func (v *sortedJobsView) resetSortedJobs(updateEmptyMessage func()) {
	v.jobs = nil
	v.firstEntry = nil
	v.lastEntry = nil
	v.resetShell()
	updateEmptyMessage()
}

func (v *sortedJobsView) selectedSortedEntry() (*sidekiq.SortedEntry, bool) {
	idx := v.lazy.Table().Cursor()
	if idx < 0 || idx >= len(v.jobs) {
		return nil, false
	}
	return v.jobs[idx], true
}

func (v sortedJobsView) renderSortedJobsBox(title string) string {
	return v.renderBox(title, len(v.jobs))
}

func (v sortedJobsView) jobName(entry *sidekiq.SortedEntry) string {
	if name := entry.DisplayClass(); name != "" {
		return name
	}
	return "selected"
}
