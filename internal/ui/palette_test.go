package ui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/ui/views"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	palettedialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/palette"
)

func TestPaletteDestinations(t *testing.T) {
	for name, enabled := range map[string]bool{"standard": false, "enterprise": true} {
		t.Run(name, func(t *testing.T) {
			features := []string(nil)
			if enabled {
				features = []string{"ent"}
			}
			capabilities, _ := sidekiq.ResolveFeatures(features)
			client := periodicAppClient{API: &ownershipClient{}, features: capabilities}
			app := New(client, "", false, nil)
			items := app.paletteItems()
			if len(items) != len(app.viewOrder)+4 {
				t.Fatal("missing destinations")
			}
			for _, id := range app.viewOrder {
				found := false
				for _, item := range items {
					if item.ID == int(id) {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing main view %d", id)
				}
			}
			for _, item := range items {
				app.navigatePalette(item.ID)
				if item.ID == int(viewProcessesList) {
					if app.activeViewID() != viewProcessesList || app.viewStack[0] != viewBusy {
						t.Fatal("process stack lost Busy parent")
					}
					updated, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
					app = updated.(App)
					if cmd == nil {
						t.Fatal("return did not refresh parent")
					}
					if app.activeViewID() != viewBusy {
						t.Fatal("Esc parent")
					}
				}
			}
		})
	}
}

func TestAppPaletteInputAndNavigation(t *testing.T) {
	app := New(&ownershipClient{}, "", false, nil)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = updated.(App)
	updated, _ = app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	app = updated.(App)
	for _, r := range "BUSY" {
		updated, _ = app.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		app = updated.(App)
	}
	if app.activeViewID() != viewDashboard || app.dialogs.ActiveDialogID() != palettedialog.DialogID {
		t.Fatal("input escaped palette")
	}
	updated, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = updated.(App)
	updated, _ = app.Update(cmd())
	app = updated.(App)
	if app.activeViewID() != viewBusy || app.dialogs.HasDialogs() {
		t.Fatal("selection did not navigate and close")
	}
	// An existing input dialog keeps ownership of Ctrl-T.
	app = appWithInputDialog(t, false)
	updated, _ = app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	app = updated.(App)
	if len(app.dialogs.Dialogs()) != 1 || app.dialogs.ActiveDialogID() == palettedialog.DialogID {
		t.Fatal("palette replaced input dialog")
	}
	updated, _ = app.Update(dialogs.CloseDialogMsg{Action: palettedialog.ActionMsg{ID: int(viewDead)}})
	if updated.(App).activeViewID() != viewDashboard {
		t.Fatal("unowned action navigated")
	}
}

func TestAppPaletteQueueDiscoveryAndNavigation(t *testing.T) {
	mr := miniredis.RunT(t)
	_, _ = mr.SetAdd("queues", "default", "batches")
	client, err := sidekiq.NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Redis().Ping(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	hook := &queueNavigationHook{}
	client.AddHook(hook)
	app := New(client, "", false, nil)
	defer app.cancel()
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = updated.(App)
	updated, load := app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	app = updated.(App)
	if len(hook.commands) != 0 {
		t.Fatal("opening palette fetched queues synchronously")
	}
	// Type while queue discovery is pending.
	updated, _ = app.Update(tea.PasteMsg{Content: "batches"})
	app = updated.(App)
	app = applyLayoutRead(app, load)
	if !slices.Equal(hook.commands, []string{"smembers"}) {
		t.Fatalf("discovery commands=%v", hook.commands)
	}
	output := app.dialogs.ActiveModel().View()
	if !strings.Contains(output, "Queue: batches") {
		t.Fatalf("queue did not match pending query: %s", output)
	}
	updated, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = updated.(App)
	updated, cmd = app.Update(cmd())
	app = updated.(App)
	app = applyLayoutRead(app, cmd)
	if app.activeViewID() != viewQueueDetails || app.dialogs.HasDialogs() {
		t.Fatal("queue selection did not navigate and close")
	}
	provider := app.viewRegistry[viewQueueDetails].(views.ContextProvider)
	if !slices.ContainsFunc(provider.ContextItems(), func(item views.ContextItem) bool { return item.Label == "Queue" && ansi.Strip(item.Value) == "batches" }) {
		t.Fatalf("wrong queue selected: %#v; view: %s", provider.ContextItems(), app.viewRegistry[viewQueueDetails].View())
	}
	// Discovery is a new snapshot on each opening.
	_, _ = mr.SetAdd("queues", "new-queue")
	*hook = queueNavigationHook{}
	updated, load = app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	app = updated.(App)
	updated, _ = app.Update(tea.PasteMsg{Content: "new-queue"})
	app = updated.(App)
	app = applyLayoutRead(app, load)
	if !slices.Equal(hook.commands, []string{"smembers"}) || !strings.Contains(app.dialogs.ActiveModel().View(), "Queue: new-queue") {
		t.Fatal("reopening did not refresh queue names once")
	}
}

func TestAppPaletteTakesFocusBeforeCommandsRun(t *testing.T) {
	app := New(&ownershipClient{}, "", false, nil)
	defer app.cancel()
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = updated.(App)
	updated, _ = app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	app = updated.(App)
	// Do not run any returned commands before the queued input arrives.
	for _, r := range "q1?" {
		updated, _ = app.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		app = updated.(App)
	}
	if app.dialogs.ActiveDialogID() != palettedialog.DialogID || app.activeViewID() != viewDashboard || !strings.Contains(ansi.Strip(app.dialogs.ActiveModel().View()), "q1?") {
		t.Fatal("immediate input escaped palette")
	}
	app.dialogs.ActiveModel().(*palettedialog.Model).Close()
}

type paletteProcessesClient struct{ busyHeaderClient }

func (c paletteProcessesClient) GetProcessesData(context.Context) ([]sidekiq.Process, error) {
	return c.data.Processes, nil
}

func TestAppPaletteProcessSelection(t *testing.T) {
	client := paletteProcessesClient{busyHeaderClient{data: sidekiq.BusyData{Processes: []sidekiq.Process{{Identity: "worker:123:nonce", Hostname: "worker", PID: 123, Concurrency: 10}}}}}
	app := New(client, "", false, nil)
	defer app.cancel()
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app = updated.(App)
	updated, _ = app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	app = updated.(App)
	updated, _ = app.Update(tea.PasteMsg{Content: "Busy: processes"})
	app = updated.(App)
	updated, closeCmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = updated.(App)
	updated, navigate := app.Update(closeCmd())
	app = applyLayoutRead(updated.(App), navigate)
	if app.activeViewID() != viewProcessesList {
		t.Fatal("process list did not open")
	}
	updated, selectProcess := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = updated.(App)
	updated, refresh := app.Update(selectProcess())
	app = applyLayoutRead(updated.(App), refresh)
	if app.activeViewID() != viewBusy || !strings.Contains(ansi.Strip(app.viewRegistry[viewBusy].View()), "Active Jobs on worker:123") {
		t.Fatal("selected process lost on return to Busy")
	}
}

type paletteQueuesClient struct{ *ownershipClient }

func (paletteQueuesClient) GetQueues(context.Context) ([]*sidekiq.Queue, error) {
	var client sidekiq.Client
	return []*sidekiq.Queue{client.NewQueue("batches")}, nil
}

func TestAppCoveredPaletteReceivesQueueResults(t *testing.T) {
	app := New(paletteQueuesClient{&ownershipClient{}}, "", false, nil)
	defer app.cancel()
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = updated.(App)
	app.setActiveView(viewBusy)
	// A previously requested filter can open after the palette takes focus.
	_, filterOpen := app.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	updated, load := app.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	app = updated.(App)
	updated, _ = app.Update(filterOpen())
	app = updated.(App)
	app = applyLayoutRead(app, load)
	if app.dialogs.ActiveDialogID() != filterdialog.DialogID {
		t.Fatal("background reply changed active dialog")
	}
	updated, closeCmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	app = updated.(App)
	updated, _ = app.Update(closeCmd())
	app = updated.(App)
	updated, _ = app.Update(tea.PasteMsg{Content: "batches"})
	app = updated.(App)
	output := ansi.Strip(app.dialogs.ActiveModel().View())
	if strings.Contains(output, "loading queues") || !strings.Contains(output, "Queue: batches") {
		t.Fatalf("covered palette lost discovery result: %s", output)
	}
	app.dialogs.ActiveModel().(*palettedialog.Model).Close()
}
