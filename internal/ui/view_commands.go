package ui

import (
	"reflect"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

// bindViewDialog captures the originating view before asynchronous execution.
// Other messages keep their existing routing and stale-result checks.
func bindViewDialog(cmd tea.Cmd, owner dialogOwner) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if open, ok := msg.(dialogs.OpenDialogMsg); ok {
			open.Owner = owner
			return open
		}

		// BatchMsg and the private Bubble Tea sequenceMsg are named []tea.Cmd
		// slices. Preserve their concrete types so the runtime retains its
		// concurrency/ordering semantics while every descendant keeps the owner.
		value := reflect.ValueOf(msg)
		if value.IsValid() && value.Type().ConvertibleTo(reflect.TypeFor[[]tea.Cmd]()) {
			children := value.Convert(reflect.TypeFor[[]tea.Cmd]()).Interface().([]tea.Cmd)
			bound := make([]tea.Cmd, len(children))
			for i, child := range children {
				bound[i] = bindViewDialog(child, owner)
			}
			return reflect.ValueOf(bound).Convert(value.Type()).Interface()
		}
		return msg
	}
}
