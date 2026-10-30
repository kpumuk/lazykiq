package devtools

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	coredevtools "github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
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

func TestDevToolsNavigationFocus(t *testing.T) {
	for name, tc := range map[string]struct {
		key    tea.KeyPressMsg
		cursor int
		text   string
	}{
		"j":         {key: keyText("j"), cursor: 16, text: "j"},
		"k":         {key: keyText("k"), cursor: 14, text: "k"},
		"down":      {key: keyCode(tea.KeyDown), cursor: 16},
		"up":        {key: keyCode(tea.KeyUp), cursor: 14},
		"page down": {key: keyCode(tea.KeyPgDown), cursor: 25},
		"page up":   {key: keyCode(tea.KeyPgUp), cursor: 5},
	} {
		for _, inputFocused := range []bool{true, false} {
			t.Run(name+"/input="+strconv.FormatBool(inputFocused), func(t *testing.T) {
				m := New()
				m.Init()
				rows := make([]table.Row, 30)
				for i := range rows {
					rows[i] = table.Row{ID: strconv.Itoa(i), Cells: []string{strconv.Itoa(i)}}
				}
				m.table.SetRows(rows)
				m.table.SetCursor(15)
				if !inputFocused {
					m, _ = updateModel(t, m, keyCode(tea.KeyTab))
				}
				m, _ = updateModel(t, m, tc.key)
				wantCursor, wantText := tc.cursor, ""
				if inputFocused && tc.text != "" {
					wantCursor, wantText = 15, tc.text
				}
				if m.table.Cursor() != wantCursor || m.input.Value() != wantText {
					t.Fatalf("cursor/input = %d/%q, want %d/%q", m.table.Cursor(), m.input.Value(), wantCursor, wantText)
				}
			})
		}
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

func TestDevToolsPaste(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ content, want string }{
		"ASCII":     {content: "PING", want: "PING"},
		"Unicode":   {content: "ECHO 任务", want: "ECHO 任务"},
		"multiline": {content: "ECHO\nhello\tworld", want: "ECHO hello world"},
		"CRLF":      {content: "ECHO\r\nhello", want: "ECHO  hello"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New()
			m.Init()
			m, cmd := updateModel(t, m, tea.PasteMsg{Content: tc.content})
			if got := m.input.Value(); got != tc.want {
				t.Fatalf("input = %q, want %q", got, tc.want)
			}
			if cmd == nil {
				t.Fatal("paste lost cursor command")
			}

			m, _ = updateModel(t, m, keyCode(tea.KeyTab))
			m, cmd = updateModel(t, m, tea.PasteMsg{Content: "ignored"})
			if m.input.Value() != tc.want || cmd != nil {
				t.Fatal("table-focused paste changed input or started cursor work")
			}
		})
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
	result, ok := msg.(CommandResultMsg)
	if !ok {
		t.Fatalf("message type = %T, want CommandResultMsg", msg)
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
		wantErr string
	}{
		"simple":                     {input: "GET foo", want: []string{"GET", "foo"}},
		"quoted":                     {input: "SET foo \"bar baz\"", want: []string{"SET", "foo", "bar baz"}},
		"escaped":                    {input: "ECHO \"hi \\\"there\\\"\"", want: []string{"ECHO", "hi \"there\""}},
		"empty double final":         {input: `SET key ""`, want: []string{"SET", "key", ""}},
		"empty single final":         {input: `SET key ''`, want: []string{"SET", "key", ""}},
		"empty double middle":        {input: `SET "" value`, want: []string{"SET", "", "value"}},
		"empty single middle":        {input: `SET '' value`, want: []string{"SET", "", "value"}},
		"separate empty arguments":   {input: "  MSET '' \"\"\tkey ''  \n", want: []string{"MSET", "", "", "key", ""}},
		"only empty argument":        {input: `""`, want: []string{""}},
		"adjacent empty quotes":      {input: `ECHO ''""`, want: []string{"ECHO", ""}},
		"concatenated quotes":        {input: `SET pre""fix 'a'"b"c`, want: []string{"SET", "prefix", "abc"}},
		"escaped space":              {input: `SET some\ key ''`, want: []string{"SET", "some key", ""}},
		"escaped quotes":             {input: `ECHO \"\"`, want: []string{"ECHO", `""`}},
		"escaped backslash":          {input: `ECHO \\`, want: []string{"ECHO", `\`}},
		"empty":                      {input: "   ", wantErr: "empty command"},
		"no input":                   {wantErr: "empty command"},
		"unterminated":               {input: `"oops`, wantErr: "unterminated quoted string"},
		"unterminated single":        {input: `ECHO 'oops`, wantErr: "unterminated quoted string"},
		"trailing escape":            {input: `ECHO foo\`, wantErr: "unterminated quoted string"},
		"empty then unterminated":    {input: `SET key ''"`, wantErr: "unterminated quoted string"},
		"empty then trailing escape": {input: `SET key ""\`, wantErr: "unterminated quoted string"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			args, err := parseRedisArgs(tc.input)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
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

func TestDevToolsExecuteEmptyQuotedValue(t *testing.T) {
	t.Parallel()

	for name, input := range map[string]string{
		"double quotes": `SET key ""`,
		"single quotes": `SET key ''`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := miniredis.RunT(t)
			if err := server.Set("key", "previous"); err != nil {
				t.Fatal(err)
			}
			client, err := sidekiq.NewClient("redis://" + server.Addr())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			m := New(WithClient(client))
			m.Init()
			m.input.SetValue(input)
			_, cmd := updateModel(t, m, keyCode(tea.KeyEnter))
			if cmd == nil {
				t.Fatal("Enter did not execute the command")
			}
			result, ok := cmd().(CommandResultMsg)
			if !ok || result.err != nil || result.output != "OK" {
				t.Fatalf("console result = %+v, want OK", result)
			}
			value, err := server.Get("key")
			if err != nil || value != "" {
				t.Fatalf("stored value = %q, error = %v, want empty value", value, err)
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

type consoleClient struct {
	do func(context.Context, ...any) (any, error)
}

func (c consoleClient) Do(ctx context.Context, args ...any) (any, error) { return c.do(ctx, args...) }

func TestDevToolsExecuteFullInput(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("界", 65536)
	resultValue := []any{payload, []any{payload}}
	tracker := coredevtools.NewTracker()
	m := New(WithTracker(tracker), WithClient(consoleClient{do: func(ctx context.Context, args ...any) (any, error) {
		if coredevtools.OriginFromContext(ctx) != "console" || len(args) != 3 || args[0] != "SET" || args[1] != "key" || args[2] != payload {
			t.Error("execution arguments truncated or origin lost")
		}
		return resultValue, nil
	}}))
	m.input.SetValue("SET key \"" + payload + "\"")
	cmd := m.executeInput()
	if m.input.Value() != "" {
		t.Fatal("input not cleared")
	}
	msg, ok := cmd().(CommandResultMsg)
	if !ok || msg.err != nil || len(msg.output) > coredevtools.DisplayByteLimit || !strings.Contains(msg.output, "[truncated]") {
		t.Fatal("unbounded or invalid result message")
	}
	if resultValue[0] != payload {
		t.Fatal("actual result changed")
	}
	m.Update(msg)
	entries := tracker.LogEntries()
	if len(entries) != 1 || entries[0].Entry.Command != msg.output || entries[0].Entry.Kind != coredevtools.EntryResult {
		t.Fatal("result not logged")
	}
}

func TestDevToolsResultErrors(t *testing.T) {
	t.Parallel()
	tests := map[string]error{
		"cancellation": context.Canceled,
		"wide":         errors.New(strings.Repeat("failure\r\n", 65536)),
	}
	for name, err := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tracker := coredevtools.NewTracker()
			m := New(WithTracker(tracker), WithClient(consoleClient{do: func(context.Context, ...any) (any, error) { return "ignored", err }}))
			m.input.SetValue("GET key")
			msg := m.executeInput()()
			m.Update(msg)
			output := tracker.LogEntries()[0].Entry.Command
			if len(output) > coredevtools.DisplayByteLimit || !strings.HasPrefix(output, "error: ") || strings.ContainsAny(output, "\r\n") || strings.Contains(output, "ignored") {
				t.Fatal("invalid error preview")
			}
			if name == "cancellation" && output != "error: context canceled" {
				t.Fatal(output)
			}
			if name == "wide" && !strings.HasSuffix(output, " [truncated]") {
				t.Fatal("missing truncation indicator")
			}
		})
	}
}

func TestDevToolsTruncatedLayout(t *testing.T) {
	t.Parallel()
	for _, width := range []int{80, 120, 160} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Parallel()
			tracker := seedTracker()
			m := New(WithTracker(tracker))
			appendResult(tracker, formatRedisResult(strings.Repeat("界", 4096)), nil)
			m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			m.syncEntries()
			m.table.ScrollToEnd()
			output := ansi.Strip(m.View())
			if !strings.Contains(output, "[truncated]") {
				t.Fatal("truncation indicator not accessible at horizontal end")
			}
			lines := strings.Split(output, "\n")
			if len(lines) != m.height {
				t.Fatalf("height=%d, want %d", len(lines), m.height)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != width {
					t.Fatalf("line width=%d, want %d", ansi.StringWidth(line), width)
				}
			}
		})
	}
}

func TestGoldenDevToolsTruncated(t *testing.T) {
	tracker := seedTracker()
	tracker.AppendLog(coredevtools.LogEntry{
		Time: time.Date(2024, 1, 2, 3, 4, 6, 0, time.UTC), Origin: "console",
		Entry: coredevtools.Entry{Kind: coredevtools.EntryResult, Command: formatRedisResult(strings.Repeat("界", 4096))},
	})
	m := New(WithTracker(tracker))
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.syncEntries()
	m.table.ScrollToEnd()
	golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
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

func TestCommandResultCollection(t *testing.T) {
	t.Parallel()
	tracker := coredevtools.NewTracker()
	m := New(WithTracker(tracker), WithClient(consoleClient{do: func(context.Context, ...any) (any, error) { return "PONG", nil }}))
	m.input.SetValue("PING")
	first := m.executeInput()().(CommandResultMsg)
	m.input.SetValue("PING")
	second := m.executeInput()().(CommandResultMsg)
	other := coredevtools.NewTracker()
	first.Collect(other)
	if len(other.LogEntries()) != 0 {
		t.Fatal("result reached unrelated tracker")
	}
	second.Collect(tracker)
	first.Collect(tracker)
	m.Update(first)
	m.Update(second)
	if len(tracker.LogEntries()) != 2 {
		t.Fatal("independent operations with identical output were lost or duplicated")
	}

	// Eviction must not make an old completion eligible for collection again.
	for range 700 {
		m.input.SetValue("PING")
		msg := m.executeInput()()
		m.Update(msg)
	}
	before := tracker.LogEntries()
	if len(before) != 500 || before[0].Seq != 202 || before[499].Seq != 701 {
		t.Fatalf("unexpected retained log: count=%d first=%d last=%d", len(before), before[0].Seq, before[len(before)-1].Seq)
	}
	first.Collect(tracker)
	after := tracker.LogEntries()
	if len(after) != 500 || after[0].Seq != before[0].Seq || after[499].Seq != before[499].Seq {
		t.Fatal("duplicate resurrected an evicted result")
	}
}

func TestCommandResultExpiredLifetime(t *testing.T) {
	t.Parallel()
	tracker := coredevtools.NewTracker()
	m := New(WithTracker(tracker), WithClient(consoleClient{do: func(context.Context, ...any) (any, error) { return "PONG", nil }}))
	ctx, cancel := context.WithCancel(context.Background())
	m.SetContext(ctx)
	m.input.SetValue("PING")
	msg := m.executeInput()()
	cancel()
	m.Update(msg)
	if len(tracker.LogEntries()) != 0 {
		t.Fatal("expired completion appended a result")
	}
}

func TestCommandResultWithoutClient(t *testing.T) {
	t.Parallel()
	tracker := coredevtools.NewTracker()
	m := New(WithTracker(tracker))
	m.input.SetValue("PING")
	msg := m.executeInput()()
	m.Update(msg)
	m.Update(msg)
	if entries := tracker.LogEntries(); len(entries) != 1 || entries[0].Entry.Command != "error: redis client not available" {
		t.Fatalf("missing-client log = %+v", entries)
	}
}
