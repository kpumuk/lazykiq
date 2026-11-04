package ui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	helpdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/help"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

func dialogStep(app *App, msg tea.Msg) tea.Cmd {
	next, cmd := app.Update(msg)
	*app = next.(App)
	return cmd
}

// submitFilter drives the same dialog transition as typing /, a query, Enter.
func submitFilter(app *App, query string) tea.Cmd {
	dialogStep(app, dialogs.OpenDialogMsg{Model: filterdialog.New()})
	for _, r := range query {
		dialogStep(app, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return dialogStep(app, dialogStep(app, tea.KeyPressMsg{Code: tea.KeyEnter})())
}

func TestAppFilterCompletionIsAtomic(t *testing.T) {
	app := New(&ownershipClient{}, "", false, nil)
	app.setActiveView(viewBusy)
	dialogStep(&app, dialogs.OpenDialogMsg{Model: filterdialog.New()})
	dialogStep(&app, tea.KeyPressMsg{Code: 'x', Text: "x"})
	completion := dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})()
	if _, ok := completion.(dialogs.CloseDialogMsg); !ok {
		t.Fatalf("submission returned %T, want single completion", completion)
	}
	fetch := dialogStep(&app, completion)
	dialogStep(&app, fetch())
	if app.dialogs.HasDialogs() {
		t.Fatal("valid completion left dialog open")
	}
	dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
	if output := ansi.Strip(app.viewRegistry[viewBusy].View()); !strings.Contains(output, "[x]") {
		t.Fatalf("Busy filter missing: %s", output)
	}

	dialogStep(&app, tea.KeyPressMsg{Code: '7', Text: "7"})
	dialogStep(&app, completion) // duplicate delivery after navigation
	for _, item := range app.viewRegistry[viewErrorsSummary].(views.ContextProvider).ContextItems() {
		if item.Label == "Filter" && item.Value == "x" {
			t.Fatal("Busy completion filtered Errors")
		}
	}
}

func TestAppDelayedBusyDialogOpen(t *testing.T) {
	for name, transition := range map[string]func(*App){
		"current": func(*App) {},
		"navigate": func(a *App) {
			dialogStep(a, tea.KeyPressMsg{Code: '7', Text: "7"})
		},
		"away and back": func(a *App) {
			dialogStep(a, tea.KeyPressMsg{Code: '7', Text: "7"})
			dialogStep(a, tea.KeyPressMsg{Code: '2', Text: "2"})
		},
		"replacement": func(a *App) {
			a.viewRegistry[viewBusy] = &dialogRecordingView{}
		},
		"push": func(a *App) {
			dialogStep(a, views.ShowProcessesListMsg{})
		},
		"push and pop": func(a *App) {
			dialogStep(a, views.ShowProcessesListMsg{})
			dialogStep(a, tea.KeyPressMsg{Code: tea.KeyEsc})
		},
	} {
		t.Run(name, func(t *testing.T) {
			app := New(&ownershipClient{}, "", false, nil)
			t.Cleanup(app.cancel)
			other := &dialogRecordingView{}
			app.viewRegistry[viewErrorsSummary] = other
			dialogStep(&app, tea.KeyPressMsg{Code: '2', Text: "2"})
			open := dialogStep(&app, tea.KeyPressMsg{Code: '/', Text: "/"})
			transition(&app)
			dialogStep(&app, open())
			if got, want := app.dialogs.HasDialogs(), name == "current"; got != want {
				t.Fatalf("dialog open = %v, want %v", got, want)
			}
			if app.dialogs.HasDialogs() {
				dialogStep(&app, tea.KeyPressMsg{Code: 'x', Text: "x"})
				fetch := dialogStep(&app, dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})())
				dialogStep(&app, fetch())
				dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
				if output := ansi.Strip(app.viewRegistry[viewBusy].View()); !strings.Contains(output, "[x]") {
					t.Fatalf("Busy filter missing: %s", output)
				}
			}
			if len(other.actions) != 0 {
				t.Fatalf("Errors received Busy filter action: %#v", other.actions)
			}
		})
	}
}

func TestAppViewFilterDialogOwners(t *testing.T) {
	for name, id := range map[string]viewID{
		"busy": viewBusy, "queue details": viewQueueDetails, "queues": viewQueuesList,
		"processes": viewProcessesList, "retries": viewRetries, "scheduled": viewScheduled,
		"dead": viewDead, "errors": viewErrorsSummary, "metrics": viewMetrics, "periodic": viewPeriodic,
	} {
		t.Run(name, func(t *testing.T) {
			client := &ownershipClient{}
			app := New(client, "", false, nil)
			t.Cleanup(app.cancel)
			app.viewRegistry[viewPeriodic] = views.NewPeriodic(client)
			app.setActiveView(id)
			open := dialogStep(&app, tea.KeyPressMsg{Code: '/', Text: "/"})
			if open == nil {
				t.Fatal("filter shortcut did not return a command")
			}
			dialogStep(&app, tea.KeyPressMsg{Code: '1', Text: "1"})
			msg := open().(dialogs.OpenDialogMsg)
			owner, ok := msg.Owner.(dialogOwner)
			if !ok || owner.id != id || owner.view != app.viewRegistry[id] {
				t.Fatalf("dialog owner = %#v, want originating view %v", msg.Owner, id)
			}
			dialogStep(&app, msg)
			if app.dialogs.HasDialogs() {
				t.Fatal("stale filter opened after navigation")
			}
		})
	}
}

type dialogCommandView struct {
	dialogRecordingView
	dialogs.Opener
	compose func(tea.Cmd, tea.Cmd, tea.Cmd) tea.Cmd
}

func (v *dialogCommandView) Init() tea.Cmd {
	return v.compose(
		func() tea.Msg { return "first" },
		func() tea.Msg { return "second" },
		v.OpenDialog(filterdialog.New()),
	)
}

func (v *dialogCommandView) Update(msg tea.Msg) (views.View, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.Text == "/" {
		return v, v.Init()
	}
	v.dialogRecordingView.Update(msg)
	return v, nil
}

// Run command trees through Bubble Tea itself and hold their results for
// delivery after a navigation transition.
type dialogCommandCollector struct {
	initial  tea.Cmd
	messages []tea.Msg
}

func (m dialogCommandCollector) Init() tea.Cmd  { return m.initial }
func (m dialogCommandCollector) View() tea.View { return tea.NewView("") }
func (m dialogCommandCollector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case dialogs.OpenDialogMsg, string:
		m.messages = append(m.messages, msg)
		if len(m.messages) == 3 {
			return m, tea.Quit
		}
	}
	return m, nil
}

func TestAppDialogCommandTrees(t *testing.T) {
	for shape, compose := range map[string]func(tea.Cmd, tea.Cmd, tea.Cmd) tea.Cmd{
		"batch":           func(a, b, c tea.Cmd) tea.Cmd { return tea.Batch(a, b, c) },
		"nested batch":    func(a, b, c tea.Cmd) tea.Cmd { return tea.Batch(a, tea.Batch(b, c)) },
		"sequence":        func(a, b, c tea.Cmd) tea.Cmd { return tea.Sequence(a, b, c) },
		"nested sequence": func(a, b, c tea.Cmd) tea.Cmd { return tea.Sequence(a, tea.Sequence(b, c)) },
		"mixed":           func(a, b, c tea.Cmd) tea.Cmd { return tea.Batch(a, tea.Sequence(b, c)) },
	} {
		for _, source := range []string{"update", "init", "app init", "stack init"} {
			for _, stale := range []bool{false, true} {
				name := shape + "/" + source + "/current"
				if stale {
					name = shape + "/" + source + "/stale"
				}
				t.Run(name, func(t *testing.T) {
					app := New(nil, "", false, nil)
					t.Cleanup(app.cancel)
					owner := &dialogCommandView{compose: compose}
					app.viewRegistry[viewDashboard] = owner
					var cmd tea.Cmd
					switch source {
					case "update":
						cmd = dialogStep(&app, tea.KeyPressMsg{Code: '/', Text: "/"})
					case "init":
						cmd = app.setActiveView(viewDashboard)
					case "app init":
						cmd = app.Init()
						// Select the view command without starting app subscriptions.
						cmd = cmd().(tea.BatchMsg)[1]
					case "stack init":
						app.viewRegistry[viewProcessesList] = owner
						cmd = dialogStep(&app, views.ShowProcessesListMsg{})
					}
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					program := tea.NewProgram(dialogCommandCollector{initial: cmd},
						tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(),
						tea.WithWindowSize(80, 24), tea.WithContext(ctx), tea.WithoutSignalHandler())
					model, err := program.Run()
					if err != nil {
						t.Fatal(err)
					}
					if stale {
						dialogStep(&app, tea.KeyPressMsg{Code: '7', Text: "7"})
						// A stale open must also leave a newer global dialog intact.
						dialogStep(&app, dialogStep(&app, tea.KeyPressMsg{Code: '?', Text: "?"})())
					}
					var markers []string
					for _, msg := range model.(dialogCommandCollector).messages {
						if marker, ok := msg.(string); ok {
							markers = append(markers, marker)
						}
						dialogStep(&app, msg)
					}
					if strings.Contains(shape, "sequence") && strings.Join(markers, ",") != "first,second" {
						t.Fatalf("sequence markers = %v", markers)
					}
					if stale {
						if len(app.dialogs.Dialogs()) != 1 || app.dialogs.ActiveDialogID() != helpdialog.New().ID() {
							t.Fatal("stale open changed the global help dialog")
						}
						return
					}
					dialogStep(&app, tea.KeyPressMsg{Code: 'x', Text: "x"})
					dialogStep(&app, dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})())
					if len(owner.actions) != 1 {
						t.Fatalf("owner actions = %#v", owner.actions)
					}
				})
			}
		}
	}
}

type dialogRecordingView struct {
	stubView
	actions []tea.Msg
}

func (v *dialogRecordingView) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg.(type) {
	case filterdialog.ActionMsg, confirmdialog.ActionMsg:
		v.actions = append(v.actions, msg)
	}
	return v, nil
}

func TestAppDialogCompletionOwnership(t *testing.T) {
	for kind, makeDialog := range map[string]func() dialogs.DialogModel{
		"filter":  func() dialogs.DialogModel { return filterdialog.New() },
		"confirm": func() dialogs.DialogModel { return confirmdialog.New(confirmdialog.WithTarget("same-target")) },
	} {
		for order, steps := range map[string][]string{
			"valid then duplicate":               {"result", "result"},
			"cancel navigate late":               {"cancel", "navigate", "result"},
			"cancel reopen late":                 {"cancel", "reopen", "result"},
			"cancel navigate reopen late":        {"cancel", "navigate", "reopen", "result"},
			"cover late reveal late":             {"cover", "result", "uncover", "result"},
			"owner away and back":                {"away-back", "result"},
			"owner replaced":                     {"replace-view", "result"},
			"late close after reopen":            {"cancel", "reopen", "close"},
			"complete navigate reopen duplicate": {"result", "navigate", "reopen", "result"},
		} {
			t.Run(kind+"/"+order, func(t *testing.T) {
				app := New(nil, "", false, nil)
				owner, other, replacement := &dialogRecordingView{}, &dialogRecordingView{}, &dialogRecordingView{}
				app.viewRegistry[viewDashboard] = owner
				app.viewRegistry[viewErrorsSummary] = other
				dialogStep(&app, dialogs.OpenDialogMsg{Model: makeDialog()})
				key := tea.KeyPressMsg{Code: 'y', Text: "y"}
				if kind == "filter" {
					dialogStep(&app, tea.KeyPressMsg{Code: 'x', Text: "x"})
					key = tea.KeyPressMsg{Code: tea.KeyEnter}
				}
				result := dialogStep(&app, key)()
				// Capture a second completion from the same activation, then deliver it late.
				cancelResult := dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEsc})()
				wantActions := 0
				if steps[0] == "result" {
					wantActions = 1
				}
				for _, step := range steps {
					switch step {
					case "result":
						dialogStep(&app, result)
					case "close":
						dialogStep(&app, cancelResult)
					case "cancel":
						dialogStep(&app, cancelResult)
					case "navigate":
						dialogStep(&app, tea.KeyPressMsg{Code: '7', Text: "7"})
					case "away-back":
						app.setActiveView(viewErrorsSummary)
						app.setActiveView(viewDashboard)
					case "replace-view":
						app.viewRegistry[viewDashboard] = replacement
					case "reopen":
						dialogStep(&app, dialogs.OpenDialogMsg{Model: makeDialog()})
					case "cover":
						dialogStep(&app, dialogs.OpenDialogMsg{Model: helpdialog.New()})
					case "uncover":
						dialogStep(&app, dialogs.CloseDialogMsg{})
					}
				}
				// A valid Escape delivers the confirmation cancellation before reopening.
				cancels := 0
				for _, action := range owner.actions {
					if a, ok := action.(confirmdialog.ActionMsg); ok && !a.Confirmed {
						cancels++
					}
				}
				wantCancels := 0
				if kind == "confirm" && steps[0] == "cancel" {
					wantCancels = 1
				}
				if cancels != wantCancels {
					t.Fatalf("cancellations = %d, want %d", cancels, wantCancels)
				}
				if len(replacement.actions) != 0 {
					t.Fatal("replacement view received stale action")
				}
				if got := len(owner.actions) - cancels; got != wantActions {
					t.Fatalf("owner actions = %#v, want %d completions", owner.actions, wantActions)
				}
				if len(other.actions) != 0 {
					t.Fatalf("other view received %#v", other.actions)
				}
				if steps[len(steps)-1] == "result" || steps[len(steps)-1] == "close" {
					for _, step := range steps {
						if step == "reopen" && !app.dialogs.HasDialogs() {
							t.Fatal("late completion closed replacement dialog")
						}
					}
				}
			})
		}
	}
}
