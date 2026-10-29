package ui

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type backgroundTerminal struct {
	subscribed chan struct{}
	once       sync.Once
}

func (t *backgroundTerminal) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte(ansi.SetModeLightDark)) {
		t.once.Do(func() { close(t.subscribed) })
	}
	return len(data), nil
}

type backgroundProgram struct {
	App
	palettes chan lipgloss.Style
}

func (m backgroundProgram) Init() tea.Cmd  { return requestBackgroundCmd() }
func (m backgroundProgram) View() tea.View { return tea.NewView("") }
func (m backgroundProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.App.Update(msg)
	m.App = updated.(App)
	switch msg.(type) {
	case tea.BackgroundColorMsg, uv.DarkColorSchemeEvent, uv.LightColorSchemeEvent:
		m.palettes <- m.styles.ViewText
	}
	return m, cmd
}

func TestAppTerminalThemeChanges(t *testing.T) {
	input, terminal := io.Pipe()
	defer func() { _ = input.Close() }()
	defer func() { _ = terminal.Close() }()
	output := &backgroundTerminal{subscribed: make(chan struct{})}
	palettes := make(chan lipgloss.Style, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := tea.NewProgram(backgroundProgram{App: New(nil, "", false, nil), palettes: palettes},
		tea.WithInput(input), tea.WithOutput(output), tea.WithContext(ctx),
		tea.WithWindowSize(120, 40), tea.WithoutSignalHandler())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	defer p.Kill()
	select {
	case <-output.subscribed:
	case <-ctx.Done():
		t.Fatal("terminal was not subscribed to theme changes")
	}
	// Feed real terminal bytes through Bubble Tea's input parser: start in light
	// mode via OSC 11, then switch dark and back via Ghostty's mode 2031 reports.
	for _, tc := range []struct {
		report string
		dark   bool
	}{
		{"\x1b]11;rgb:ffff/ffff/ffff\x1b\\", false},
		{ansi.LightDarkReport(true), true},
		{ansi.LightDarkReport(false), false},
	} {
		if _, err := io.WriteString(terminal, tc.report); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-palettes:
			if want := theme.NewStylesForBackground(tc.dark).ViewText; !reflect.DeepEqual(got, want) {
				t.Fatalf("report %q did not refresh the palette", tc.report)
			}
		case <-ctx.Done():
			t.Fatalf("report %q was not handled", tc.report)
		}
	}
	p.Send(tea.QuitMsg{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

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
