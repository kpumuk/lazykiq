package histogram

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestViewDimensions(t *testing.T) {
	tests := map[string]struct {
		width     int
		height    int
		totals    []int64
		labels    []string
		emptyMsg  string
		wantEmpty bool
		fullWidth bool
	}{
		"too narrow":     {width: 1, height: 5, totals: []int64{1}, emptyMsg: "empty", wantEmpty: true},
		"too short":      {width: 10, height: 1, totals: []int64{1}, emptyMsg: "empty", wantEmpty: true},
		"no data":        {width: 20, height: 5, totals: nil, emptyMsg: "empty", wantEmpty: false, fullWidth: false},
		"zero data":      {width: 20, height: 5, totals: []int64{0, 0}, emptyMsg: "empty", wantEmpty: false, fullWidth: false},
		"valid data":     {width: 30, height: 6, totals: []int64{2, 4, 6, 8}, labels: []string{"A", "B", "C", "D"}, emptyMsg: "empty", wantEmpty: false, fullWidth: true},
		"label trimming": {width: 30, height: 6, totals: []int64{2, 4}, labels: []string{"A", "B", "C"}, emptyMsg: "empty", wantEmpty: false, fullWidth: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(
				WithSize(tc.width, tc.height),
				WithData(tc.totals, tc.labels),
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

func TestEmptyMessageRendered(t *testing.T) {
	m := New(
		WithSize(20, 5),
		WithEmptyMessage("no data"),
	)
	output := ansi.Strip(m.View())
	if !strings.Contains(output, "no data") {
		t.Fatalf("expected empty message to be rendered, got %q", output)
	}
}

func TestGoldenHistogram(t *testing.T) {
	m := New(
		WithSize(32, 6),
		WithData([]int64{4, 8, 2, 6, 10}, []string{"0-1", "1-2", "2-3", "3-4", "4-5"}),
		WithEmptyMessage("no data"),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenHistogramNoLabels(t *testing.T) {
	m := New(
		WithSize(28, 2),
		WithData([]int64{3, 1, 2}, []string{"A", "B", "C"}),
		WithEmptyMessage("no data"),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
