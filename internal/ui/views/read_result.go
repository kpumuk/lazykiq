package views

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// ReadResultMsg binds a replaceable read completion to its view and generation.
// Each Controller.Start creates a unique context; replacement or hiding cancels
// that generation. Check it at delivery even if the command already completed.
// Mutation results deliberately do not use this envelope.
type ReadResultMsg struct {
	Owner   View
	context context.Context
	Data    tea.Msg
	Err     error
}

// Current reports whether this read's generation is still active.
func (r ReadResultMsg) Current() bool { return r.context != nil && r.context.Err() == nil }

func readCmd(ctx context.Context, owner View, cmd tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		data := cmd()
		if data == nil {
			return nil
		}
		result := ReadResultMsg{Owner: owner, context: ctx, Data: data}
		if failure, ok := data.(ConnectionErrorMsg); ok {
			result.Data = nil
			result.Err = failure.Err
		}
		return result
	}
}
