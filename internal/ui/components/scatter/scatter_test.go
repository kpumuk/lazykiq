package scatter

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/ui/charts"
)

func TestViewDimensions(t *testing.T) {
	buckets := []time.Time{
		time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 12, 10, 0, 0, time.UTC),
	}
	points := []charts.ScatterPoint{
		{X: 0, Y: 0, Count: 1},
		{X: 1, Y: 1, Count: 3},
		{X: 2, Y: 2, Count: 8},
	}

	tests := map[string]struct {
		width     int
		height    int
		points    []charts.ScatterPoint
		emptyMsg  string
		wantEmpty bool
		fullWidth bool
	}{
		"too narrow": {width: 1, height: 6, points: points, emptyMsg: "empty", wantEmpty: true},
		"too short":  {width: 20, height: 1, points: points, emptyMsg: "empty", wantEmpty: true},
		"no data":    {width: 20, height: 6, points: nil, emptyMsg: "empty", wantEmpty: false, fullWidth: false},
		"valid":      {width: 40, height: 6, points: points, emptyMsg: "empty", wantEmpty: false, fullWidth: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(
				WithSize(tc.width, tc.height),
				WithData(tc.points, buckets, []string{"0", "1", "2"}, 10, 2),
				WithEmptyMessage(tc.emptyMsg),
			)
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

func TestScatterRune(t *testing.T) {
	tests := map[string]struct {
		count    int64
		maxCount int64
		want     rune
	}{
		"zero max": {count: 0, maxCount: 0, want: '·'},
		"negative": {count: -1, maxCount: 10, want: '·'},
		"low":      {count: 1, maxCount: 10, want: '◦'},
		"high":     {count: 10, maxCount: 10, want: '●'},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := scatterRune(tc.count, tc.maxCount); got != tc.want {
				t.Fatalf("scatterRune() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGoldenScatter(t *testing.T) {
	buckets := []time.Time{
		time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 12, 10, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC),
	}
	points := []charts.ScatterPoint{
		{X: 0, Y: 0, Count: 1},
		{X: 1, Y: 1, Count: 3},
		{X: 2, Y: 2, Count: 6},
		{X: 3, Y: 0, Count: 10},
	}

	m := New(
		WithSize(40, 6),
		WithData(points, buckets, []string{"0", "1", "2"}, 10, 2),
		WithEmptyMessage("no data"),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
