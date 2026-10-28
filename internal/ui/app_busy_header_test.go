package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type busyHeaderClient struct {
	sidekiq.API
	data sidekiq.BusyData
}

func (c busyHeaderClient) GetBusyData(context.Context, string) (sidekiq.BusyData, error) {
	return c.data, nil
}

func busyHeaderApp(size int) App {
	processes := make([]sidekiq.Process, size)
	for i := range processes {
		processes[i] = sidekiq.Process{Identity: fmt.Sprintf("worker-%05d:12345:nonce", i), Hostname: fmt.Sprintf("worker-%05d", i), PID: 12345, Busy: 3, Concurrency: 25}
	}
	app := New(busyHeaderClient{data: sidekiq.BusyData{Processes: processes}}, "", false, nil)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	app = updated.(App)
	busy := app.viewRegistry[viewBusy].(*views.Busy)
	msg := busy.Init()()
	app.viewStack = []viewID{viewBusy}
	updated, _ = app.Update(msg)
	return updated.(App)
}

func BenchmarkAppBusyHeaderRender(b *testing.B) {
	for _, size := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprintf("processes_%d", size), func(b *testing.B) {
			app := busyHeaderApp(size)
			b.Cleanup(app.viewRegistry[viewBusy].(*views.Busy).CancelRequests)
			b.ReportAllocs()
			for b.Loop() {
				app.syncContextbar()
				if app.View().Content == "" {
					b.Fatal("empty app render")
				}
			}
		})
	}
}

func TestGoldenAppBusyHeader(t *testing.T) {
	for name, tc := range map[string]struct {
		processes int
		width     int
	}{
		"empty":  {width: 80},
		"one":    {processes: 1, width: 80},
		"wide":   {processes: 1000, width: 160},
		"narrow": {processes: 1000, width: 80},
		"tiny":   {processes: 10, width: 40},
	} {
		t.Run(name, func(t *testing.T) {
			app := busyHeaderApp(tc.processes)
			t.Cleanup(app.viewRegistry[viewBusy].(*views.Busy).CancelRequests)
			updated, _ := app.Update(tea.WindowSizeMsg{Width: tc.width, Height: 24})
			app = updated.(App)
			app.syncContextbar()
			output := ansi.Strip(app.View().Content)
			if got := lipgloss.Height(output); got != 24 {
				t.Fatalf("app height = %d, want 24", got)
			}
			if got := lipgloss.Width(output); got > tc.width {
				t.Fatalf("app width = %d, want <= %d", got, tc.width)
			}
			golden.RequireEqual(t, []byte(output))
		})
	}
}

func TestAppBusyHeaderSelectedProcess(t *testing.T) {
	app := busyHeaderApp(12)
	busy := app.viewRegistry[viewBusy].(*views.Busy)
	t.Cleanup(busy.CancelRequests)
	busy.SetProcessIdentity("worker-00011:12345:nonce")
	app.syncContextbar()
	output := ansi.Strip(app.View().Content)
	if !strings.Contains(output, "worker-00011:12345") || strings.Contains(output, "ctrl+12") {
		t.Fatalf("selected process or shortcuts incorrect:\n%s", output)
	}
	if got := lipgloss.Height(output); got != 40 {
		t.Fatalf("app height = %d, want 40", got)
	}
}
