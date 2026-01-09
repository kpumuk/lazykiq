package metrics

import (
	"fmt"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func testStyles() Styles {
	return Styles{
		Bar:   lipgloss.NewStyle(),
		Fill:  lipgloss.NewStyle(),
		Label: lipgloss.NewStyle(),
		Value: lipgloss.NewStyle(),
	}
}

func testData() Data {
	return Data{
		Processed: 1,
		Failed:    22,
		Busy:      333,
		Enqueued:  4444,
		Retries:   55555,
		Scheduled: 6,
		Dead:      7777777,
	}
}

func TestViewDimensions(t *testing.T) {
	data := testData()
	cases := map[string]struct {
		width     int
		wantEmpty bool
	}{
		"zero width": {width: 0, wantEmpty: true},
		"narrow":     {width: 60, wantEmpty: false},
		"wide":       {width: 120, wantEmpty: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := New(
				WithStyles(testStyles()),
				WithWidth(tc.width),
				WithData(data),
			)
			output := m.View()
			if tc.wantEmpty {
				if output != "" {
					t.Fatalf("expected empty output, got %q", output)
				}
				return
			}
			if w := ansi.StringWidth(output); w != tc.width {
				t.Fatalf("expected width %d, got %d", tc.width, w)
			}
			if m.Height() != 1 {
				t.Fatalf("expected height 1, got %d", m.Height())
			}
		})
	}
}

func TestGoldenMetricsWidths(t *testing.T) {
	data := testData()
	for _, width := range []int{99, 101, 116, 118, 120} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			m := New(
				WithStyles(testStyles()),
				WithWidth(width),
				WithData(data),
			)
			output := ansi.Strip(m.View())
			golden.RequireEqual(t, []byte(output))
		})
	}
}
