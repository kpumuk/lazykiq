package filter

import (
	"testing"

	tea "charm.land/bubbletea/v2"

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
