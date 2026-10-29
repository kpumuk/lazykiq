package filter

import (
	"strconv"
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

func keyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: r, Mod: tea.ModCtrl})
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

func TestFilterDialogEnterApplyAndClear(t *testing.T) {
	t.Parallel()

	m := New(WithQuery("old"))
	m.Init()
	m.input.SetValue("new")
	m.input.CursorEnd()

	m, cmd := updateModel(t, m, keyCode(tea.KeyEnter))
	msgs := collectMsgs(t, cmd)

	var gotAction *ActionMsg
	gotClose := false
	for _, msg := range msgs {
		switch v := msg.(type) {
		case ActionMsg:
			gotAction = &v
		case dialogs.CloseDialogMsg:
			gotClose = true
		default:
			t.Fatalf("unexpected message %T", msg)
		}
	}
	if gotAction == nil {
		t.Fatal("expected ActionMsg")
	}
	if gotAction.Action != ActionApply {
		t.Fatalf("Action = %v, want %v", gotAction.Action, ActionApply)
	}
	if gotAction.Query != "new" {
		t.Fatalf("Query = %q, want %q", gotAction.Query, "new")
	}
	if !gotClose {
		t.Fatal("expected CloseDialogMsg")
	}
	if m.query != "new" {
		t.Fatalf("query = %q, want %q", m.query, "new")
	}

	m = New(WithQuery("old"))
	m.Init()
	m.input.SetValue("   ")
	m.input.CursorEnd()

	m, cmd = updateModel(t, m, keyCode(tea.KeyEnter))
	msgs = collectMsgs(t, cmd)

	gotAction = nil
	gotClose = false
	for _, msg := range msgs {
		switch v := msg.(type) {
		case ActionMsg:
			gotAction = &v
		case dialogs.CloseDialogMsg:
			gotClose = true
		default:
			t.Fatalf("unexpected message %T", msg)
		}
	}
	if gotAction == nil {
		t.Fatal("expected ActionMsg")
	}
	if gotAction.Action != ActionClear {
		t.Fatalf("Action = %v, want %v", gotAction.Action, ActionClear)
	}
	if gotAction.Query != "" {
		t.Fatalf("Query = %q, want empty", gotAction.Query)
	}
	if !gotClose {
		t.Fatal("expected CloseDialogMsg")
	}
	if m.query != "" {
		t.Fatalf("query = %q, want empty", m.query)
	}
}

func TestFilterDialogEnterUnchangedCloses(t *testing.T) {
	t.Parallel()

	m := New(WithQuery("same"))
	m.Init()

	_, cmd := updateModel(t, m, keyCode(tea.KeyEnter))
	msgs := collectMsgs(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	if _, ok := msgs[0].(dialogs.CloseDialogMsg); !ok {
		t.Fatalf("message type = %T, want dialogs.CloseDialogMsg", msgs[0])
	}
}

func TestFilterDialogEnterTrimmedSameCloses(t *testing.T) {
	t.Parallel()

	m := New(WithQuery("same"))
	m.Init()
	m.input.SetValue("  same  ")
	m.input.CursorEnd()

	_, cmd := updateModel(t, m, keyCode(tea.KeyEnter))
	msgs := collectMsgs(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	if _, ok := msgs[0].(dialogs.CloseDialogMsg); !ok {
		t.Fatalf("message type = %T, want dialogs.CloseDialogMsg", msgs[0])
	}
}

func TestFilterDialogEnterTrimsWhitespace(t *testing.T) {
	t.Parallel()

	m := New()
	m.Init()
	m.input.SetValue("  spaced  ")
	m.input.CursorEnd()

	_, cmd := updateModel(t, m, keyCode(tea.KeyEnter))
	msgs := collectMsgs(t, cmd)

	var gotAction *ActionMsg
	for _, msg := range msgs {
		if v, ok := msg.(ActionMsg); ok {
			gotAction = &v
		}
	}
	if gotAction == nil {
		t.Fatal("expected ActionMsg")
	}
	if gotAction.Action != ActionApply {
		t.Fatalf("Action = %v, want %v", gotAction.Action, ActionApply)
	}
	if gotAction.Query != "spaced" {
		t.Fatalf("Query = %q, want %q", gotAction.Query, "spaced")
	}
}

func TestFilterDialogCtrlUClearsInput(t *testing.T) {
	t.Parallel()

	m := New()
	m.Init()
	m.input.SetValue("abc")
	m.input.CursorEnd()

	m, _ = updateModel(t, m, keyCtrl('u'))
	if got := m.input.Value(); got != "" {
		t.Fatalf("input value = %q, want empty", got)
	}
}

func TestFilterDialogPaste(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ content, want string }{
		"ASCII":     {content: "critical", want: "critical"},
		"Unicode":   {content: "任务 café", want: "任务 café"},
		"multiline": {content: "one\ntwo\tthree", want: "one two three"},
		"CRLF":      {content: "one\r\ntwo", want: "one  two"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New(WithQuery("[]"))
			m.Init()
			m.input.SetCursor(1)
			m, cmd := updateModel(t, m, tea.PasteMsg{Content: tc.content})
			if got, want := m.input.Value(), "["+tc.want+"]"; got != want {
				t.Fatalf("input = %q, want %q", got, want)
			}
			if cmd == nil {
				t.Fatal("paste lost cursor command")
			}
			if m.query != "[]" {
				t.Fatal("paste applied query before submission")
			}
		})
	}
}

func TestFilterDialogEscCloses(t *testing.T) {
	t.Parallel()

	m := New(WithQuery("keep"))
	m.Init()
	m.input.SetValue("new")
	m.input.CursorEnd()

	_, cmd := updateModel(t, m, keyCode(tea.KeyEsc))
	msgs := collectMsgs(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	if _, ok := msgs[0].(dialogs.CloseDialogMsg); !ok {
		t.Fatalf("message type = %T, want dialogs.CloseDialogMsg", msgs[0])
	}
	if m.query != "keep" {
		t.Fatalf("query = %q, want %q", m.query, "keep")
	}
}

func TestFilterDialogWindowSizing(t *testing.T) {
	t.Parallel()

	m := New()
	m.Init()

	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 60 {
		t.Fatalf("width = %d, want %d", m.width, 60)
	}
	if m.height != 3 {
		t.Fatalf("height = %d, want %d", m.height, 3)
	}
	if m.row != 18 {
		t.Fatalf("row = %d, want %d", m.row, 18)
	}
	if m.col != 30 {
		t.Fatalf("col = %d, want %d", m.col, 30)
	}
	if got := m.input.Width(); got != 55 {
		t.Fatalf("input width = %d, want %d", got, 55)
	}
}

func TestFilterDialogWindowSizingMinWidth(t *testing.T) {
	t.Parallel()

	m := New()
	m.Init()

	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 30, Height: 10})
	if m.width != 26 {
		t.Fatalf("width = %d, want %d", m.width, 26)
	}
	if m.height != 3 {
		t.Fatalf("height = %d, want %d", m.height, 3)
	}
	if m.row != 3 {
		t.Fatalf("row = %d, want %d", m.row, 3)
	}
	if m.col != 2 {
		t.Fatalf("col = %d, want %d", m.col, 2)
	}
	if got := m.input.Width(); got != 21 {
		t.Fatalf("input width = %d, want %d", got, 21)
	}
}

func TestFilterDialogViewDimensions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		windowWidth  int
		windowHeight int
	}{
		"normal window": {windowWidth: 80, windowHeight: 24},
		"small window":  {windowWidth: 30, windowHeight: 10},
		"narrow window": {windowWidth: 25, windowHeight: 8},
		"short window":  {windowWidth: 50, windowHeight: 4},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New(WithQuery("critical"))
			m.Init()
			m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: tc.windowWidth, Height: tc.windowHeight})

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
		})
	}
}

func TestGoldenFilterDialog(t *testing.T) {
	m := New(WithQuery("critical"))
	m.Init()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestInputResize(t *testing.T) {
	t.Parallel()

	for name, text := range map[string]string{
		"ASCII":     strings.Repeat("abcdefghij", 16),
		"wide":      strings.Repeat("任务世界", 40),
		"combining": strings.Repeat("cafe\u0301", 32),
	} {
		for position, pos := range map[string]int{
			"beginning": 0,
			"middle":    len([]rune(text)) / 2,
			"end":       len([]rune(text)),
		} {
			t.Run(name+"/"+position, func(t *testing.T) {
				t.Parallel()
				m := New()
				m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 240, Height: 30})
				m.Init()
				m.input.SetValue(text)
				m.input.SetCursor(pos)
				for _, width := range []int{180, 80, 44, 100, 400, 44} {
					m, cmd := updateModel(t, m, tea.WindowSizeMsg{Width: width, Height: 30})
					if cmd != nil {
						t.Fatal("resize restarted cursor work")
					}
					input := m.input.View()
					output := m.View()
					if m.input.View() != input {
						t.Fatal("render changed input geometry")
					}
					visible := strings.TrimSpace(strings.TrimPrefix(ansi.Strip(input), m.input.Prompt))
					if got, want := ansi.StringWidth(visible), min(m.input.Width(), ansi.StringWidth(text)); got < want-1 {
						t.Fatalf("width %d: visible text width %d, want at least %d", width, got, want-1)
					}
					if m.input.Position() != pos || m.input.Value() != text || !m.input.Focused() {
						t.Fatal("resize changed editing state")
					}
					// The virtual cursor is the reverse-video cell in the rendered row.
					cursor := strings.Index(output, "\x1b[7")
					if cursor < 0 {
						t.Fatalf("width %d: cursor clipped from dialog: %q", width, output)
					}
					rowStart := strings.LastIndex(output[:cursor], "\n") + 1
					x := ansi.StringWidth(output[rowStart:cursor])
					if x < 2 || x >= m.width-2 {
						t.Fatalf("width %d: cursor column %d outside content", width, x)
					}
					char := " "
					if pos < len([]rune(text)) {
						char = string([]rune(text)[pos])
					}
					if !strings.HasPrefix(ansi.Strip(output[cursor:]), char) {
						t.Fatalf("width %d: cursor is not on %q", width, char)
					}
					for line := range strings.SplitSeq(output, "\n") {
						if got := ansi.StringWidth(line); got != m.width {
							t.Fatalf("width %d: dialog row width %d, want %d", width, got, m.width)
						}
					}
				}
			})
		}
	}
}

func TestGoldenInputResize(t *testing.T) {
	for name, tc := range map[string]struct {
		text     string
		position int
	}{
		"ASCII_end":      {text: strings.Repeat("abcdefghij", 8), position: 80},
		"Unicode_middle": {text: strings.Repeat("任务cafe\u0301", 16), position: 56},
	} {
		t.Run(name, func(t *testing.T) {
			m := New()
			m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 180, Height: 30})
			m.Init()
			m.input.SetValue(tc.text)
			m.input.SetCursor(tc.position)
			for _, width := range []int{44, 120} {
				m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: width, Height: 30})
				t.Run(strconv.Itoa(width), func(t *testing.T) {
					golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
				})
			}
		})
	}
}
