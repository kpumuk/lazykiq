package filter_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs/devtools"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

func TestDialogTextInputMessagesThroughApp(t *testing.T) {
	t.Parallel()

	for name, newDialog := range map[string]func() dialogs.DialogModel{
		"filter":   func() dialogs.DialogModel { return filter.New() },
		"devtools": func() dialogs.DialogModel { return devtools.New() },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, err := sidekiq.NewClient("redis://localhost:6379/0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			var app tea.Model = ui.New(client, "", false, nil)
			app, _ = app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			dialog := newDialog()
			app, focus := app.Update(dialogs.OpenDialogMsg{Model: dialog})
			if focus == nil {
				t.Fatal("open lost focus command")
			}
			before := dialog.View()
			app, blink := app.Update(focus())
			if blink == nil {
				t.Fatal("focus result lost blink continuation")
			}
			if dialog.View() == before {
				t.Fatal("focus result did not blink cursor")
			}
			app, nextBlink := app.Update(blink())
			if nextBlink == nil || dialog.View() != before {
				t.Fatal("cursor did not continue blinking")
			}
			for _, msg := range []tea.Msg{struct{}{}, cursor.BlinkMsg{}} {
				var cmd tea.Cmd
				app, cmd = app.Update(msg)
				if cmd != nil || dialog.View() != before {
					t.Fatalf("unrelated %T restarted cursor", msg)
				}
			}
			app, paste := app.Update(tea.PasteMsg{Content: "pasted 任务"})
			if paste == nil || !strings.Contains(ansi.Strip(app.View().Content), "pasted 任务") {
				t.Fatal("paste or its command did not reach dialog through App")
			}
			if name == "devtools" {
				app, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
				before = dialog.View()
				app, cmd := app.Update(paste())
				if cmd != nil || dialog.View() != before {
					t.Fatal("blurred input continued blinking")
				}
				app, focus = app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
				if focus == nil {
					t.Fatal("refocus lost cursor command")
				}
				_, cmd = app.Update(focus())
				if cmd == nil {
					t.Fatal("refocused input did not resume blinking")
				}
			}
		})
	}
}
