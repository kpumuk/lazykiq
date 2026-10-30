package confirm

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

func keyCode(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func keyText(text string) tea.KeyPressMsg {
	var code rune
	for _, r := range text {
		code = r
		break
	}
	return tea.KeyPressMsg(tea.Key{Text: text, Code: code})
}

func updateModel(t *testing.T, m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	updated, ok := next.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", next)
	}
	return updated, cmd
}

func TestConfirmDialogCompletion(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		key       tea.KeyPressMsg
		selection Selection
		confirmed bool
	}{
		"yes":        {keyText("y"), SelectionNo, true},
		"no":         {keyText("n"), SelectionYes, false},
		"escape":     {keyCode(tea.KeyEsc), SelectionYes, false},
		"enter yes":  {keyCode(tea.KeyEnter), SelectionYes, true},
		"enter no":   {keyCode(tea.KeyEnter), SelectionNo, false},
		"enter none": {keyCode(tea.KeyEnter), SelectionNone, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New(WithTarget("queue"))
			m.selection = tc.selection
			_, cmd := updateModel(t, m, tc.key)
			result, ok := cmd().(dialogs.CloseDialogMsg)
			if !ok {
				t.Fatal("confirmation must return one result-and-close message")
			}
			action, ok := result.Action.(ActionMsg)
			if !ok || action.Confirmed != tc.confirmed || action.Target != "queue" {
				t.Fatalf("action = %#v", result.Action)
			}
			m.target = "replacement"
			if again := cmd().(dialogs.CloseDialogMsg).Action.(ActionMsg); again != action {
				t.Fatalf("command read mutable target: %#v", again)
			}
		})
	}
}

func TestConfirmDialogSelectionMoves(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		start Selection
		msg   tea.Msg
		want  Selection
	}{
		"left from no":       {start: SelectionNo, msg: keyCode(tea.KeyLeft), want: SelectionYes},
		"right from yes":     {start: SelectionYes, msg: keyCode(tea.KeyRight), want: SelectionNo},
		"tab from no":        {start: SelectionNo, msg: keyCode(tea.KeyTab), want: SelectionYes},
		"shift tab from yes": {start: SelectionYes, msg: tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}), want: SelectionNo},
		"h from no":          {start: SelectionNo, msg: keyText("h"), want: SelectionYes},
		"l from yes":         {start: SelectionYes, msg: keyText("l"), want: SelectionNo},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New()
			m.selection = tc.start
			m.Init()

			m, _ = updateModel(t, m, tc.msg)
			if m.selection != tc.want {
				t.Fatalf("selection = %v, want %v", m.selection, tc.want)
			}
		})
	}
}

func TestConfirmDialogWindowSizing(t *testing.T) {
	t.Parallel()

	m := New(WithMessage("Delete these jobs?"))
	m.Init()

	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 60 {
		t.Fatalf("width = %d, want %d", m.width, 60)
	}
	if m.height != 5 {
		t.Fatalf("height = %d, want %d", m.height, 5)
	}
	if m.row != 17 {
		t.Fatalf("row = %d, want %d", m.row, 17)
	}
	if m.col != 30 {
		t.Fatalf("col = %d, want %d", m.col, 30)
	}
}

func TestConfirmDialogViewDimensions(t *testing.T) {
	t.Parallel()

	m := New(WithTitle("Confirm"), WithMessage("Delete these jobs?"))
	m.Init()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	output := ansi.Strip(m.View())
	lines := strings.Split(output, "\n")
	if len(lines) != m.height {
		t.Fatalf("lines = %d, want %d", len(lines), m.height)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w != m.width {
			t.Fatalf("line %d width = %d, want %d", i, w, m.width)
		}
	}
}

func TestGoldenConfirmDialog(t *testing.T) {
	m := New(
		WithTitle("Confirm"),
		WithMessage("Delete 3 retries from critical queue?"),
		WithTarget("critical"),
	)
	m.Init()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
