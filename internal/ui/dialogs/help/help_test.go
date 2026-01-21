package help

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
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

func collectMsgs(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	switch m := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, collectMsgs(t, c)...)
		}
		return out
	default:
		return []tea.Msg{m}
	}
}

func sampleSections() []Section {
	return []Section{
		{
			Title: "General",
			Bindings: []key.Binding{
				key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
				key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
			},
			Column: ColumnLeft,
		},
		{
			Title: "Navigation",
			Bindings: []key.Binding{
				key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/down", "down")),
				key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/up", "up")),
			},
			Column: ColumnRight,
		},
		{
			Title:  "Filters",
			Lines:  []string{"type to filter", "enter to apply"},
			Column: ColumnAuto,
		},
		{
			Title: "Stacks",
			Bindings: []key.Binding{
				key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			},
			Column: ColumnAuto,
		},
	}
}

func TestHelpDialogWindowSizing(t *testing.T) {
	t.Parallel()

	m := New(WithSections(sampleSections()))
	m.Init()

	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 80 {
		t.Fatalf("width = %d, want %d", m.width, 80)
	}
	if m.height != 20 {
		t.Fatalf("height = %d, want %d", m.height, 20)
	}
	if m.row != 10 {
		t.Fatalf("row = %d, want %d", m.row, 10)
	}
	if m.col != 20 {
		t.Fatalf("col = %d, want %d", m.col, 20)
	}
}

func TestHelpDialogCloseKeys(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		msg tea.Msg
	}{
		"question": {msg: keyText("?")},
		"escape":   {msg: keyCode(tea.KeyEsc)},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New(WithSections(sampleSections()))
			m.Init()

			_, cmd := updateModel(t, m, tc.msg)
			msgs := collectMsgs(t, cmd)
			if len(msgs) != 1 {
				t.Fatalf("messages = %d, want 1", len(msgs))
			}
			if _, ok := msgs[0].(dialogs.CloseDialogMsg); !ok {
				t.Fatalf("message type = %T, want dialogs.CloseDialogMsg", msgs[0])
			}
		})
	}
}

func TestHelpDialogScrollClamp(t *testing.T) {
	t.Parallel()

	lines := make([]string, 0, 20)
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+string(rune('a'+i-1)))
	}
	sections := []Section{{Title: "Many", Lines: lines}}

	m := New(WithSections(sections))
	m.Init()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 14})

	m, _ = updateModel(t, m, keyCode(tea.KeyEnd))
	if m.yOffset != m.maxOffset() {
		t.Fatalf("yOffset = %d, want %d", m.yOffset, m.maxOffset())
	}

	m, _ = updateModel(t, m, keyCode(tea.KeyHome))
	if m.yOffset != 0 {
		t.Fatalf("yOffset = %d, want %d", m.yOffset, 0)
	}
}

func TestHelpDialogSplitSections(t *testing.T) {
	t.Parallel()

	sections := []Section{
		{Title: "Left", Column: ColumnLeft},
		{Title: "Right", Column: ColumnRight},
		{Title: "Auto1", Column: ColumnAuto},
		{Title: "Auto2", Column: ColumnAuto},
	}

	left, right := splitSections(sections)
	if len(left) != 2 {
		t.Fatalf("left count = %d, want %d", len(left), 2)
	}
	if len(right) != 2 {
		t.Fatalf("right count = %d, want %d", len(right), 2)
	}
}

func TestHelpDialogRenderSectionsContains(t *testing.T) {
	t.Parallel()

	sections := []Section{
		{
			Title: "General",
			Bindings: []key.Binding{
				key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
				key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "hidden"), key.WithDisabled()),
			},
			Lines: []string{"extra info"},
		},
	}

	lines := renderSections(sections, 30, Styles{})
	content := strings.Join(lines, "\n")
	if !strings.Contains(content, "General") {
		t.Fatalf("expected title to be rendered, got %q", content)
	}
	if !strings.Contains(content, "q") || !strings.Contains(content, "quit") {
		t.Fatalf("expected key binding to be rendered, got %q", content)
	}
	if strings.Contains(content, "hidden") {
		t.Fatalf("unexpected disabled binding in output: %q", content)
	}
	if !strings.Contains(content, "extra info") {
		t.Fatalf("expected custom line to be rendered, got %q", content)
	}
}

func TestGoldenHelpDialog(t *testing.T) {
	m := New(WithSections(sampleSections()))
	m.Init()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
