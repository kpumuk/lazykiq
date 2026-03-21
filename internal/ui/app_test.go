package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

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
