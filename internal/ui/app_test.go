package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	devtoolsdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/devtools"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

func appWithInputDialog(t *testing.T, console bool) App {
	t.Helper()
	app := New(nil, "", false, devtools.NewTracker())
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 300, Height: 40})
	app = updated.(App)
	var model dialogs.DialogModel = filterdialog.New()
	if console {
		_, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyF12})
		model = cmd().(dialogs.OpenDialogMsg).Model
	}
	updated, _ = app.Update(dialogs.OpenDialogMsg{Model: model})
	return updated.(App)
}

func TestAppDialogPrintableInput(t *testing.T) {
	for name, console := range map[string]bool{"filter": false, "console": true} {
		t.Run(name, func(t *testing.T) {
			app := appWithInputDialog(t, console)
			var want strings.Builder
			for r := ' '; r <= '~'; r++ {
				if console && r == '~' { // The console deliberately reserves its toggle.
					continue
				}
				want.WriteRune(r)
				updated, _ := app.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
				app = updated.(App)
				output := ansi.Strip(app.dialogs.ActiveModel().View())
				if !strings.Contains(output, want.String()) {
					t.Fatalf("after typing %q, want input %q in:\n%s", r, want.String(), output)
				}
				if app.activeViewID() != viewDashboard || len(app.dialogs.Dialogs()) != 1 {
					t.Fatal("printable input triggered app navigation")
				}
			}
		})
	}
}

func TestAppDialogInterrupt(t *testing.T) {
	for name, tc := range map[string]struct{ console, table bool }{
		"filter": {}, "console input": {console: true}, "console table": {console: true, table: true},
	} {
		t.Run(name, func(t *testing.T) {
			app := appWithInputDialog(t, tc.console)
			if tc.table {
				updated, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
				app = updated.(App)
			}
			_, cmd := app.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			if cmd == nil {
				t.Fatal("Ctrl+C did not return a quit command")
			}
			if msg := cmd(); msg != tea.Quit() {
				t.Fatalf("Ctrl+C returned %T, want tea.QuitMsg", msg)
			}
		})
	}
}

func TestAppDialogCloseKeys(t *testing.T) {
	for name, tc := range map[string]struct {
		console bool
		key     tea.KeyPressMsg
	}{
		"filter escape":  {key: tea.KeyPressMsg{Code: tea.KeyEsc}},
		"console escape": {console: true, key: tea.KeyPressMsg{Code: tea.KeyEsc}},
		"console f12":    {console: true, key: tea.KeyPressMsg{Code: tea.KeyF12}},
		"console tilde":  {console: true, key: tea.KeyPressMsg{Code: '~', Text: "~"}},
	} {
		t.Run(name, func(t *testing.T) {
			for _, table := range []bool{false, true} {
				if table && !tc.console {
					continue
				}
				app := appWithInputDialog(t, tc.console)
				app.viewStack = []viewID{viewDashboard, viewJobDetail}
				if table {
					updated, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
					app = updated.(App)
				}
				updated, cmd := app.Update(tc.key)
				app = updated.(App)
				if cmd == nil {
					t.Fatal("missing close command")
				}
				msg := cmd()
				if _, ok := msg.(dialogs.CloseDialogMsg); !ok {
					t.Fatalf("close key returned %T", msg)
				}
				updated, _ = app.Update(msg)
				app = updated.(App)
				if app.dialogs.HasDialogs() || app.activeViewID() != viewJobDetail {
					t.Fatal("close key failed to close just the dialog")
				}
			}
		})
	}
}

func TestAppConsoleFocusRouting(t *testing.T) {
	app := appWithInputDialog(t, true)
	for _, key := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: tea.KeyTab},
		{Code: 'j', Text: "j"},
		{Code: 'k', Text: "k"},
		{Code: tea.KeyTab, Mod: tea.ModShift},
		{Code: 'j', Text: "j"},
		{Code: 'k', Text: "k"},
	} {
		updated, _ := app.Update(key)
		app = updated.(App)
	}
	if app.dialogs.ActiveDialogID() != devtoolsdialog.DialogID ||
		!strings.Contains(ansi.Strip(app.dialogs.ActiveModel().View()), "redis> qjk") {
		t.Fatal("Tab/Shift+Tab failed to transfer input ownership")
	}
}

func TestAppQuitWithoutDialog(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'q', Text: "q"}, {Code: 'c', Mod: tea.ModCtrl}} {
		app := New(nil, "", false, nil)
		_, cmd := app.Update(key)
		if cmd == nil {
			t.Fatalf("%s did not quit", key.String())
		}
		if msg := cmd(); msg != tea.Quit() {
			t.Fatalf("%s returned %T", key.String(), msg)
		}
	}
}

func TestAppDashboardShortTerminal(t *testing.T) {
	client, err := sidekiq.NewClient("redis://localhost:6379/0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	app := New(client, "", false, nil)
	// Render WindowSizeMsg updates without executing the Redis commands.
	for _, width := range []int{40, 80, 120} {
		for _, height := range []int{40, 24, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 40} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				updated, _ := app.Update(tea.WindowSizeMsg{Width: width, Height: height})
				app = updated.(App)
				output := app.View().Content
				if got := lipgloss.Height(output); got != height {
					t.Fatalf("height = %d, want %d", got, height)
				}
				if got := lipgloss.Width(output); got > width {
					t.Fatalf("width = %d, want <= %d", got, width)
				}
			})
		}
	}
}

func TestQueueMutationErrorSurvivesNavigation(t *testing.T) {
	queues := &cancelableStubView{name: "Queues"}
	app := App{
		viewStack:    []viewID{viewQueuesList},
		viewRegistry: map[viewID]views.View{viewQueuesList: queues, viewBusy: stubView{}},
		dialogs:      stubDialogs{},
	}
	app.setActiveView(viewBusy)
	want := errors.New("queue pause failed after navigation")
	updated, _ := app.Update(views.QueueActionResultMsg{Err: want})
	got := updated.(App)
	if !errors.Is(got.connectionError, want) {
		t.Fatalf("connectionError = %v; want %v", got.connectionError, want)
	}
	if queues.cancelations == 0 {
		t.Fatal("test did not hide original view")
	}
}

type stubView struct{}

func (v stubView) Init() tea.Cmd                        { return nil }
func (v stubView) Update(tea.Msg) (views.View, tea.Cmd) { return v, nil }
func (v stubView) View() string                         { return "base view" }
func (v stubView) Name() string                         { return "Stub" }
func (v stubView) ShortHelp() []key.Binding             { return nil }
func (v stubView) SetSize(int, int) views.View          { return v }
func (v stubView) SetStyles(views.Styles) views.View    { return v }

type stubDialogs struct {
	layers []*lipgloss.Layer
}

func (d stubDialogs) Init() tea.Cmd                               { return nil }
func (d stubDialogs) Update(tea.Msg) (dialogs.DialogCmp, tea.Cmd) { return d, nil }
func (d stubDialogs) View() string                                { return "" }
func (d stubDialogs) Dialogs() []dialogs.DialogModel              { return nil }
func (d stubDialogs) HasDialogs() bool                            { return len(d.layers) > 0 }
func (d stubDialogs) GetLayers() []*lipgloss.Layer                { return d.layers }
func (d stubDialogs) ActiveModel() dialogs.DialogModel            { return nil }
func (d stubDialogs) ActiveDialogID() dialogs.DialogID            { return "" }

func (d stubDialogs) ActiveSession() *dialogs.Session { return nil }

type cancelableStubView struct {
	name         string
	cancelations int
}

func (v *cancelableStubView) Init() tea.Cmd                        { return nil }
func (v *cancelableStubView) Update(tea.Msg) (views.View, tea.Cmd) { return v, nil }
func (v *cancelableStubView) View() string                         { return v.name }
func (v *cancelableStubView) Name() string                         { return v.name }
func (v *cancelableStubView) ShortHelp() []key.Binding             { return nil }
func (v *cancelableStubView) SetSize(int, int) views.View          { return v }
func (v *cancelableStubView) SetStyles(views.Styles) views.View    { return v }
func (v *cancelableStubView) CancelRequests()                      { v.cancelations++ }

func TestAppViewOverlaysDialogsAtLayerCoordinates(t *testing.T) {
	t.Parallel()

	app := App{
		keys:      DefaultKeyMap(),
		ready:     true,
		width:     20,
		height:    6,
		viewStack: []viewID{viewDashboard},
		viewOrder: []viewID{viewDashboard},
		viewRegistry: map[viewID]views.View{
			viewDashboard: stubView{},
		},
		dialogs: stubDialogs{
			layers: []*lipgloss.Layer{
				lipgloss.NewLayer("BOX").X(5).Y(3),
			},
		},
	}

	out := ansi.Strip(app.View().Content)
	if !strings.Contains(out, "base view") {
		t.Fatalf("overlay view dropped base content:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Fatalf("line count = %d, want at least 4", len(lines))
	}
	if strings.HasPrefix(lines[0], "BOX") {
		t.Fatalf("dialog rendered in top-left instead of its layer coordinates:\n%s", out)
	}
	if !strings.HasPrefix(lines[3], "     BOX") {
		t.Fatalf("dialog line = %q, want it positioned at x=5", lines[3])
	}
}

func TestSetActiveViewCancelsInactiveViewRequests(t *testing.T) {
	t.Parallel()

	dashboard := &cancelableStubView{name: "Dashboard"}
	busy := &cancelableStubView{name: "Busy"}
	app := App{
		viewStack: []viewID{viewDashboard},
		viewRegistry: map[viewID]views.View{
			viewDashboard: dashboard,
			viewBusy:      busy,
		},
	}

	app.setActiveView(viewBusy)

	if dashboard.cancelations != 1 {
		t.Fatalf("dashboard cancelations = %d, want 1", dashboard.cancelations)
	}
	if busy.cancelations != 0 {
		t.Fatalf("busy cancelations = %d, want 0", busy.cancelations)
	}
}

func TestPushViewCancelsPreviousActiveViewRequests(t *testing.T) {
	t.Parallel()

	busy := &cancelableStubView{name: "Busy"}
	processes := &cancelableStubView{name: "Processes"}
	app := App{
		viewStack: []viewID{viewBusy},
		viewRegistry: map[viewID]views.View{
			viewBusy:          busy,
			viewProcessesList: processes,
		},
	}

	app.pushView(viewProcessesList)

	if busy.cancelations != 1 {
		t.Fatalf("busy cancelations = %d, want 1", busy.cancelations)
	}
	if processes.cancelations != 0 {
		t.Fatalf("processes cancelations = %d, want 0", processes.cancelations)
	}
}

func TestAppShowsUnscopedConnectionErrors(t *testing.T) {
	app := New(nil, "", false, nil)
	wantErr := errors.New("connection failed")
	updated, _ := app.Update(views.ConnectionErrorMsg{Err: wantErr})
	if got := updated.(App).connectionError; !errors.Is(got, wantErr) {
		t.Fatalf("connection error = %v, want %v", got, wantErr)
	}
}

type ownershipClient struct {
	sidekiq.API
	value int64
	err   error
}

func (c *ownershipClient) DisplayRedisURL() string { return "redis://fixture/0" }
func (c *ownershipClient) GetStats(context.Context) (sidekiq.Stats, error) {
	return sidekiq.Stats{Processed: c.value}, c.err
}

func (c *ownershipClient) GetProcessesData(context.Context) ([]sidekiq.Process, error) {
	return nil, c.err
}

func (c *ownershipClient) GetBusyData(context.Context, string) (sidekiq.BusyData, error) {
	return sidekiq.BusyData{}, c.err
}

func (c *ownershipClient) GetErrorSummary(context.Context, string) ([]sidekiq.ErrorSummaryRow, sidekiq.ErrorSummaryMeta, error) {
	return nil, sidekiq.ErrorSummaryMeta{}, c.err
}

func (c *ownershipClient) GetRedisInfo(context.Context) (sidekiq.RedisInfo, error) {
	return sidekiq.RedisInfo{}, c.err
}

func (c *ownershipClient) GetStatsHistory(context.Context, int) (sidekiq.StatsHistory, error) {
	return sidekiq.StatsHistory{}, c.err
}

func (c *ownershipClient) GetMetricsJobDetail(context.Context, string, sidekiq.MetricsPeriod) (sidekiq.MetricsJobDetailResult, error) {
	return sidekiq.MetricsJobDetailResult{}, c.err
}
func (c *ownershipClient) MetricsPeriodOrder(context.Context) []string { return []string{"1h"} }
func (c *ownershipClient) GetMetricsTopJobs(context.Context, sidekiq.MetricsPeriod, string) (sidekiq.MetricsTopJobsResult, error) {
	return sidekiq.MetricsTopJobsResult{}, c.err
}

func (c *ownershipClient) GetCapabilities() sidekiq.Capabilities               { return sidekiq.Capabilities{} }
func (c *ownershipClient) GetQueues(context.Context) ([]*sidekiq.Queue, error) { return nil, c.err }

func firstReadReply(cmd tea.Cmd) tea.Msg {
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		return firstReadReply(batch[0])
	}
	return msg
}

func TestAppReadErrorOwnership(t *testing.T) {
	for _, id := range []viewID{viewBusy, viewDashboard, viewErrorsSummary, viewJobMetrics, viewProcessesList, viewMetrics, viewQueuesList} {
		for scenario := range map[string]bool{"refresh": true, "hidden": true, "reopen": true, "different app": true} {
			t.Run(fmt.Sprintf("%d/%s", id, scenario), func(t *testing.T) {
				staleError, currentError := errors.New("old read"), errors.New("current error")
				client := &ownershipClient{err: staleError}
				app := New(client, "", false, nil)
				app.viewStack = []viewID{id}
				if id == viewJobMetrics {
					app.viewRegistry[id].(views.JobMetricsSetter).SetJobMetrics("OldJob", "1h")
				}
				stale := firstReadReply(app.viewRegistry[id].Init())
				t.Cleanup(func() {
					for id := range app.viewRegistry {
						app.cancelViewRequests(id)
					}
				})
				switch scenario {
				case "refresh":
					client.err = currentError
					current := firstReadReply(app.viewRegistry[id].Init())
					updated, _ := app.Update(current)
					app = updated.(App)
					if !errors.Is(app.connectionError, currentError) {
						t.Fatal("current failure was lost")
					}
				case "hidden":
					app.setActiveView(viewScheduled)
				case "reopen":
					app.setActiveView(viewScheduled)
					app.setActiveView(id)
				case "different app":
					app = New(client, "", false, nil)
					app.viewStack = []viewID{id}
				}
				app.connectionError = currentError
				updated, _ := app.Update(stale)
				app = updated.(App)
				if !errors.Is(app.connectionError, currentError) {
					t.Fatalf("stale read changed error to %v", app.connectionError)
				}
			})
		}
	}
}

func TestAppStatsRequestOwnership(t *testing.T) {
	failure := errors.New("stats unavailable")
	for name, errs := range map[string][2]error{"success then success": {}, "failure then success": {failure, nil}, "success then failure": {nil, failure}, "failure then failure": {failure, failure}} {
		for scenario := range map[string]bool{"refresh": true, "init copy": true, "different app": true} {
			t.Run(name+"/"+scenario, func(t *testing.T) {
				client := &ownershipClient{value: 1, err: errs[0]}
				app := New(client, "", false, nil)
				t.Cleanup(func() { app.statsRequest.Cancel() })
				var old tea.Msg
				if scenario == "init copy" {
					// App.Init has a value receiver; its controller must still be shared.
					for _, cmd := range app.Init()().(tea.BatchMsg) {
						msg := cmd()
						if _, ok := msg.(statsResultMsg); ok {
							old = msg
							break
						}
					}
				} else {
					old = app.fetchStatsCmd()()
				}
				if old == nil {
					t.Fatal("missing old stats reply")
				}
				if scenario == "different app" {
					oldOwner := app.statsRequest
					t.Cleanup(oldOwner.Cancel)
					app = New(client, "", false, nil)
				}
				client.value, client.err = 2, errs[1]
				fresh := app.fetchStatsCmd()()
				before := app.metrics.Data()
				updated, _ := app.Update(old)
				app = updated.(App)
				if app.metrics.Data() != before || app.connectionError != nil {
					t.Fatal("old stats changed state before current completion")
				}
				updated, _ = app.Update(fresh)
				app = updated.(App)
				if errs[1] == nil && app.metrics.Data().Processed != 2 {
					t.Fatal("current stats were lost")
				}
				if errs[1] != nil && !errors.Is(app.connectionError, failure) {
					t.Fatal("current failure was lost")
				}
				after, afterError := app.metrics.Data(), app.connectionError
				updated, _ = app.Update(old)
				app = updated.(App)
				if app.metrics.Data() != after || !errors.Is(app.connectionError, afterError) {
					t.Fatal("old stats changed current data or error")
				}
			})
		}
	}
}

func TestAppJobMetricsClassReplacementRejectsReadError(t *testing.T) {
	app := New(&ownershipClient{err: errors.New("old class read")}, "", false, nil)
	app.viewStack = []viewID{viewJobMetrics}
	job := app.viewRegistry[viewJobMetrics].(views.JobMetricsSetter)
	job.SetJobMetrics("OldJob", "1h")
	stale := app.viewRegistry[viewJobMetrics].Init()()
	job.SetJobMetrics("NewJob", "1h")
	updated, _ := app.Update(stale)
	if updated.(App).connectionError != nil {
		t.Fatal("old class error covered replacement before it fetched")
	}
}
