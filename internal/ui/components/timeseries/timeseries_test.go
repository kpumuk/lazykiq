package timeseries

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func sampleSeries() []Series {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	return []Series{
		{
			Name:   "A",
			Times:  []time.Time{base, base.Add(5 * time.Minute), base.Add(10 * time.Minute)},
			Values: []float64{1, 2, 3},
			Style:  lipgloss.NewStyle(),
		},
		{
			Name:   "B",
			Times:  []time.Time{base, base.Add(5 * time.Minute), base.Add(10 * time.Minute)},
			Values: []float64{3, 2, 1},
			Style:  lipgloss.NewStyle(),
		},
	}
}

func TestCommonLength(t *testing.T) {
	series := []Series{
		{
			Name:   "A",
			Times:  []time.Time{time.Now(), time.Now()},
			Values: []float64{1, 2, 3},
		},
		{
			Name:   "B",
			Times:  []time.Time{time.Now()},
			Values: []float64{1},
		},
	}
	m := New(WithSeries(series...))
	if got := m.commonLength(); got != 1 {
		t.Fatalf("commonLength() = %d, want 1", got)
	}
}

func TestSetters(t *testing.T) {
	m := New()
	minTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	maxTime := minTime.Add(time.Hour)

	m.SetSeries(sampleSeries()...)
	m.SetXFormatter(func(_ int, _ float64) string { return "x" })
	m.SetYFormatter(func(_ int, _ float64) string { return "y" })
	m.SetXYSteps(3, 4)
	m.SetTimeRange(minTime, maxTime)
	m.SetValueRange(0, 10)
	m.SetEmptyMessage("empty")

	if m.xSteps != 3 || m.ySteps != 4 {
		t.Fatalf("SetXYSteps not applied")
	}
	if m.minTime == nil || m.maxTime == nil {
		t.Fatalf("SetTimeRange not applied")
	}
	if m.minValue == nil || m.maxValue == nil {
		t.Fatalf("SetValueRange not applied")
	}
	if m.emptyMessage != "empty" {
		t.Fatalf("SetEmptyMessage not applied")
	}
}

func TestViewDimensions(t *testing.T) {
	series := sampleSeries()
	tests := map[string]struct {
		width     int
		height    int
		useSeries bool
		wantEmpty bool
		fullWidth bool
	}{
		"zero width":  {width: 0, height: 5, useSeries: true, wantEmpty: true},
		"zero height": {width: 10, height: 0, useSeries: true, wantEmpty: true},
		"no series":   {width: 20, height: 4, useSeries: false, wantEmpty: false, fullWidth: false},
		"valid":       {width: 40, height: 6, useSeries: true, wantEmpty: false, fullWidth: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(WithSize(tc.width, tc.height), WithEmptyMessage("empty"))
			if tc.useSeries {
				m.SetSeries(series...)
			}
			output := m.View()
			if tc.wantEmpty {
				if output != "" {
					t.Fatalf("expected empty output, got %q", output)
				}
				return
			}

			lines := strings.Split(ansi.Strip(output), "\n")
			if len(lines) != tc.height {
				t.Fatalf("expected %d lines, got %d", tc.height, len(lines))
			}
			for i, line := range lines {
				w := ansi.StringWidth(line)
				if tc.fullWidth && w != tc.width {
					t.Fatalf("line %d: expected width %d, got %d", i, tc.width, w)
				}
				if !tc.fullWidth && w > tc.width {
					t.Fatalf("line %d: expected width <= %d, got %d", i, tc.width, w)
				}
			}
		})
	}
}

func TestGoldenTimeseries(t *testing.T) {
	series := sampleSeries()
	minTime := series[0].Times[0]
	maxTime := series[0].Times[len(series[0].Times)-1]

	m := New(
		WithSize(40, 6),
		WithSeries(series...),
		WithXYSteps(2, 2),
		WithXFormatter(func(_ int, _ float64) string { return "" }),
		WithYFormatter(func(_ int, v float64) string { return fmt.Sprintf("%.0f", v) }),
		WithTimeRange(minTime, maxTime),
		WithValueRange(0, 3),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
