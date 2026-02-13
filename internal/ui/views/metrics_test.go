package views

import (
	"context"
	"slices"
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type metricsClientStub struct {
	sidekiq.API
	periodOrder     []string
	result          sidekiq.MetricsTopJobsResult
	err             error
	requestedPeriod sidekiq.MetricsPeriod
	requestedFilter string
}

func (m *metricsClientStub) MetricsPeriodOrder(context.Context) []string {
	return append([]string(nil), m.periodOrder...)
}

func (m *metricsClientStub) GetMetricsTopJobs(
	_ context.Context,
	period sidekiq.MetricsPeriod,
	classFilter string,
) (sidekiq.MetricsTopJobsResult, error) {
	m.requestedPeriod = period
	m.requestedFilter = classFilter
	if m.err != nil {
		return sidekiq.MetricsTopJobsResult{}, m.err
	}
	return m.result, nil
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
