package views

import (
	"bytes"
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type clipboardTestModel struct {
	cmd tea.Cmd
}

func (m clipboardTestModel) Init() tea.Cmd { return tea.Sequence(m.cmd, tea.Quit) }

func (m clipboardTestModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

func (m clipboardTestModel) View() tea.View { return tea.NewView("") }

func TestCopyTextCmdWritesOSC52(t *testing.T) {
	for name, tc := range map[string]struct {
		text     string
		wantCmd  bool
		sequence string
	}{
		"job id": {
			text:     "jid-123",
			wantCmd:  true,
			sequence: "\x1b]52;c;amlkLTEyMw==\a",
		},
		"multiline job JSON": {
			text:     "{\n  \"jid\": \"abc\"\n}",
			wantCmd:  true,
			sequence: "\x1b]52;c;ewogICJqaWQiOiAiYWJjIgp9\a",
		},
		"empty text": {
			text: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := copyTextCmd(tc.text)
			if (cmd != nil) != tc.wantCmd {
				t.Fatalf("copyTextCmd(%q) returned command %v, want %v", tc.text, cmd != nil, tc.wantCmd)
			}
			if cmd == nil {
				return
			}

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var out bytes.Buffer
			program := tea.NewProgram(clipboardTestModel{cmd: cmd},
				tea.WithContext(ctx),
				tea.WithInput(nil),
				tea.WithOutput(&out),
				tea.WithoutSignals(),
			)
			if _, err := program.Run(); err != nil {
				t.Fatalf("program.Run() error = %v", err)
			}
			if !bytes.Contains(out.Bytes(), []byte(tc.sequence)) {
				t.Fatalf("terminal output %q does not contain OSC 52 sequence %q", out.String(), tc.sequence)
			}
		})
	}
}
