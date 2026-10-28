package views

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

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
			})
		}
	}
}
