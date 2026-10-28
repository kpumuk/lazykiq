package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func TestGoldenBusyHeader(t *testing.T) {
	for name, size := range map[string]int{"empty": 0, "one": 1, "five": 5, "many": 12} {
		t.Run(name, func(t *testing.T) {
			processes := busyHeaderProcesses(size)
			for i := range processes {
				processes[i].StartedAt = time.Time{}
			}
			view := NewBusy(nil)
			view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes}})
			golden.RequireEqual(t, []byte(ansi.Strip(strings.Join(view.HeaderLines(), "\n"))))
		})
	}
}

func TestBusyHeaderSelection(t *testing.T) {
	for name, tc := range map[string]struct {
		size     int
		selected int
		first    int
		last     int
	}{
		"empty":             {selected: -1, first: -1, last: -1},
		"one":               {size: 1, selected: -1, first: 0, last: 0},
		"all processes":     {size: 1000, selected: -1, first: 0, last: 4},
		"early selection":   {size: 1000, selected: 2, first: 0, last: 4},
		"shortcut nine":     {size: 1000, selected: 8, first: 4, last: 8},
		"process list pick": {size: 1000, selected: 999, first: 995, last: 999},
	} {
		t.Run(name, func(t *testing.T) {
			view := NewBusy(nil)
			processes := busyHeaderProcesses(tc.size)
			view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes}})
			if tc.selected >= 0 {
				view.SetProcessIdentity(processes[tc.selected].Identity)
			}
			lines := view.HeaderLines()
			if len(lines) != 5 || cap(lines) != 5 {
				t.Fatalf("header size/capacity = %d/%d; want 5/5", len(lines), cap(lines))
			}
			for row, line := range lines {
				index := tc.first + row
				if tc.first < 0 || index > tc.last {
					if line != "" {
						t.Fatalf("unused header row is not blank: %q", line)
					}
					continue
				}
				if !strings.Contains(ansi.Strip(line), processes[index].Hostname) {
					t.Fatalf("row %d does not contain process %d: %q", row, index, line)
				}
				if index >= 9 && strings.Contains(line, "ctrl+") {
					t.Fatalf("header advertises an unsupported shortcut: %q", line)
				}
			}
		})
	}
}

func TestBusyHeaderInvalidation(t *testing.T) {
	view := NewBusy(nil)
	view.SetSize(160, 40)
	processes := busyHeaderProcesses(10)
	view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes}})
	now := time.Now()
	original := strings.Join(view.processListLines(now), "\n")
	header := view.processHeader
	if got := testing.AllocsPerRun(100, func() { view.processListLines(now) }); got != 0 {
		t.Fatalf("cached header allocated %v times", got)
	}
	if view.processHeader != header {
		t.Fatal("unchanged header rebuilt fleet metadata")
	}

	for name, invalidate := range map[string]func(){
		"width":  func() { view.SetSize(80, 40) },
		"height": func() { view.SetSize(80, 20) },
		"styles": func() {
			styles := view.styles
			styles.Text = styles.Text.Italic(true)
			view.SetStyles(styles)
		},
		"tree mode": func() {
			view.treeMode = !view.treeMode
			view.updateTableRows()
		},
		"selection": func() { view.SetProcessIdentity(processes[8].Identity) },
	} {
		t.Run(name, func(t *testing.T) {
			invalidate()
			if view.processHeader != header {
				t.Fatal("header invalidation rebuilt fleet metadata")
			}
			if header.lines != nil {
				t.Fatal("header invalidation left stale rendered rows")
			}
			view.processListLines(now)
		})
	}

	processes[0].Hostname = "replacement-with-a-wider-hostname"
	processes[0].Busy = 1234
	view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes}})
	view.SetProcessIdentity("")
	changed := strings.Join(view.processListLines(now), "\n")
	if view.processHeader == header || changed == original || !strings.Contains(changed, processes[0].Hostname) || !strings.Contains(changed, "1234/25") {
		t.Fatalf("new process data did not replace the header: %q", changed)
	}
	wide := view.processHeader.maxNameLen
	view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes[1:2]}})
	view.processListLines(now)
	if view.processHeader.maxNameLen >= wide {
		t.Fatal("removed outlier still determines the header width")
	}
	view.Update(busyDataMsg{})
	if got := strings.Join(view.HeaderLines(), ""); got != "" || view.selectedProcess != -1 {
		t.Fatalf("empty update retained a process header: %q", got)
	}
	view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes}})
	view.HeaderLines()
	view.Dispose()
	if view.processHeader != nil {
		t.Fatal("disposed view retains its header")
	}
	if got := strings.Join(view.HeaderLines(), ""); got != "" {
		t.Fatalf("disposed header is not empty: %q", got)
	}
}

func TestBusyHeaderAgeUpdates(t *testing.T) {
	now := time.Unix(1000, 0)
	view := NewBusy(nil)
	view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: []sidekiq.Process{{Identity: "worker:1:nonce", StartedAt: now.Add(-9 * time.Second)}}}})
	first := strings.Join(view.processListLines(now), "\n")
	second := strings.Join(view.processListLines(now.Add(time.Second)), "\n")
	if !strings.Contains(first, "9s") || !strings.Contains(second, "10s") {
		t.Fatalf("uptime did not advance: %q -> %q", first, second)
	}
}

func TestGoldenBusyHeaderSelected(t *testing.T) {
	for name, selected := range map[string]int{"shortcut": 8, "process list": 11} {
		t.Run(name, func(t *testing.T) {
			processes := busyHeaderProcesses(12)
			for i := range processes {
				processes[i].StartedAt = time.Time{}
			}
			view := NewBusy(nil)
			view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes}})
			view.SetProcessIdentity(processes[selected].Identity)
			golden.RequireEqual(t, []byte(ansi.Strip(strings.Join(view.HeaderLines(), "\n"))))
		})
	}
}

func BenchmarkBusyHeaderAgeTick(b *testing.B) {
	for _, size := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprintf("processes_%d", size), func(b *testing.B) {
			view := NewBusy(nil)
			view.SetSize(160, 40)
			view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: busyHeaderProcesses(size)}})
			now := time.Now()
			view.processListLines(now)
			b.ReportAllocs()
			for b.Loop() {
				now = now.Add(time.Second)
				view.processListLines(now)
			}
		})
	}
}

func TestBusyHeaderFormatsOnlyVisibleRows(t *testing.T) {
	for _, size := range []int{10, 1000, 10000} {
		t.Run(fmt.Sprintf("processes_%d", size), func(t *testing.T) {
			view := NewBusy(nil)
			view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: busyHeaderProcesses(size)}})
			nameRenders := 0
			styles := view.styles
			styles.Text = styles.Text.Transform(func(s string) string { nameRenders++; return s })
			view.SetStyles(styles)
			now := time.Now()
			view.processListLines(now)
			if nameRenders != 10 {
				t.Fatalf("formatted %d names; want two passes over five visible rows", nameRenders)
			}
			view.processListLines(now)
			if nameRenders != 10 {
				t.Fatal("unchanged header reformatted process rows")
			}
			view.processListLines(now.Add(time.Second))
			if nameRenders != 20 {
				t.Fatal("uptime change did not format exactly five rows")
			}
		})
	}
}

func TestBusyHeaderTreePreservesProcessData(t *testing.T) {
	view := NewBusy(nil)
	view.SetSize(160, 12)
	processes := busyHeaderProcesses(2)
	processes[0].Tag = "mail"
	processes[0].Status = sidekiq.ProcessStatusStopping
	processes[0].Quiet = true
	processes[0].Capsules = map[string]sidekiq.Capsule{
		"default": {Weights: map[string]int{"default": 1, "critical": 5}},
		"serial":  {Weights: map[string]int{"mailers": 1}},
	}
	view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: processes}})
	view.HeaderLines()
	view.treeMode = true
	view.updateTableRows()
	for range 2 {
		view.HeaderLines()
		output := ansi.Strip(view.View())
		for _, want := range []string{processGlyph, "[mail]", "critical[5]", "default; mailers"} {
			if !strings.Contains(output, want) {
				t.Fatalf("process tree lost %q:\n%s", want, output)
			}
		}
	}
	if !view.data.Processes[0].Quiet || view.data.Processes[0].Status != sidekiq.ProcessStatusStopping {
		t.Fatal("header changed process signal metadata")
	}
}
