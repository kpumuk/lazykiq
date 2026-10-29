package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type navigationMetricsClient struct {
	sidekiq.API
	ctx    context.Context
	period sidekiq.MetricsPeriod
	filter string
	calls  int
	err    error
}

func (*navigationMetricsClient) MetricsPeriodOrder(context.Context) []string {
	return []string{"1h", "2h", "4h", "8h"}
}

func (c *navigationMetricsClient) GetMetricsTopJobs(ctx context.Context, period sidekiq.MetricsPeriod, filter string) (sidekiq.MetricsTopJobsResult, error) {
	c.ctx, c.period, c.filter = ctx, period, filter
	c.calls++
	return sidekiq.MetricsTopJobsResult{Jobs: map[string]sidekiq.MetricsJobTotals{
		fmt.Sprintf("FreshJob%d", c.calls): {Processed: int64(c.calls)},
	}}, c.err
}

func updateMetricsApp(app *App, msg tea.Msg) tea.Cmd {
	updated, cmd := app.Update(msg)
	*app = updated.(App)
	return cmd
}

func metricsAppKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: string(code)}
}

func requireMetricsLoading(t *testing.T, app *App) {
	t.Helper()
	m := app.viewRegistry[viewMetrics]
	if !strings.Contains(ansi.Strip(m.View()), "Loading...") {
		t.Fatal("disposed Metrics does not show its loading state")
	}
	for _, item := range m.(views.ContextProvider).ContextItems() {
		if item.Value != "-" {
			t.Fatalf("disposed Metrics exposes %s=%s", item.Label, item.Value)
		}
	}
}

func TestAppMetricsNavigationDisposesAndRefetches(t *testing.T) {
	client := &navigationMetricsClient{}
	app := New(client, "test", false, nil)
	updateMetricsApp(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
	m := app.viewRegistry[viewMetrics]
	disposable, ok := m.(views.Disposable)
	if !ok {
		t.Fatal("Metrics must implement Disposable")
	}
	t.Cleanup(disposable.Dispose)
	var stale tea.Msg
	for round := range 3 {
		cmd := updateMetricsApp(&app, metricsAppKey('8'))
		if cmd == nil || app.activeViewID() != viewMetrics {
			t.Fatal("Metrics entry did not start a fetch")
		}
		requireMetricsLoading(t, &app)
		if stale != nil {
			updateMetricsApp(&app, stale)
			requireMetricsLoading(t, &app)
		}
		updateMetricsApp(&app, cmd())
		if round == 0 {
			cmd = updateMetricsApp(&app, filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "tenant"})
			updateMetricsApp(&app, cmd())
			cmd = updateMetricsApp(&app, metricsAppKey('}'))
			updateMetricsApp(&app, cmd())
		}
		if client.period != sidekiq.MetricsPeriods["2h"] || client.filter != "tenant" {
			t.Fatalf("preferences changed on entry: period=%+v filter=%q", client.period, client.filter)
		}
		currentJob := fmt.Sprintf("FreshJob%d", client.calls)
		if stale != nil {
			updateMetricsApp(&app, stale)
		}
		if !strings.Contains(ansi.Strip(m.View()), currentJob) {
			t.Fatal("late reply replaced the current snapshot")
		}
		stale = updateMetricsApp(&app, views.RefreshMsg{})()
		ctx := client.ctx
		updateMetricsApp(&app, metricsAppKey('1'))
		if app.activeViewID() != viewDashboard || !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("navigation did not hide and cancel Metrics")
		}
		updateMetricsApp(&app, stale)
		updateMetricsApp(&app, tea.WindowSizeMsg{Width: 100 + round*10, Height: 24 + round})
		m.SetStyles(views.Styles{TableHeader: lipgloss.NewStyle().Bold(true)})
		requireMetricsLoading(t, &app)
		if app.viewRegistry[viewMetrics] != m {
			t.Fatal("navigation replaced the Metrics instance")
		}
	}
}

func TestAppMetricsFailedRefreshAndReentry(t *testing.T) {
	client := &navigationMetricsClient{}
	app := New(client, "test", false, nil)
	updateMetricsApp(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
	updateMetricsApp(&app, updateMetricsApp(&app, metricsAppKey('8'))())
	m := app.viewRegistry[viewMetrics]
	t.Cleanup(m.(views.Disposable).Dispose)
	client.err = errors.New("metrics unavailable")
	updateMetricsApp(&app, updateMetricsApp(&app, views.RefreshMsg{})())
	if !errors.Is(app.connectionError, client.err) || !strings.Contains(ansi.Strip(m.View()), "FreshJob1") {
		t.Fatal("failed refresh did not preserve the active snapshot and report the error")
	}
	updateMetricsApp(&app, metricsAppKey('1'))
	requireMetricsLoading(t, &app)
	updateMetricsApp(&app, updateMetricsApp(&app, metricsAppKey('8'))())
	requireMetricsLoading(t, &app)
	client.err = nil
	updateMetricsApp(&app, updateMetricsApp(&app, views.RefreshMsg{})())
	if !strings.Contains(ansi.Strip(m.View()), "FreshJob4") {
		t.Fatal("refresh did not recover after failed reentry")
	}
}

func TestAppMetricsStackPreservesParentSnapshot(t *testing.T) {
	client := &navigationMetricsClient{}
	app := New(client, "test", false, nil)
	updateMetricsApp(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
	updateMetricsApp(&app, updateMetricsApp(&app, metricsAppKey('8'))())
	m := app.viewRegistry[viewMetrics]
	t.Cleanup(m.(views.Disposable).Dispose)
	stale := updateMetricsApp(&app, views.RefreshMsg{})()
	ctx := client.ctx
	updateMetricsApp(&app, views.ShowJobMetricsMsg{Job: "FreshJob1", Period: "1h"})
	if !errors.Is(ctx.Err(), context.Canceled) || app.activeViewID() != viewJobMetrics {
		t.Fatal("stack entry did not cancel the parent request")
	}
	updateMetricsApp(&app, tea.KeyPressMsg{Code: tea.KeyEscape})
	updateMetricsApp(&app, stale)
	if app.activeViewID() != viewMetrics || !strings.Contains(ansi.Strip(m.View()), "FreshJob1") {
		t.Fatal("stack return lost the parent snapshot or applied a canceled reply")
	}
	updateMetricsApp(&app, updateMetricsApp(&app, views.RefreshMsg{})())
	if !strings.Contains(ansi.Strip(m.View()), "FreshJob3") {
		t.Fatal("stack return did not allow a fresh request")
	}
}
