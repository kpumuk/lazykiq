package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func testDashboard() *Dashboard {
	d := NewDashboard(nil)
	base := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	d.realtimeTimes = []time.Time{base, base.Add(5 * time.Second), base.Add(10 * time.Second)}
	d.realtimeProcessed = []int64{5, 12, 3}
	d.realtimeFailed = []int64{0, 2, 1}
	d.lastDeltaP, d.lastDeltaF = 3, 1
	d.lastPollAt = d.realtimeTimes[2]
	d.historyDates = []time.Time{base, base.AddDate(0, 0, 1), base.AddDate(0, 0, 2)}
	d.historyProcessed = []int64{5, 12, 3}
	d.historyFailed = []int64{0, 2, 1}
	return d
}

func TestDashboardViewDimensions(t *testing.T) {
	for name, newDashboard := range map[string]func() *Dashboard{
		"loading":   func() *Dashboard { return NewDashboard(nil) },
		"populated": testDashboard,
	} {
		t.Run(name, func(t *testing.T) {
			for _, width := range []int{-1, 0, 1, 2, 3, 4, 5, 6, 10, 20, 40, 120} {
				for height := -1; height <= 32; height++ {
					t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
						d := newDashboard()
						d.SetSize(width, height)
						output := d.View()
						if width <= 0 || height <= 0 {
							if output != "" {
								t.Fatalf("expected empty output, got %q", output)
							}
							return
						}
						if got := lipgloss.Height(output); got != height {
							t.Fatalf("height = %d, want %d", got, height)
						}
						if got := lipgloss.Width(output); got > width {
							t.Fatalf("width = %d, want <= %d", got, width)
						}
					})
				}
			}
		})
	}
}

func TestGoldenDashboard(t *testing.T) {
	for name, size := range map[string][2]int{
		"normal":      {80, 24},
		"short":       {80, 8},
		"summary":     {80, 6},
		"compact":     {80, 3},
		"single row":  {80, 1},
		"narrow":      {10, 12},
		"short chart": {80, 12},
		"odd height":  {80, 13},
	} {
		t.Run(name, func(t *testing.T) {
			d := testDashboard()
			d.SetSize(size[0], size[1])
			golden.RequireEqual(t, []byte(ansi.Strip(d.View())))
		})
	}
}

func TestDashboardResizePreservesSummaries(t *testing.T) {
	d := testDashboard()
	for _, height := range []int{24, 8, 6, 3, 24} {
		d.SetSize(80, height)
		output := ansi.Strip(d.View())
		for _, want := range []string{"Dashboard", "History", "Processed: 3", "Processed: 20", "Failed: 1", "Failed: 3"} {
			if !strings.Contains(output, want) {
				t.Fatalf("height %d: missing %q:\n%s", height, want, output)
			}
		}
	}
}

func TestDashboardCompactFocus(t *testing.T) {
	d := testDashboard()
	d.SetSize(80, 1)
	if output := d.View(); !strings.Contains(output, "Dashboard: Processed: 3") {
		t.Fatalf("expected realtime summary, got %q", output)
	}
	d.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if output := d.View(); !strings.Contains(output, "History: Processed: 20") {
		t.Fatalf("expected history summary after tab, got %q", output)
	}
}

func TestGoldenDashboardLoading(t *testing.T) {
	for name, height := range map[string]int{"normal": 24, "short": 8, "compact": 3} {
		t.Run(name, func(t *testing.T) {
			d := NewDashboard(nil)
			// Model the initial frame before any data or seeded realtime samples.
			d.width, d.height = 80, height
			golden.RequireEqual(t, []byte(ansi.Strip(d.View())))
		})
	}
}
