package ui

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type backgroundTerminal struct {
	subscribed chan struct{}
	queries    chan struct{}
	once       sync.Once
}

func (t *backgroundTerminal) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte(ansi.SetModeLightDark)) {
		t.once.Do(func() { close(t.subscribed) })
	}
	for range bytes.Count(data, []byte(ansi.RequestBackgroundColor)) {
		t.queries <- struct{}{}
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
	if _, ok := msg.(tea.BackgroundColorMsg); ok {
		m.palettes <- m.styles.ViewText
	}
	return m, cmd
}

func TestAppTerminalThemeChanges(t *testing.T) {
	input, terminal := io.Pipe()
	defer func() { _ = input.Close() }()
	defer func() { _ = terminal.Close() }()
	output := &backgroundTerminal{subscribed: make(chan struct{}), queries: make(chan struct{}, 8)}
	palettes := make(chan lipgloss.Style, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mr := miniredis.RunT(t)
	client, err := sidekiq.NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	app := New(client, "", false, nil)
	app.viewRegistry[viewDashboard] = stubView{}
	p := tea.NewProgram(backgroundProgram{App: app, palettes: palettes},
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
	// Feed real terminal bytes through Bubble Tea's input parser. Config changes
	// need not send a notification, and appearance reports can disagree with OSC 11.
	for _, tc := range []struct {
		notification string
		message      tea.Msg
		dark         bool
	}{
		{}, // Initial query: light background.
		{notification: ansi.LightDarkReport(true), dark: true},
		{notification: ansi.LightDarkReport(false), dark: true}, // Preference disagrees with theme.
		{message: tickMsg{}}, // Silent config reload to light.
		{message: tea.FocusMsg{}, dark: true},
	} {
		if tc.notification != "" {
			if _, err := io.WriteString(terminal, tc.notification); err != nil {
				t.Fatal(err)
			}
		}
		if tc.message != nil {
			p.Send(tc.message)
		}
		select {
		case <-output.queries:
		case <-ctx.Done():
			t.Fatalf("background was not queried after %q / %T", tc.notification, tc.message)
		}
		report := "\x1b]11;rgb:ffff/ffff/ffff\x1b\\"
		if tc.dark {
			report = "\x1b]11;rgb:0000/0000/0000\x1b\\"
		}
		if _, err := io.WriteString(terminal, report); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-palettes:
			if want := theme.NewStylesForBackground(tc.dark).ViewText; !reflect.DeepEqual(got, want) {
				t.Fatalf("report %q did not refresh the palette", report)
			}
		case <-ctx.Done():
			t.Fatalf("report %q was not handled", report)
		}
	}
	p.Send(tea.QuitMsg{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestAppThemeChangeMatchesFreshDark(t *testing.T) {
	mr := miniredis.RunT(t)
	_, _ = mr.SetAdd("queues", "batch")
	_, _ = mr.Lpush("queue:batch", `{"jid":"first","class":"TestJob","args":["example"]}`)
	_, _ = mr.Lpush("queue:batch", `{"jid":"second","class":"TestJob","args":["other"]}`)
	client, err := sidekiq.NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	for scene, withDialog := range map[string]bool{"queue": false, "open filter": true} {
		t.Run(scene, func(t *testing.T) {
			build := func(background color.Color) App {
				app := New(client, "test", false, nil)
				app = queueNavigationUpdate(t, app, tea.BackgroundColorMsg{Color: background})
				app = queueNavigationUpdate(t, app, tea.WindowSizeMsg{Width: 120, Height: 30})
				app = queueNavigationUpdate(t, app, views.ShowQueueDetailsMsg{QueueName: "batch"})
				if withDialog {
					updated, _ := app.Update(dialogs.OpenDialogMsg{Model: filterdialog.New(filterdialog.WithQuery("worker"))})
					app = updated.(App)
				}
				return app
			}
			switched := build(color.White)
			_ = switched.View() // Populate caches under the light palette.
			switched = queueNavigationUpdate(t, switched, tea.BackgroundColorMsg{Color: color.Black})
			fresh := build(color.Black)
			got, want := switched.View(), fresh.View()
			if !got.ReportFocus || !strings.Contains(got.Content, "TestJob") {
				t.Fatal("missing focus reporting or populated queue fixture")
			}
			if got.Content != want.Content {
				t.Fatal("light-to-dark rendering differs from a fresh dark start")
			}
		})
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
