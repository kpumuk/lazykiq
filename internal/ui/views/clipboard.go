package views

import (
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// copyTextCmd copies text with the native clipboard tool when one is present
// and through the terminal's OSC 52 support, so copying also works over SSH
// and in terminals that reject OSC 52.
func copyTextCmd(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return tea.Batch(
		tea.SetClipboard(text),
		func() tea.Msg {
			// OSC 52 is the fallback when no native clipboard tool is available.
			_ = clipboard.WriteAll(text)
			return nil
		},
	)
}
