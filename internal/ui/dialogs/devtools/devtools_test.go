package devtools

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	coredevtools "github.com/kpumuk/lazykiq/internal/devtools"
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

func seedTracker() *coredevtools.Tracker {
	tracker := coredevtools.NewTracker()
	tracker.AppendLog(coredevtools.LogEntry{
		Time:   time.Date(2024, 1, 2, 3, 4, 5, 678000000, time.UTC),
		Origin: "worker",
		Entry: coredevtools.Entry{
			Kind:     coredevtools.EntryCommand,
			Command:  "GET critical",
			Duration: 3 * time.Millisecond,
		},
	})
	tracker.AppendLog(coredevtools.LogEntry{
		Time:   time.Date(2024, 1, 2, 3, 4, 5, 900000000, time.UTC),
		Origin: "console",
		Entry: coredevtools.Entry{
			Kind:     coredevtools.EntryResult,
			Command:  "ok",
			Duration: 0,
		},
	})
	return tracker
}

func TestDevToolsCloseKeys(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		msg tea.Msg
	}{
		"f12":   {msg: keyCode(tea.KeyF12)},
		"tilde": {msg: keyText("~")},
		"esc":   {msg: keyCode(tea.KeyEsc)},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New()
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

func TestDevToolsToggleFocus(t *testing.T) {
	t.Parallel()

	m := New()
	m.Init()

	m, _ = updateModel(t, m, keyCode(tea.KeyTab))
	if m.inputFocused {
		t.Fatalf("inputFocused = true, want false")
	}

	m, _ = updateModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	if !m.inputFocused {
		t.Fatalf("inputFocused = false, want true")
	}
}

func TestDevToolsCtrlUClearsInput(t *testing.T) {
	t.Parallel()

	m := New()
	m.Init()
	m.input.SetValue("get foo")
	m.input.CursorEnd()

	m, _ = updateModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	if got := m.input.Value(); got != "" {
		t.Fatalf("input value = %q, want empty", got)
	}
}

func TestDevToolsExecuteInputNoClient(t *testing.T) {
	t.Parallel()

	m := New()
	m.Init()
	m.input.SetValue("get foo")
	m.input.CursorEnd()

	cmd := m.executeInput()
	if got := m.input.Value(); got != "" {
		t.Fatalf("input value = %q, want empty", got)
	}
	msg := cmd()
	result, ok := msg.(commandResultMsg)
	if !ok {
		t.Fatalf("message type = %T, want commandResultMsg", msg)
	}
	if result.err == nil || !strings.Contains(result.err.Error(), "redis client not available") {
		t.Fatalf("error = %v, want redis client not available", result.err)
	}
}

func TestDevToolsParseRedisArgs(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    []string
		wantErr bool
	}{
		"simple":       {input: "GET foo", want: []string{"GET", "foo"}},
		"quoted":       {input: "SET foo \"bar baz\"", want: []string{"SET", "foo", "bar baz"}},
		"escaped":      {input: "ECHO \"hi \\\"there\\\"\"", want: []string{"ECHO", "hi \"there\""}},
		"empty":        {input: "   ", wantErr: true},
		"unterminated": {input: "\"oops", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			args, err := parseRedisArgs(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(args) != len(tc.want) {
				t.Fatalf("args len = %d, want %d", len(args), len(tc.want))
			}
			for i := range tc.want {
				if args[i] != tc.want[i] {
					t.Fatalf("args[%d] = %q, want %q", i, args[i], tc.want[i])
				}
			}
		})
	}
}

func TestDevToolsSyncEntries(t *testing.T) {
	t.Parallel()

	tracker := seedTracker()
	m := New(WithTracker(tracker))
	m.syncEntries()

	rows := m.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want %d", len(rows), 2)
	}
	if m.table.Cursor() != len(rows)-1 {
		t.Fatalf("cursor = %d, want %d", m.table.Cursor(), len(rows)-1)
	}
}

func TestDevToolsViewDimensions(t *testing.T) {
	t.Parallel()

	m := New(WithTracker(seedTracker()))
	m.Init()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})

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

func TestGoldenDevToolsDialog(t *testing.T) {
	m := New(WithTracker(seedTracker()))
	m.Init()
	m.input.SetValue("get critical")
	m.input.CursorEnd()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})

	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
