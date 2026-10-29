package views

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

type metricsClientStub struct {
	sidekiq.API
	periodOrder     []string
	result          sidekiq.MetricsTopJobsResult
	err             error
	requestedPeriod sidekiq.MetricsPeriod
	requestedFilter string
	requestContext  context.Context
}

func (m *metricsClientStub) MetricsPeriodOrder(context.Context) []string {
	return append([]string(nil), m.periodOrder...)
}

func (m *metricsClientStub) GetMetricsTopJobs(
	ctx context.Context,
	period sidekiq.MetricsPeriod,
	classFilter string,
) (sidekiq.MetricsTopJobsResult, error) {
	m.requestedPeriod = period
	m.requestedFilter = classFilter
	m.requestContext = ctx
	if m.err != nil {
		return sidekiq.MetricsTopJobsResult{}, m.err
	}
	return m.result, nil
}

func TestMetricsDisposeReleasesSnapshot(t *testing.T) {
	client := &metricsClientStub{result: sidekiq.MetricsTopJobsResult{
		Jobs: map[string]sidekiq.MetricsJobTotals{"OldJob": {Processed: 12, Seconds: 5.2}},
	}}
	m := NewMetrics(client)
	m.SetSize(120, 12)
	m.SetStyles(Styles{})
	m.Update(m.Init()())
	if !m.ready || len(m.table.Rows()) == 0 {
		t.Fatal("fixture did not load metrics")
	}
	disposable, ok := any(m).(Disposable)
	if !ok {
		t.Fatal("Metrics must implement Disposable")
	}
	m.result.StartsAt = time.Now()
	m.result.EndsAt = m.result.StartsAt.Add(time.Hour)
	m.resetScroll = true
	disposable.Dispose()
	disposable.Dispose()
	if m.ready || m.result.Jobs != nil || m.rows != nil || m.table.Rows() != nil || m.resetScroll {
		t.Fatal("disposed Metrics retains its snapshot")
	}
	if !m.result.StartsAt.IsZero() || !m.result.EndsAt.IsZero() || m.table.Cursor() != 0 || m.table.YOffset() != 0 {
		t.Fatal("disposal retained range or scroll state")
	}
	if client.requestContext.Err() != context.Canceled {
		t.Fatal("disposal did not cancel the request")
	}
}

func TestMetricsCancelRequestsRejectsLateReplies(t *testing.T) {
	client := &metricsClientStub{result: sidekiq.MetricsTopJobsResult{
		Jobs: map[string]sidekiq.MetricsJobTotals{"OldJob": {Processed: 12, Seconds: 5.2}},
	}}
	m := NewMetrics(client)
	late := m.Init()()
	m.CancelRequests()
	m.Update(late)
	if m.ready || len(m.result.Jobs) != 0 || len(m.table.Rows()) != 0 {
		t.Fatal("canceled request installed stale metrics")
	}
}

func TestMetricsRequestOwnership(t *testing.T) {
	for name, invalidate := range map[string]func(*Metrics) tea.Cmd{
		"refresh": func(m *Metrics) tea.Cmd {
			_, cmd := m.Update(RefreshMsg{})
			return cmd
		},
		"filter": func(m *Metrics) tea.Cmd {
			_, cmd := m.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "New"})
			return cmd
		},
		"period": func(m *Metrics) tea.Cmd {
			_, cmd := m.Update(tea.KeyPressMsg{Code: '}', Text: "}"})
			return cmd
		},
		"hidden": func(m *Metrics) tea.Cmd {
			m.CancelRequests()
			return m.Init()
		},
		"disposed": func(m *Metrics) tea.Cmd {
			m.Dispose()
			return m.Init()
		},
	} {
		t.Run(name, func(t *testing.T) {
			client := &metricsClientStub{result: sidekiq.MetricsTopJobsResult{
				Jobs: map[string]sidekiq.MetricsJobTotals{"OldJob": {Processed: 1}},
			}}
			m := NewMetrics(client)
			t.Cleanup(m.Dispose)
			stale := m.Init()()
			oldContext := client.requestContext
			current := invalidate(m)
			if !errors.Is(oldContext.Err(), context.Canceled) {
				t.Fatal("previous request remains active")
			}
			m.Update(stale)
			if m.ready || m.result.Jobs != nil || len(m.rows) != 0 || len(m.table.Rows()) != 0 {
				t.Fatal("stale reply installed metrics before the new reply")
			}
			client.result.Jobs = map[string]sidekiq.MetricsJobTotals{"NewJob": {Processed: 2}}
			m.Update(current())
			m.Update(stale)
			if !m.ready || len(m.rows) != 1 || m.rows[0].class != "NewJob" {
				t.Fatal("stale reply replaced the new snapshot")
			}
		})
	}
}

func TestMetricsCanceledCommandDoesNotFetch(t *testing.T) {
	client := &metricsClientStub{}
	m := NewMetrics(client)
	stale := m.Init()
	current := m.Init()
	defer m.Dispose()
	if msg := stale(); msg != nil || client.requestContext != nil {
		t.Fatal("superseded command fetched metrics")
	}
	m.Dispose()
	if msg := current(); msg != nil || client.requestContext != nil {
		t.Fatal("disposed command fetched metrics")
	}
}

type blockingMetricsClient struct {
	sidekiq.API
	started chan context.Context
	release chan struct{}
	err     error
}

func (*blockingMetricsClient) MetricsPeriodOrder(context.Context) []string {
	return []string{"1h"}
}

func (c *blockingMetricsClient) GetMetricsTopJobs(ctx context.Context, _ sidekiq.MetricsPeriod, _ string) (sidekiq.MetricsTopJobsResult, error) {
	c.started <- ctx
	<-c.release // Deliberately return a reply even if the context was canceled.
	return sidekiq.MetricsTopJobsResult{Jobs: map[string]sidekiq.MetricsJobTotals{"LateJob": {Processed: 1}}}, c.err
}

func TestMetricsDisposeCancelsInFlightRequest(t *testing.T) {
	for name, err := range map[string]error{"success": nil, "failure": errors.New("connection lost")} {
		t.Run(name, func(t *testing.T) {
			client := &blockingMetricsClient{started: make(chan context.Context, 1), release: make(chan struct{}), err: err}
			m := NewMetrics(client)
			cmd := m.Init()
			done := make(chan tea.Msg, 1)
			go func() { done <- cmd() }()
			var ctx context.Context
			select {
			case ctx = <-client.started:
			case <-time.After(time.Second):
				close(client.release)
				t.Fatal("fetch did not start")
			}
			m.Dispose()
			close(client.release)
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatal("in-flight request was not canceled")
			}
			select {
			case msg := <-done:
				if msg != nil {
					t.Fatalf("disposed request returned %T", msg)
				}
			case <-time.After(time.Second):
				t.Fatal("fetch did not stop")
			}
		})
	}
}

func TestMetricsFailedRefreshAndReentry(t *testing.T) {
	client := &metricsClientStub{result: sidekiq.MetricsTopJobsResult{
		Jobs: map[string]sidekiq.MetricsJobTotals{"OldJob": {Processed: 1}},
	}}
	m := NewMetrics(client)
	t.Cleanup(m.Dispose)
	m.Update(m.Init()())
	client.err = errors.New("metrics unavailable")
	_, refresh := m.Update(RefreshMsg{})
	if msg, ok := refresh().(ConnectionErrorMsg); !ok || !errors.Is(msg.Err, client.err) {
		t.Fatal("refresh did not report its connection error")
	}
	if !m.ready || len(m.rows) != 1 {
		t.Fatal("failed refresh discarded the active snapshot")
	}
	m.Dispose()
	if _, ok := m.Init()().(ConnectionErrorMsg); !ok {
		t.Fatal("reentry did not report its connection error")
	}
	if m.ready || m.result.Jobs != nil || m.rows != nil || len(m.table.Rows()) != 0 {
		t.Fatal("failed reentry restored disposed data")
	}
	client.err = nil
	client.result.Jobs = map[string]sidekiq.MetricsJobTotals{"RecoveredJob": {Processed: 2}}
	_, refresh = m.Update(RefreshMsg{})
	m.Update(refresh())
	if !m.ready || len(m.rows) != 1 || m.rows[0].class != "RecoveredJob" {
		t.Fatal("refresh did not recover after failed reentry")
	}
}

func TestMetricsFetchListCmd_MVUSafeStateFlow(t *testing.T) {
	client := &metricsClientStub{
		periodOrder: []string{"1h", "2h", "4h", "8h"},
		result: sidekiq.MetricsTopJobsResult{
			Jobs: map[string]sidekiq.MetricsJobTotals{
				"EmailJob": {Processed: 12, Failed: 1, Seconds: 5.2},
			},
		},
	}
	m := NewMetrics(client)
	m.periods = []string{"1h", "2h", "4h", "8h", "24h"}
	m.period = "24h"
	m.periodIdx = 4
	m.filter = "email"

	cmd := m.fetchListCmd()
	if cmd == nil {
		t.Fatal("fetchListCmd returned nil")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("fetch command returned nil message")
	}

	// Command execution must not mutate model state directly.
	if m.period != "24h" {
		t.Fatalf("period mutated in command: got %q, want %q", m.period, "24h")
	}
	if m.periodIdx != 4 {
		t.Fatalf("periodIdx mutated in command: got %d, want %d", m.periodIdx, 4)
	}
	if !slices.Equal(m.periods, []string{"1h", "2h", "4h", "8h", "24h"}) {
		t.Fatalf("periods mutated in command: got %v", m.periods)
	}

	// The command should still query with a valid period for detected Sidekiq version.
	if client.requestedPeriod != sidekiq.MetricsPeriods["1h"] {
		t.Fatalf("requested period = %+v, want %+v", client.requestedPeriod, sidekiq.MetricsPeriods["1h"])
	}
	if client.requestedFilter != "email" {
		t.Fatalf("requested filter = %q, want %q", client.requestedFilter, "email")
	}

	updated, _ := m.Update(msg)
	got, ok := updated.(*Metrics)
	if !ok {
		t.Fatalf("updated view type = %T, want *Metrics", updated)
	}

	if got.period != "1h" {
		t.Fatalf("period after Update = %q, want %q", got.period, "1h")
	}
	if got.periodIdx != 0 {
		t.Fatalf("periodIdx after Update = %d, want %d", got.periodIdx, 0)
	}
	if !slices.Equal(got.periods, []string{"1h", "2h", "4h", "8h"}) {
		t.Fatalf("periods after Update = %v, want %v", got.periods, []string{"1h", "2h", "4h", "8h"})
	}
	if !got.ready {
		t.Fatal("ready should be true after metrics list message")
	}
}

func TestNormalizeMetricsPeriods_FallbackToDefaults(t *testing.T) {
	got := normalizeMetricsPeriods([]string{"", "invalid", "24h", "24h"})
	want := []string{"24h"}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeMetricsPeriods(...) = %v, want %v", got, want)
	}

	got = normalizeMetricsPeriods(nil)
	if !slices.Equal(got, sidekiq.MetricsPeriodOrder) {
		t.Fatalf("normalizeMetricsPeriods(nil) = %v, want %v", got, sidekiq.MetricsPeriodOrder)
	}
}

func metricsListFixture(width, height int, filter string, jobs map[string]sidekiq.MetricsJobTotals) *Metrics {
	m := NewMetrics(nil)
	m.SetStyles(Styles{})
	m.SetSize(width, height)
	m.filter = filter
	m.Update(metricsListMsg{
		period: "8h",
		result: sidekiq.MetricsTopJobsResult{Jobs: jobs},
	})
	return m
}

func TestMetricsListOrderAndDimensions(t *testing.T) {
	jobs := map[string]sidekiq.MetricsJobTotals{
		"EmailJob":  {Processed: 20, Failed: 2, Milliseconds: 5000, Seconds: 5},
		"ReportJob": {Processed: 10, Failed: 1, Milliseconds: 10000, Seconds: 10},
	}
	for name, size := range map[string][2]int{
		"narrow": {60, 8}, "wide": {120, 12}, "zero width": {0, 12}, "zero height": {120, 0},
	} {
		t.Run(name, func(t *testing.T) {
			m := metricsListFixture(size[0], size[1], "", jobs)
			before := ansi.Strip(m.View())
			m.client = &metricsClientStub{periodOrder: []string{"8h"}, result: sidekiq.MetricsTopJobsResult{Jobs: jobs}}
			m.Dispose()
			m.SetStyles(Styles{})
			m.SetSize(size[0], size[1])
			m.Update(m.Init()())
			defer m.Dispose()
			if got := ansi.Strip(m.View()); got != before {
				t.Fatal("reentry changed the list layout")
			}
			if len(m.rows) != 2 || m.rows[0].class != "ReportJob" || m.rows[1].class != "EmailJob" {
				t.Fatalf("jobs are not ordered by descending execution time: %+v", m.rows)
			}
			if m.rows[0].totals != jobs["ReportJob"] || m.rows[1].totals != jobs["EmailJob"] {
				t.Fatal("building rows changed the totals")
			}
			if count, succeeded, failed := m.aggregateTotals(); count != 2 || succeeded != 27 || failed != 3 {
				t.Fatalf("context totals = %d/%d/%d, want 2/27/3", count, succeeded, failed)
			}
			output := ansi.Strip(m.View())
			if size[0] == 0 || size[1] == 0 {
				if output != "" {
					t.Fatal("zero-sized view should be empty")
				}
				return
			}
			lines := strings.Split(output, "\n")
			if len(lines) != size[1] {
				t.Fatalf("height = %d, want %d", len(lines), size[1])
			}
			for _, line := range lines {
				if width := ansi.StringWidth(line); width != size[0] {
					t.Fatalf("width = %d, want %d", width, size[0])
				}
			}
			if !strings.Contains(output, "ReportJob") || !strings.Contains(output, "EmailJob") {
				t.Fatal("job rows missing from view")
			}
		})
	}
}

func TestGoldenMetricsList(t *testing.T) {
	for name, spec := range map[string]struct {
		filter string
		jobs   map[string]sidekiq.MetricsJobTotals
	}{
		"empty":      {},
		"no matches": {filter: "missing"},
		"matching filter": {
			filter: "EMAIL",
			jobs: map[string]sidekiq.MetricsJobTotals{
				"App::EmailJob": {Processed: 48000, Failed: 480, Milliseconds: 96000, Seconds: 96},
			},
		},
		"multiple jobs": {
			jobs: map[string]sidekiq.MetricsJobTotals{
				"EmailJob":  {Processed: 20, Failed: 2, Milliseconds: 5000, Seconds: 5},
				"ReportJob": {Processed: 10, Failed: 1, Milliseconds: 10000, Seconds: 10},
			},
		},
	} {
		for _, width := range []int{100, 120} {
			t.Run(fmt.Sprintf("%s/width %d", name, width), func(t *testing.T) {
				m := metricsListFixture(width, 10, spec.filter, spec.jobs)
				golden.RequireEqual(t, []byte(ansi.Strip(m.View())))

				m.client = &metricsClientStub{
					periodOrder: []string{"8h"},
					result:      sidekiq.MetricsTopJobsResult{Jobs: spec.jobs},
				}
				m.Dispose()
				m.SetStyles(Styles{})
				m.SetSize(width, 10)
				m.Update(m.Init()())
				defer m.Dispose()
				golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
			})
		}
	}
}
