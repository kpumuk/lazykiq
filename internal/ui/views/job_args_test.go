package views

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestJobsTableNestedArgumentsRemainCompleteInDetail(t *testing.T) {
	args := []any{map[string]any{"a": []any{"hello\n\"<>&", strings.Repeat("👩🏽‍💻1️⃣", 512)}, "z": "TAIL"}}
	raw, err := json.Marshal(map[string]any{"jid": "nested", "class": "TestJob", "args": args})
	if err != nil {
		t.Fatal(err)
	}
	job := sidekiq.NewJobRecord(string(raw), "default")
	rows := NewQueueDetails(nil).buildRows([]*sidekiq.PositionedEntry{{JobRecord: job, Position: 1}})
	preview := rows[0].Cells[2]
	if ansi.StringWidth(preview) > 200 || !strings.HasSuffix(preview, "…") || strings.Contains(preview, "TAIL") {
		t.Fatalf("unexpected argument preview: %q", preview)
	}
	view := NewJobDetail()
	view.SetJob(job)
	var copied struct{ Args []any }
	if err := json.Unmarshal([]byte(view.jobJSON()), &copied); err != nil {
		t.Fatal(err)
	}
	want := display.Args(args)
	if display.Args(copied.Args) != want {
		t.Fatal("copy JSON lost arguments")
	}
	for _, property := range view.properties {
		if property.Label == "Args" {
			if property.Value != want {
				t.Fatal("job detail lost arguments")
			}
			return
		}
	}
	t.Fatal("job detail has no arguments")
}

func TestGoldenJobsTableUnicodeArgumentPreview(t *testing.T) {
	view := NewQueueDetails(nil)
	jobs := make([]*sidekiq.PositionedEntry, 0, 2)
	for i, args := range [][]any{
		{strings.Repeat("👩🏽‍💻🇨🇦", 512)},
		{map[string]any{"a": []any{"hello\n\"<>&", strings.Repeat("界é", 512)}, "z": "TAIL"}},
	} {
		raw, err := json.Marshal(map[string]any{"jid": fmt.Sprintf("job%d", i), "class": "ReportJob", "args": args})
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, &sidekiq.PositionedEntry{JobRecord: sidekiq.NewJobRecord(string(raw), "default"), Position: i + 1})
	}
	rows := view.buildRows(jobs)
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

func BenchmarkBusyRowsArgumentPreview(b *testing.B) {
	for _, size := range []int{1024, 65536} {
		b.Run(fmt.Sprintf("jobs=75/arg_bytes=%d", size), func(b *testing.B) {
			view := NewBusy(nil)
			view.SetSize(160, 40)
			view.data.Processes = []sidekiq.Process{{Identity: "worker:1:nonce", Hostname: "worker", PID: 1, Concurrency: 100, Busy: 75}}
			for i := range 75 {
				raw := fmt.Sprintf(`{"jid":"job-%d","class":"WideJob","queue":"default","args":["%s"]}`, i, strings.Repeat("x", size))
				job := sidekiq.NewJobRecord(raw, "default")
				job.DisplayArgs() // Exclude lazy JSON decoding, as in the retained audit fixture.
				view.data.Jobs = append(view.data.Jobs, sidekiq.Job{
					JobRecord: job, ProcessIdentity: "worker:1:nonce", ThreadID: strconv.Itoa(i), RunAt: time.Now(),
				})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				view.updateTableRows()
			}
		})
	}
}
