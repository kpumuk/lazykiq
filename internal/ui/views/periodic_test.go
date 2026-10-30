package views

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

func periodicFixture() periodicDataMsg {
	job := sidekiq.PeriodicJob{LID: strings.Repeat("abc12345", 8), Version: "v1", Class: "CleanupJob", Schedule: "*/5 * * * *", Options: `{"queue":"low","args":["sessions",7],"retry":0}`}
	paused := sidekiq.PeriodicJob{LID: "paused", Version: "v1", Class: "ReportJob", Schedule: "0 9 * * 1-5", Timezone: "America/Toronto", Paused: true, Options: `{"queue":"reports","args":[]}`}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	return periodicDataMsg{jobs: []sidekiq.PeriodicJob{job, paused}, job: &job, now: now, history: []sidekiq.PeriodicExecution{
		{JID: "abcdef012345678901234567", EnqueuedAt: now.Add(-time.Minute)},
		{JID: "111111111111111111111111", EnqueuedAt: now.Add(-6 * time.Minute)},
	}}
}

func periodicKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(text)[0], Text: text}
}

func periodicTestClient(t *testing.T) (*miniredis.Miniredis, *sidekiq.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	capabilities, err := sidekiq.ResolveFeatures([]string{"ent"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := sidekiq.NewClient("redis://"+mr.Addr(), sidekiq.WithFeatures(capabilities))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	data := periodicFixture()
	_ = mr.Set("periodic-version", data.job.Version)
	for _, job := range data.jobs {
		_, _ = mr.SetAdd("loops-v1", job.LID)
		mr.HSet("loop-"+job.LID, "class", job.Class, "schedule", job.Schedule, "options", job.Options, "tz_name", job.Timezone)
		if job.Paused {
			mr.HSet("loop-"+job.LID, "paused", "1")
		}
	}
	return mr, client
}

func TestPeriodicFilteringAndNavigation(t *testing.T) {
	p := NewPeriodic(nil)
	p.Update(periodicFixture())
	p.table.SetCursor(1)
	p.Update(periodicFixture())
	if row := p.table.SelectedRow(); row.ID != "paused" {
		t.Fatal("refresh lost stable selection")
	}
	_, open := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if msg := open().(ShowPeriodicDetailMsg); msg.LID != "paused" {
		t.Fatalf("opened wrong job: %+v", msg)
	}
	p.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "sessions"})
	if len(p.rows) != 1 || p.rows[0].Class != "CleanupJob" {
		t.Fatalf("filter = %+v", p.rows)
	}
	p.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "missing"})
	if _, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("opened nonexistent filtered row")
	}
	p.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if p.filter != "" || len(p.rows) != 2 {
		t.Fatal("reset did not restore rows")
	}
	p.Update(periodicKey("]"))
	if p.table.Cursor() != 1 {
		t.Fatal("page shortcut did not move selection")
	}
	p.Update(periodicKey("["))
	if p.table.Cursor() != 0 {
		t.Fatal("page-up shortcut did not move selection")
	}
}

func TestGoldenPeriodicDetailHeader(t *testing.T) {
	for name, test := range map[string]struct {
		paused             bool
		timezone, fallback string
		wantZone, wantNext string
		now                time.Time
	}{
		"active":            {wantZone: "UTC", wantNext: "2026-10-31 02:30:00 UTC (in 14h30m)"},
		"paused":            {paused: true, wantZone: "UTC", wantNext: "2026-10-31 02:30:00 UTC (in 14h30m)"},
		"explicit timezone": {timezone: "America/Toronto", fallback: "Asia/Tokyo", wantZone: "America/Toronto", wantNext: "2026-10-31 02:30:00 EDT (in 18h30m)"},
		"server timezone":   {fallback: "Asia/Tokyo", wantZone: "Asia/Tokyo", wantNext: "2026-10-31 02:30:00 JST (in 5h30m)"},
		"minutes and seconds": {
			wantZone: "UTC", wantNext: "2026-10-30 02:30:00 UTC (in 33m24s)",
			now: time.Date(2026, 10, 30, 1, 56, 36, 0, time.UTC),
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := NewPeriodicDetail(nil)
			data := periodicFixture()
			data.job.Schedule = "30 2 * * *"
			data.job.Paused = test.paused
			data.job.Timezone, data.job.DefaultTimezone = test.timezone, test.fallback
			if !test.now.IsZero() {
				data.now = test.now
			}
			p.Update(data)
			items := p.ContextItems()
			values := make(map[string]string, len(items))
			for _, item := range items {
				values[item.Label] = ansi.Strip(item.Value)
			}
			if values["Schedule"] != "30 2 * * * ["+test.wantZone+"]" || values["Next Run"] != test.wantNext {
				t.Fatalf("wrong schedule or next run: %+v", values)
			}
			if _, exists := values["State"]; exists {
				t.Fatal("header still contains a State row")
			}
			if _, exists := values["Timezone"]; exists {
				t.Fatal("header still contains a Timezone row")
			}
			if marked := strings.Contains(values["Class"], "  Paused"); marked != test.paused {
				t.Fatalf("wrong paused marker: %q", values["Class"])
			}
			golden.RequireEqual(t, []byte(ansi.Strip(renderContextBar(items, p.HintBindings()))))
		})
	}
}

func TestPeriodicDangerGates(t *testing.T) {
	for name, test := range map[string]struct{ feature, danger bool }{
		"readonly": {feature: true}, "no feature": {danger: true}, "neither": {},
	} {
		t.Run(name, func(t *testing.T) {
			p := NewPeriodic(nil)
			if test.feature {
				p.capabilities, _ = sidekiq.ResolveFeatures([]string{"ent"})
			}
			p.SetDangerousActionsEnabled(test.danger)
			p.Update(periodicFixture())
			for _, key := range []string{"e", "p"} {
				if _, cmd := p.Update(periodicKey(key)); cmd != nil {
					t.Fatal("mutation offered without both gates")
				}
			}
			if len(p.MutationBindings()) != 0 {
				t.Fatal("mutation hints visible without both gates")
			}
		})
	}
}

func TestPeriodicConfirmedActions(t *testing.T) {
	for name, detail := range map[string]bool{"list": false, "detail": true} {
		t.Run(name, func(t *testing.T) {
			mr, client := periodicTestClient(t)
			p := NewPeriodic(client)
			if detail {
				p = NewPeriodicDetail(client)
				p.SetPeriodicJob(periodicFixture().job.LID)
			}
			p.SetDangerousActionsEnabled(true)
			p.Update(p.Init()())
			headerItemCount := len(p.ContextItems())
			_, open := p.Update(periodicKey("p"))
			if open == nil {
				t.Fatal("missing pause confirmation")
			}
			dialog := open().(dialogs.OpenDialogMsg).Model
			dialog.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			if !strings.Contains(ansi.Strip(dialog.View()), "Pause periodic job") {
				t.Fatal("wrong confirmation")
			}
			target := p.actions.pending.target
			p.Update(confirmdialog.ActionMsg{Target: target})
			if mr.HGet("loop-"+periodicFixture().job.LID, "paused") != "" {
				t.Fatal("cancel wrote pause state")
			}
			p.Update(periodicKey("p"))
			target = p.actions.pending.target
			if _, cmd := p.Update(confirmdialog.ActionMsg{Target: "wrong", Confirmed: true}); cmd != nil {
				t.Fatal("accepted unrelated confirmation")
			}
			stale := p.fetch()()
			// Selection changes after opening must not change the action's target.
			p.table.SetCursor(1)
			_, mutate := p.Update(confirmdialog.ActionMsg{Target: target, Confirmed: true})
			if mutate == nil {
				t.Fatal("confirmation ignored")
			}
			p.Update(stale)
			if _, duplicate := p.Update(confirmdialog.ActionMsg{Target: target, Confirmed: true}); duplicate != nil {
				t.Fatal("duplicate confirmation accepted")
			}
			_, refresh := p.Update(mutate())
			if refresh == nil {
				t.Fatal("mutation did not request readback")
			}
			p.Update(refresh())
			p.Update(stale)
			if mr.HGet("loop-"+periodicFixture().job.LID, "paused") != "1" {
				t.Fatal("wrong definition paused")
			}
			p.table.SetCursor(0)
			job, ok := p.selectedJob()
			if !ok || !job.Paused {
				t.Fatal("stale read overwrote pause readback")
			}
			p.Update(periodicKey("p"))
			_, resume := p.Update(confirmdialog.ActionMsg{Target: p.actions.pending.target, Confirmed: true})
			_, refresh = p.Update(resume())
			p.Update(refresh())
			p.Update(periodicKey("e"))
			_, enqueue := p.Update(confirmdialog.ActionMsg{Target: p.actions.pending.target, Confirmed: true})
			_, refresh = p.Update(enqueue())
			p.Update(refresh())
			if jobs, _ := mr.List("queue:low"); len(jobs) != 1 || !strings.Contains(jobs[0], "CleanupJob") {
				t.Fatalf("enqueued = %v", jobs)
			}
			if mr.HGet("loop-"+periodicFixture().job.LID, "paused") != "" || mr.Exists("loop-history-"+periodicFixture().job.LID) {
				t.Fatal("enqueue changed history or pause state")
			}
			if len(p.ContextItems()) != headerItemCount {
				t.Fatal("action added a view-specific header row")
			}
		})
	}
}

func TestPeriodicDisposeRejectsOldCompletions(t *testing.T) {
	_, client := periodicTestClient(t)
	p := NewPeriodicDetail(client)
	p.SetDangerousActionsEnabled(true)
	p.SetPeriodicJob(periodicFixture().job.LID)
	p.Update(p.Init()())
	stale := p.fetch()()
	p.Update(periodicKey("p"))
	target := p.actions.pending.target
	_, mutation := p.Update(confirmdialog.ActionMsg{Target: target, Confirmed: true})
	p.SetPeriodicJob("paused")
	p.Update(p.Init()())
	p.Update(stale)
	p.Update(mutation())
	if p.job == nil || p.job.LID != "paused" || p.actions.busy {
		t.Fatal("old completion changed new detail state")
	}
	if _, cmd := p.Update(confirmdialog.ActionMsg{Target: target, Confirmed: true}); cmd != nil {
		t.Fatal("disposed confirmation accepted")
	}
}

func TestPeriodicHiddenActions(t *testing.T) {
	_, client := periodicTestClient(t)
	p := NewPeriodic(client)
	p.SetDangerousActionsEnabled(true)
	p.Update(p.Init()())
	p.Update(periodicKey("p"))
	target := p.actions.pending.target
	p.CancelRequests()
	if _, cmd := p.Update(confirmdialog.ActionMsg{Target: target, Confirmed: true}); cmd != nil {
		t.Fatal("hidden confirmation was accepted")
	}
	if _, open := p.Update(periodicKey("p")); open == nil {
		t.Fatal("hidden confirmation blocked future actions")
	}
	_, mutate := p.Update(confirmdialog.ActionMsg{Target: p.actions.pending.target, Confirmed: true})
	if cmd := p.Init(); cmd != nil || !p.ready || !p.actions.busy {
		t.Fatal("reselecting a busy view restarted loading")
	}
	p.CancelRequests()
	if p.actions.busy {
		t.Fatal("hidden mutation blocked navigation")
	}
	p.Update(p.Init()()) // Read before the old mutation has completed.
	_, refresh := p.Update(mutate())
	if refresh == nil {
		t.Fatal("late mutation did not refresh a reopened view")
	}
	p.Update(refresh())
	if !p.rows[0].Paused {
		t.Fatal("late mutation left stale data")
	}
}

func TestPeriodicDimensions(t *testing.T) {
	for _, detail := range []bool{false, true} {
		for _, width := range []int{-1, 0, 1, 3, 20, 69, 70, 80, 160} {
			for _, height := range []int{-1, 0, 1, 2, 3, 5, 20} {
				p := NewPeriodic(nil)
				if detail {
					p = NewPeriodicDetail(nil)
				}
				p.SetSize(width, height)
				p.Update(periodicFixture())
				p.Update(periodicKey("G"))
				for range 2 {
					p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
					output := p.View()
					if width <= 0 || height <= 0 {
						if output != "" {
							t.Fatal("nonpositive dimensions rendered content")
						}
						continue
					}
					if lines := strings.Split(output, "\n"); len(lines) != height {
						t.Fatalf("detail=%v %dx%d: got %d lines", detail, width, height, len(lines))
					}
					for line := range strings.SplitSeq(output, "\n") {
						if ansi.StringWidth(line) > width {
							t.Fatalf("detail=%v %dx%d: width overflow %d", detail, width, height, ansi.StringWidth(line))
						}
					}
				}
			}
		}
	}
}

func TestGoldenPeriodic(t *testing.T) {
	for name, test := range map[string]struct {
		detail, empty, loading, bottom bool
		emptyHistory                   bool
		focusHistory                   bool
		width, height                  int
	}{
		"list": {width: 160, height: 12}, "list narrow": {width: 60, height: 10},
		"empty": {empty: true, width: 80, height: 10}, "loading": {loading: true, width: 80, height: 10},
		"detail": {detail: true, width: 140, height: 22}, "detail bottom": {detail: true, bottom: true, width: 140, height: 16},
		"detail narrow": {detail: true, width: 60, height: 24}, "removed": {detail: true, empty: true, width: 80, height: 10},
		"detail narrow history": {detail: true, focusHistory: true, width: 60, height: 24},
		"empty history":         {detail: true, emptyHistory: true, width: 100, height: 18},
	} {
		t.Run(name, func(t *testing.T) {
			p := NewPeriodic(nil)
			if test.detail {
				p = NewPeriodicDetail(nil)
			}
			p.SetSize(test.width, test.height)
			if !test.loading {
				data := periodicFixture()
				if test.empty {
					data = periodicDataMsg{}
				}
				if test.emptyHistory {
					data.history = nil
				}
				p.Update(data)
			}
			if test.bottom {
				p.Update(periodicKey("G"))
			}
			if test.focusHistory {
				p.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			}
			golden.RequireEqual(t, []byte(ansi.Strip(p.View())))
		})
	}
}
