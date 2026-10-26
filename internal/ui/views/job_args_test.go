package views

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

func TestJobsTableArgumentPreviews(t *testing.T) {
	argument := strings.Repeat("界", 4096) + "tail"
	job := sidekiq.NewJobRecord(fmt.Sprintf(
		`{"jid":"job1","class":"TestJob","queue":"default","args":[%q]}`, argument,
	), "default")
	entries := []*sidekiq.SortedEntry{{JobRecord: job, Score: 1}}
	busyRows := func(tree bool) []table.Row {
		view := NewBusy(nil)
		view.treeMode = tree
		view.data = sidekiq.BusyData{
			Processes: []sidekiq.Process{{Identity: "worker:123:abc"}},
			Jobs:      []sidekiq.Job{{JobRecord: job, ProcessIdentity: "worker:123:abc", ThreadID: "1"}},
		}
		view.SetSize(120, 8)
		view.updateTableRows()
		return view.table.Rows()
	}
	for name, tc := range map[string]struct {
		rows   []table.Row
		column int
	}{
		"queue":     {NewQueueDetails(nil).buildRows([]*sidekiq.PositionedEntry{{JobRecord: job, Position: 1}}), 2},
		"retries":   {NewRetries(nil).buildRows(entries), 4},
		"dead":      {NewDead(nil).buildRows(entries), 3},
		"scheduled": {NewScheduled(nil).buildRows(entries), 3},
		"errors":    {NewErrorsDetails(nil).buildRows([]sidekiq.ErrorGroupEntry{{Entry: entries[0], Source: "retry"}}), 4},
		"busy flat": {busyRows(false), 6},
		"busy tree": {busyRows(true), 5},
	} {
		t.Run(name, func(t *testing.T) {
			if len(tc.rows) == 0 {
				t.Fatal("no job rows")
			}
			row := tc.rows[len(tc.rows)-1]
			preview := row.Cells[tc.column]
			if ansi.StringWidth(preview) > 200 || !strings.HasSuffix(preview, "…") {
				t.Fatalf("arguments preview exceeds 200 columns or lacks ellipsis: %q", preview)
			}
			if row.ID != job.JID() || strings.Contains(preview, "tail") {
				t.Fatal("job identity or preview content is incorrect")
			}
		})
	}

	detail := NewJobDetail()
	detail.SetJob(job)
	for _, property := range detail.properties {
		if property.Label == "Args" {
			if property.Value != display.Args([]any{argument}) {
				t.Fatal("job detail arguments were truncated")
			}
			return
		}
	}
	t.Fatal("job detail has no arguments")
}

func TestGoldenJobsTableArgumentPreview(t *testing.T) {
	view := NewQueueDetails(nil)
	rows := view.buildRows([]*sidekiq.PositionedEntry{
		{
			JobRecord: sidekiq.NewJobRecord(fmt.Sprintf(
				`{"jid":"job1","class":"ReportJob","args":[%q],"cattr":{"user_id":42}}`, strings.Repeat("payload ", 4096),
			), "default"),
			Position: 1,
		},
		{
			JobRecord: sidekiq.NewJobRecord(`{"jid":"job2","class":"EmailJob","args":["hello"]}`, "default"),
			Position:  2,
		},
	})
	for _, width := range []int{120, 300} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			model := table.New(table.WithColumns(queueJobColumns), table.WithRows(rows))
			model.SetSize(width, 5)
			t.Run("start", func(t *testing.T) {
				golden.RequireEqual(t, []byte(ansi.Strip(model.View())))
			})
			model.ScrollToEnd()
			t.Run("end", func(t *testing.T) {
				golden.RequireEqual(t, []byte(ansi.Strip(model.View())))
			})
		})
	}
}

func BenchmarkJobsTableScrollLargeArguments(b *testing.B) {
	jobs := make([]*sidekiq.PositionedEntry, 75)
	for i := range jobs {
		jobs[i] = &sidekiq.PositionedEntry{
			JobRecord: sidekiq.NewJobRecord(fmt.Sprintf(
				`{"jid":"job%d","class":"TestJob","args":[%q]}`, i, strings.Repeat("x", 32*1024),
			), "default"),
			Position: i + 1,
		}
	}
	view := NewQueueDetails(nil)
	rows := view.buildRows(jobs)
	for _, direction := range []string{"down", "left"} {
		b.Run(direction, func(b *testing.B) {
			model := table.New(table.WithColumns(queueJobColumns), table.WithRows(rows))
			model.SetSize(120, 25)
			model.ScrollToEnd()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if direction == "down" {
					model.MoveDown(1)
				} else {
					model.ScrollLeft()
				}
				_ = model.View()
			}
		})
	}
}
