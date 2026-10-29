package views

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/redis/go-redis/v9"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

type queueViewRedisHook struct {
	commands []string
	batches  []int
}

func (h *queueViewRedisHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *queueViewRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.commands = append(h.commands, cmd.Name())
		return next(ctx, cmd)
	}
}

func (h *queueViewRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.batches = append(h.batches, len(cmds))
		for _, cmd := range cmds {
			h.commands = append(h.commands, cmd.Name())
		}
		return next(ctx, cmds)
	}
}

func queueFetchMessage(t *testing.T, cmd tea.Cmd) lazytable.DataMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing fetch command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		// The lazy table batches its spinner and fetch; only run the fetch.
		msg = batch[len(batch)-1]()
	}
	data, ok := msg.(lazytable.DataMsg)
	if !ok || data.Err != nil {
		t.Fatalf("fetch result: %T %+v", msg, msg)
	}
	return data
}

func TestQueueViewsMetadataFanout(t *testing.T) {
	for _, count := range []int{10, 100, 1000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			mr, client := queueTestClient(t)
			for i := range count {
				_, _ = mr.SetAdd("queues", fmt.Sprintf("queue-%04d", i))
			}
			for i := range 150 {
				_, _ = mr.Lpush("queue:queue-0000", fmt.Sprintf(`{"jid":"job-%03d","class":"TestJob"}`, i))
			}
			if err := client.Redis().Ping(t.Context()).Err(); err != nil {
				t.Fatal(err)
			}
			hook := &queueViewRedisHook{}
			client.AddHook(hook)
			list := NewQueuesList(client)
			msg := list.fetchDataCmd()()
			if _, ok := msg.(ReadResultMsg).Data.(queuesListDataMsg); !ok {
				t.Fatalf("list result: %T %+v", msg, msg)
			}
			if len(hook.commands) != 2+2*count || len(hook.batches) != (count+99)/100 {
				t.Fatalf("list commands=%d batches=%v", len(hook.commands), hook.batches)
			}
			*hook = queueViewRedisHook{}
			detail := NewQueueDetails(client)
			data := queueFetchMessage(t, detail.Init())
			detail.Update(data)
			if len(hook.commands) != 4+2*count || len(hook.batches) != (count+99)/100 {
				t.Fatalf("detail commands=%d batches=%v", len(hook.commands), hook.batches)
			}
			for _, batch := range hook.batches {
				if batch > 200 {
					t.Fatalf("unbounded pipeline: %d", batch)
				}
			}
			*hook = queueViewRedisHook{}
			data = queueFetchMessage(t, detail.lazy.RequestWindow(75, lazytable.CursorStart))
			detail.Update(data)
			if !slices.Equal(hook.commands, []string{"llen", "lrange"}) || len(hook.batches) != 0 {
				t.Fatalf("adjacent page refetched metadata: commands=%v batches=%v", hook.commands, hook.batches)
			}
			if len(detail.jobs) != 75 || detail.jobs[0].JID() != "job-074" || detail.jobs[0].Position != 75 {
				t.Fatalf("adjacent page changed job order/positions: %+v", detail.jobs)
			}
			*hook = queueViewRedisHook{}
			detail.Update(queueFetchMessage(t, detail.selectQueue(1)))
			if !slices.Equal(hook.commands, []string{"llen"}) || len(hook.batches) != 0 || detail.selectedQueue != 1 {
				t.Fatalf("queue switch refetched metadata: commands=%v batches=%v selected=%d", hook.commands, hook.batches, detail.selectedQueue)
			}
		})
	}
}

func TestQueueDetailsMetadataRefresh(t *testing.T) {
	mr, client := queueTestClient(t)
	_, _ = mr.SetAdd("queues", "default", "mailers")
	_, _ = mr.Lpush("queue:mailers", `{"jid":"job1","class":"TestJob"}`)
	view := NewQueueDetails(client)
	view.SetQueue("mailers")
	view.Update(queueFetchMessage(t, view.Init()))
	_, _ = mr.SetAdd("queues", "critical")
	_, _ = mr.SetAdd("paused", "mailers")
	_, _ = mr.Lpush("queue:mailers", `{"jid":"job2","class":"TestJob"}`)
	// Paging sees live selected-queue jobs and retains the metadata snapshot.
	view.Update(queueFetchMessage(t, view.lazy.RequestWindow(0, lazytable.CursorStart)))
	if len(view.queues) != 2 || view.queues[1].Size != 1 || view.queues[1].Paused || len(view.jobs) != 2 {
		t.Fatalf("paging snapshot/live jobs: %+v jobs=%d", view.queues, len(view.jobs))
	}
	_, cmd := view.Update(RefreshMsg{})
	view.Update(queueFetchMessage(t, cmd))
	if len(view.queues) != 3 || view.queues[2].Name != "mailers" || view.selectedQueue != 2 || view.queues[2].Size != 2 || !view.queues[2].Paused {
		t.Fatalf("refresh lost membership, selection or pause: %+v selected=%d", view.queues, view.selectedQueue)
	}
	if err := client.Redis().SRem(t.Context(), "queues", "mailers").Err(); err != nil {
		t.Fatal(err)
	}
	_, cmd = view.Update(RefreshMsg{})
	view.Update(queueFetchMessage(t, cmd))
	if len(view.queues) != 2 || view.selectedQueue != 0 || len(view.jobs) != 0 {
		t.Fatalf("deleted queue selection: %+v selected=%d jobs=%d", view.queues, view.selectedQueue, len(view.jobs))
	}
}

func TestQueueDetailsEmptyMetadataRefresh(t *testing.T) {
	mr, client := queueTestClient(t)
	view := NewQueueDetails(client)
	view.Update(queueFetchMessage(t, view.Init()))
	if view.queues == nil || len(view.queues) != 0 {
		t.Fatal("empty snapshot was not retained")
	}
	_, _ = mr.SetAdd("queues", "default")
	_, cmd := view.Update(RefreshMsg{})
	view.Update(queueFetchMessage(t, cmd))
	if len(view.queues) != 1 || view.queues[0].Name != "default" {
		t.Fatalf("empty snapshot hid new queue: %+v", view.queues)
	}
}

func TestQueueDetailsMetadataRequestOwnership(t *testing.T) {
	mr, client := queueTestClient(t)
	_, _ = mr.SetAdd("queues", "default")
	view := NewQueueDetails(client)
	stale := queueFetchMessage(t, view.Init())
	view.CancelRequests()
	view.Update(stale)
	if view.queues != nil {
		t.Fatal("canceled result installed metadata")
	}
	view.Update(queueFetchMessage(t, view.lazy.RequestWindow(0, lazytable.CursorStart)))
	_, cmd := view.Update(RefreshMsg{})
	stale = queueFetchMessage(t, cmd)
	// A refresh during a fetch must leave that result stale for metadata cadence.
	view.Update(RefreshMsg{})
	view.Update(stale)
	if view.metadataLoadedRevision == view.metadataRevision {
		t.Fatal("in-flight metadata swallowed the next refresh")
	}
	_, _ = mr.SetAdd("queues", "new")
	view.Update(queueFetchMessage(t, view.lazy.RequestWindow(0, lazytable.CursorStart)))
	if len(view.queues) != 2 {
		t.Fatal("next page did not fulfill pending metadata refresh")
	}
	view.Dispose()
	view.Update(stale)
	if view.queues != nil || view.ready {
		t.Fatal("disposed view accepted stale metadata")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := view.fetchWindow(ctx, 0, 75, lazytable.CursorStart); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled fetch: %v", err)
	}
}

func TestQueuesListMetadataFilterAndPause(t *testing.T) {
	mr, client := queueTestClient(t)
	_, _ = mr.SetAdd("queues", "zeta", "Default", "default-empty")
	_, _ = mr.SetAdd("paused", "default-empty")
	_, _ = mr.Lpush("queue:Default", `{"jid":"one","enqueued_at":1780000000000}`)
	view := NewQueuesList(client)
	view.filter = "DEFAULT"
	view.Update(view.fetchDataCmd()())
	if len(view.queues) != 2 || view.queues[0].Name != "Default" || view.queues[1].Name != "default-empty" {
		t.Fatalf("filter/order: %+v", view.queues)
	}
	if view.queues[0].Size != 1 || !view.queues[0].HasOldestJob || !view.queues[1].Paused || view.queues[1].HasOldestJob {
		t.Fatalf("queue state: %+v %+v", view.queues[0], view.queues[1])
	}
}

func TestGoldenQueueMetadataSnapshot(t *testing.T) {
	for name, width := range map[string]int{"wide": 140, "narrow": 80} {
		t.Run(name, func(t *testing.T) {
			mr, client := queueTestClient(t)
			_, _ = mr.SetAdd("queues", "critical", "default", "empty")
			_, _ = mr.SetAdd("paused", "critical")
			_, _ = mr.Lpush("queue:critical", `{"jid":"critical1","class":"CriticalJob"}`)
			_, _ = mr.Lpush("queue:default", `{"jid":"default1","class":"DefaultJob","args":[1]}`)
			view := NewQueueDetails(client)
			view.SetStyles(Styles{})
			view.SetSize(width, 12)
			view.SetQueue("default")
			view.Update(queueFetchMessage(t, view.Init()))
			header := view.HeaderLines()
			output := ansi.Strip(view.View())
			if len(header) != 5 || len(strings.Split(output, "\n")) != 12 || !strings.Contains(output, "DefaultJob") {
				t.Fatalf("snapshot layout: %q\n%s", header, output)
			}
			for line := range strings.SplitSeq(output, "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatalf("line exceeds width %d: %q", width, line)
				}
			}
			golden.RequireEqual(t, []byte(ansi.Strip(strings.Join(header, "\n")+"\n"+view.View())))
		})
	}
}

func TestQueueDetailsFetchWindow_FilteredJobs(t *testing.T) {
	mr := miniredis.RunT(t)
	client, err := sidekiq.NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	_, _ = mr.SetAdd("queues", "default")

	entries := []string{
		`{"jid":"job1","class":"TestJob","args":["skip"]}`,
		`{"jid":"job2","class":"TestJob","args":["match older"]}`,
		`{"jid":"job3","class":"TestJob","args":["skip"]}`,
		`{"jid":"job4","class":"TestJob","args":["match middle"]}`,
		`{"jid":"job5","class":"TestJob","args":["match newest"]}`,
	}
	for _, entry := range entries {
		_, _ = mr.Lpush("queue:default", entry)
	}

	view := NewQueueDetails(client)
	view.filter = "match"

	result, err := view.fetchWindow(context.Background(), 1, 1, lazytable.CursorStart)
	if err != nil {
		t.Fatalf("fetchWindow failed: %v", err)
	}

	if result.Total != 3 {
		t.Fatalf("result.Total = %d, want 3", result.Total)
	}
	if result.WindowStart != 1 {
		t.Fatalf("result.WindowStart = %d, want 1", result.WindowStart)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("len(result.Rows) = %d, want 1", len(result.Rows))
	}
	if result.Rows[0].ID != "job4" {
		t.Fatalf("result.Rows[0].ID = %q, want %q", result.Rows[0].ID, "job4")
	}

	payload, ok := result.Payload.(queueDetailsPayload)
	if !ok {
		t.Fatalf("result.Payload type = %T, want queueDetailsPayload", result.Payload)
	}
	if len(payload.jobs) != 1 {
		t.Fatalf("len(payload.jobs) = %d, want 1", len(payload.jobs))
	}
	if got := payload.jobs[0].JID(); got != "job4" {
		t.Fatalf("payload.jobs[0].JID() = %q, want %q", got, "job4")
	}
	if got := payload.jobs[0].Position; got != 4 {
		t.Fatalf("payload.jobs[0].Position = %d, want %d", got, 4)
	}
}

func TestQueueDetailsRenderJobsBoxShowsRowsMetaOnly(t *testing.T) {
	view := NewQueueDetails(nil)
	view.SetSize(140, 30)
	view.SetStyles(Styles{})

	rows := make([]table.Row, 26)
	for i := range rows {
		rows[i] = table.Row{
			ID: "row",
			Cells: []string{
				"14",
				"CleanupJob",
				"{}",
				"",
			},
		}
	}

	updated, _ := view.Update(lazytable.DataMsg{
		RequestID: view.lazy.RequestID(),
		Result: lazytable.FetchResult{
			Rows:        rows,
			Total:       1500,
			WindowStart: 13,
			Payload: queueDetailsPayload{
				queues: []*QueueInfo{
					{Name: "default", Size: 1500},
				},
				jobs:          make([]*sidekiq.PositionedEntry, len(rows)),
				selectedQueue: 0,
			},
		},
	})
	view = updated.(*QueueDetails)

	output := ansi.Strip(view.renderJobsBox())
	if !strings.Contains(output, "rows: 14-39/1,500") {
		t.Fatalf("renderJobsBox() missing rows meta:\n%s", output)
	}
	if strings.Contains(output, "SIZE:") {
		t.Fatalf("renderJobsBox() still shows size meta:\n%s", output)
	}
	if strings.Contains(output, "1.5K") {
		t.Fatalf("renderJobsBox() still shows abbreviated size value:\n%s", output)
	}
}
