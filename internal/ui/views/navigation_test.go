package views

import (
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

func TestErrorNavigationPreservesQuery(t *testing.T) {
	for name, change := range map[string]func(*ErrorsSummary){
		"filter update": func(v *ErrorsSummary) {
			v.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "changed"})
		},
		"dispose": (*ErrorsSummary).Dispose,
	} {
		t.Run(name, func(t *testing.T) {
			v := NewErrorsSummary(nil)
			v.filter = "original"
			v.rows = []sidekiq.ErrorSummaryRow{{ErrorClass: "OriginalError"}}
			v.updateTableRows()
			_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("missing navigation command")
			}
			want := ShowErrorDetailsMsg{Key: errorGroupKeyForRow(v.rows[0]), Query: "original"}
			change(v)
			defer v.Dispose()
			if got := cmd().(ShowErrorDetailsMsg); got != want {
				t.Fatalf("navigation = %+v, want %+v", got, want)
			}
		})
	}
}

func TestFilterNavigationPreservesInputs(t *testing.T) {
	original := Styles{Title: lipgloss.NewStyle().Bold(true)}
	for name, capture := range map[string]func() (tea.Cmd, func()){
		"metrics": func() (tea.Cmd, func()) {
			v := NewMetrics(nil)
			v.filter = "original"
			v.SetStyles(original)
			_, cmd := v.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			return cmd, func() { v.filter = "changed"; v.SetStyles(Styles{}) }
		},
		"errors": func() (tea.Cmd, func()) {
			v := NewErrorsSummary(nil)
			v.filter = "original"
			v.SetStyles(original)
			_, cmd := v.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			return cmd, func() { v.Dispose(); v.SetStyles(Styles{}) }
		},
		"busy": func() (tea.Cmd, func()) {
			v := NewBusy(nil)
			v.filter = "original"
			v.SetStyles(original)
			_, cmd := v.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			return cmd, func() { v.filter = "changed"; v.SetStyles(Styles{}) }
		},
		"detail list": func() (tea.Cmd, func()) {
			v := newDetailListView("test", nil, "empty", 1, 10)
			v.filter = "original"
			v.setStyles(original)
			_, cmd := v.handleKeyPress(tea.KeyPressMsg{Code: '/', Text: "/"}, func() {})
			return cmd, func() { v.filter = "changed"; v.setStyles(Styles{}) }
		},
		"processes": func() (tea.Cmd, func()) {
			v := NewProcessesList(nil)
			v.filter = "original"
			v.SetStyles(original)
			_, cmd := v.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			return cmd, func() { v.filter = "changed"; v.SetStyles(Styles{}) }
		},
		"queues": func() (tea.Cmd, func()) {
			v := NewQueuesList(nil)
			v.filter = "original"
			v.SetStyles(original)
			_, cmd := v.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			return cmd, func() { v.filter = "changed"; v.SetStyles(Styles{}) }
		},
	} {
		t.Run(name, func(t *testing.T) {
			cmd, change := capture()
			if cmd == nil {
				t.Fatal("missing filter command")
			}
			change()
			dialog := cmd().(dialogs.OpenDialogMsg).Model
			dialog.Init()
			dialog.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			want := filterdialog.New(filterdialog.WithStyles(filterDialogStylesFromTheme(original)), filterdialog.WithQuery("original"))
			want.Init()
			want.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			if got := dialog.View(); got != want.View() || !strings.Contains(got, "original") {
				t.Fatalf("delayed filter changed inputs:\n%s\nwant:\n%s", got, want.View())
			}
		})
	}
}

func TestMetricsNavigationPreservesPeriod(t *testing.T) {
	v := NewMetrics(nil)
	defer v.Dispose()
	v.rows = []metricsRow{{class: "OriginalWorker"}}
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing navigation command")
	}
	for range 3 {
		v.Update(tea.KeyPressMsg{Code: '}', Text: "}"})
	}
	v.rows[0].class = "ChangedWorker"
	v.Dispose()
	if got := cmd().(ShowJobMetricsMsg); got.Job != "OriginalWorker" || got.Period != "1h" {
		t.Fatalf("navigation = %+v, want OriginalWorker at 1h", got)
	}
}

func TestMetricsNavigationPeriodConcurrentUpdate(t *testing.T) {
	v := NewMetrics(nil)
	defer v.Dispose()
	v.rows = []metricsRow{{class: "Worker"}}
	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("missing navigation command")
	}
	start, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		<-start
		for range 10000 {
			if got := cmd().(ShowJobMetricsMsg); got.Period != "1h" {
				t.Errorf("command period = %q, want 1h", got.Period)
				return
			}
			runtime.Gosched()
		}
	}()
	close(start)
	for range 10000 {
		v.Update(tea.KeyPressMsg{Code: '}', Text: "}"})
		runtime.Gosched()
		v.Update(tea.KeyPressMsg{Code: '{', Text: "{"})
	}
	<-done
}
