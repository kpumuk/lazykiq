package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	helpdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/help"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

func TestAppGlobalDialogCommandsSurviveNavigation(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: '?', Text: "?"}, {Code: tea.KeyF12}} {
		t.Run(key.String(), func(t *testing.T) {
			app := New(&ownershipClient{}, "", false, devtools.NewTracker())
			defer app.cancel()
			cmd := dialogStep(&app, key)
			dialogStep(&app, tea.KeyPressMsg{Code: '7', Text: "7"})
			dialogStep(&app, cmd())
			if !app.dialogs.HasDialogs() || app.activeViewID() != viewErrorsSummary {
				t.Fatal("global dialog was lost after navigation")
			}
		})
	}
}

func TestAppViewIntentOwnership(t *testing.T) {
	for _, input := range []string{"/", "s"} {
		for _, state := range []string{"current", "away", "away-back", "replaced", "covered", "covered-closed", "shutdown", "different app"} {
			for _, executeLate := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/executeLate=%t", input, state, executeLate), func(t *testing.T) {
					app := New(&ownershipClient{}, "", false, nil)
					defer app.cancel()
					dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
					app.setActiveView(viewBusy)
					issuer := app.viewRegistry[viewBusy]
					cmd := dialogStep(&app, tea.KeyPressMsg{Code: rune(input[0]), Text: input})
					var result tea.Msg
					if !executeLate {
						result = firstReadReply(cmd)
					}
					switch state {
					case "away", "away-back":
						dialogStep(&app, tea.KeyPressMsg{Code: '7', Text: "7"})
						if state == "away-back" {
							dialogStep(&app, tea.KeyPressMsg{Code: '2', Text: "2"})
						}
					case "replaced":
						app.viewRegistry[viewBusy] = views.NewBusy(&ownershipClient{})
					case "covered", "covered-closed":
						dialogStep(&app, dialogs.OpenDialogMsg{Model: helpdialog.New()})
						if state == "covered-closed" {
							dialogStep(&app, dialogs.CloseDialogMsg{})
						}
					case "shutdown":
						app.cancel()
					case "different app":
						app = New(&ownershipClient{}, "", false, nil)
						defer app.cancel()
						app.setActiveView(viewBusy)
					}
					active, session := app.activeViewID(), app.dialogs.ActiveSession()
					if executeLate {
						result = firstReadReply(cmd)
					}
					dialogStep(&app, result)
					if state != "current" {
						if app.activeViewID() != active || app.dialogs.ActiveSession() != session {
							t.Fatalf("stale %q changed screen or modal: active=%s", input, app.viewRegistry[app.activeViewID()].Name())
						}
						return
					}
					if input == "s" {
						if app.activeViewID() != viewProcessesList {
							t.Fatal("current navigation was rejected")
						}
						return
					}
					owner := app.dialogs.ActiveSession().Owner.(dialogOwner)
					if owner.id != viewBusy || owner.view != issuer || owner.generation != app.viewGeneration {
						t.Fatalf("filter owner = %#v", owner)
					}
					dialogStep(&app, tea.KeyPressMsg{Code: 'x', Text: "x"})
					fetch := dialogStep(&app, firstReadReply(dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})))
					dialogStep(&app, firstReadReply(fetch))
					if !strings.Contains(ansi.Strip(issuer.View()), "[x]") || app.dialogs.HasDialogs() {
						t.Fatal("current filter did not apply to Busy and close")
					}
				})
			}
		}
	}
}

type initIntentView struct {
	stubView
	views.IntentScope
}

func (v *initIntentView) Init() tea.Cmd {
	return v.EmitIntent(dialogs.OpenDialogMsg{Model: filterdialog.New()})
}

func TestAppInitialIntentOwnership(t *testing.T) {
	for _, route := range []string{"init", "switch", "push"} {
		for _, stale := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stale=%t", route, stale), func(t *testing.T) {
				app := New(&ownershipClient{}, "", false, nil)
				defer app.cancel()
				view := &initIntentView{}
				var cmd tea.Cmd
				if route == "init" {
					app.viewRegistry[viewDashboard] = view
					// The second startup command belongs to the initial view.
					cmd = app.Init()().(tea.BatchMsg)[1]
				} else {
					app.viewRegistry[viewBusy] = view
					if route == "switch" {
						cmd = app.setActiveView(viewBusy)
					} else {
						cmd = app.pushView(viewBusy)
					}
				}
				if stale {
					id := app.activeViewID()
					app.setActiveView(viewErrorsSummary)
					app.setActiveView(id)
				}
				dialogStep(&app, cmd())
				if app.dialogs.HasDialogs() == stale {
					t.Fatal("initial intent did not follow its emitting activation")
				}
				if !stale && app.dialogs.ActiveSession().Owner.(dialogOwner).view != view {
					t.Fatal("initial intent acquired a different owner")
				}
			})
		}
	}
}

type intentCommandView struct {
	stubView
	views.IntentScope
	intent tea.Msg
	err    error
}

func (v *intentCommandView) Update(tea.Msg) (views.View, tea.Cmd) {
	intent := v.EmitIntent(v.intent)
	failure := v.err
	return v, tea.Sequence(
		tea.Batch(tea.Sequence(intent, tea.Batch(
			func() tea.Msg { return views.ConnectionErrorMsg{Err: failure} },
			func() tea.Msg { return views.QueueActionResultMsg{Err: failure} },
		)), func() tea.Msg { return nil }),
		func() tea.Msg { return intentCommandsDone{} },
	)
}

type intentCommandsDone struct{}

// Let Bubble Tea execute the command tree and deliver every leaf to the root.
type intentProgram struct {
	app App
	cmd tea.Cmd
}

func (m intentProgram) Init() tea.Cmd  { return m.cmd }
func (m intentProgram) View() tea.View { return m.app.View() }
func (m intentProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, done := msg.(intentCommandsDone); done {
		return m, tea.Quit
	}
	next, cmd := m.app.Update(msg)
	m.app = next.(App)
	return m, cmd
}

func TestAppNestedViewCommands(t *testing.T) {
	for name, intent := range map[string]tea.Msg{
		"filter":    dialogs.OpenDialogMsg{Model: filterdialog.New()},
		"processes": views.ShowProcessesListMsg{},
		"job":       views.ShowJobDetailMsg{},
		"errors":    views.ShowErrorDetailsMsg{},
		"metrics":   views.ShowJobMetricsMsg{},
		"periodic":  views.ShowPeriodicDetailMsg{},
		"queues":    views.ShowQueuesListMsg{},
		"queue":     views.ShowQueueDetailsMsg{QueueName: "default"},
		"process":   views.ShowProcessSelectMsg{Identity: "old"},
	} {
		t.Run(name, func(t *testing.T) {
			failure := errors.New("global operation failed")
			app := New(&ownershipClient{}, "", false, nil)
			defer app.cancel()
			view := &intentCommandView{intent: intent, err: failure}
			app.viewRegistry[viewBusy] = view
			app.setActiveView(viewBusy)
			cmd := dialogStep(&app, tea.KeyPressMsg{Code: '/', Text: "/"})
			app.setActiveView(viewErrorsSummary)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := tea.NewProgram(intentProgram{app, cmd}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer()).Run()
			if err != nil {
				t.Fatal(err)
			}
			app = result.(intentProgram).app
			if app.activeViewID() != viewErrorsSummary || app.dialogs.HasDialogs() {
				t.Fatal("nested stale intent changed screen")
			}
			if !errors.Is(app.connectionError, failure) {
				t.Fatal("global operation error was suppressed")
			}
		})
	}
}

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
