package ui

import (
	"image/color"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type paletteView struct {
	stubView
	styles views.Styles
	calls  int
}

func (v *paletteView) SetStyles(styles views.Styles) views.View {
	v.styles = styles
	v.calls++
	return v
}

func TestAppBackgroundPalette(t *testing.T) {
	app := New(nil, "test", false, nil)
	if !reflect.DeepEqual(app.styles, theme.NewStyles()) {
		t.Fatal("missing dark no-reply fallback")
	}
	// Include hidden/stacked views, not just the active dashboard.
	probes := make([]*paletteView, 0, len(app.viewRegistry))
	for id := range app.viewRegistry {
		v := &paletteView{}
		app.viewRegistry[id] = v
		probes = append(probes, v)
	}
	for name, tc := range map[string]struct {
		background color.Color
		dark       bool
	}{
		"light": {color.White, false}, "dark": {color.Black, true},
	} {
		t.Run(name, func(t *testing.T) {
			// Force a transition for both table cases without relying on map order.
			app.darkBackground = !tc.dark
			updated, cmd := app.Update(tea.BackgroundColorMsg{Color: tc.background})
			app = updated.(App)
			if cmd != nil {
				t.Fatal("background reply should not start fetches")
			}
			want := theme.NewStylesForBackground(tc.dark)
			if !reflect.DeepEqual(app.styles, want) {
				t.Fatal("palette was not resolved")
			}
			for _, v := range probes {
				if !reflect.DeepEqual(v.styles.Text, want.ViewText) || !reflect.DeepEqual(v.styles.ChartSuccess, want.ChartSuccess) || !reflect.DeepEqual(v.styles.JSONKey, want.JSONKey) {
					t.Fatal("view/chart/syntax styles were not propagated")
				}
			}
			calls := probes[0].calls
			updated, _ = app.Update(tea.BackgroundColorMsg{Color: tc.background})
			app = updated.(App)
			if calls != probes[0].calls {
				t.Fatal("duplicate reply invalidated view caches")
			}
		})
	}
}

func TestAppLightReply(t *testing.T) {
	app := New(nil, "", false, nil)
	updated, _ := app.Update(tea.BackgroundColorMsg{Color: color.White})
	got := updated.(App).styles.ViewText.GetForeground()
	if got != lipgloss.Color("#111827") {
		t.Fatalf("light background foreground = %v; want #111827", got)
	}
}

func TestAppBackgroundReplyPreservesDialog(t *testing.T) {
	app := New(nil, "", false, nil)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app = updated.(App)
	dialog := filterdialog.New(filterdialog.WithQuery("worker"))
	updated, _ = app.Update(dialogs.OpenDialogMsg{Model: dialog})
	app = updated.(App)
	before := dialog.View()
	updated, _ = app.Update(tea.BackgroundColorMsg{Color: color.White})
	app = updated.(App)
	if app.dialogs.ActiveModel() != dialog || app.width != 120 || app.height != 40 {
		t.Fatal("theme reply replaced dialog or geometry")
	}
	if after := dialog.View(); after == before {
		t.Fatal("open dialog retained old colors")
	}
	// A delayed command may return a dialog built with an old palette.
	updated, _ = app.Update(dialogs.CloseDialogMsg{})
	app = updated.(App)
	stale := filterdialog.New(filterdialog.WithQuery("worker"))
	updated, _ = app.Update(dialogs.OpenDialogMsg{Model: stale})
	app = updated.(App)
	if stale.View() != dialog.View() {
		t.Fatal("new dialog did not receive current palette")
	}
}

func TestAppBackgroundPreservesConfirmationIntent(t *testing.T) {
	for name, danger := range map[string]bool{"neutral": false, "danger": true} {
		t.Run(name, func(t *testing.T) {
			app := New(nil, "", false, nil)
			yes := app.styles.ContextKey
			if danger {
				yes = app.styles.ContextDangerKey
			}
			dialog := confirmdialog.New(confirmdialog.WithStyles(confirmdialog.Styles{ButtonYesActive: yes, ButtonNoActive: app.styles.ContextKey}))
			updated, _ := app.Update(dialogs.OpenDialogMsg{Model: dialog})
			app = updated.(App)
			updated, _ = app.Update(tea.BackgroundColorMsg{Color: color.White})
			app = updated.(App)
			want := app.styles.ContextKey
			if danger {
				want = app.styles.ContextDangerKey
			}
			if !reflect.DeepEqual(dialog.Styles().ButtonYesActive, want) {
				t.Fatal("background reply changed confirmation intent")
			}
		})
	}
}
