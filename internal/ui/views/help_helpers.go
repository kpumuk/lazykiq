package views

import (
	"charm.land/bubbles/v2/key"

	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

// helpBinding creates a binding for help rendering.
func helpBinding(keys []string, label, desc string) key.Binding {
	return key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(label, desc),
	)
}

func tableHelpBindings(km table.KeyMap) []key.Binding {
	return []key.Binding{
		km.LineUp,
		km.LineDown,
		km.PageUp,
		km.PageDown,
		km.GotoTop,
		km.GotoBottom,
		km.ScrollLeft,
		km.ScrollRight,
		km.Home,
		km.End,
	}
}
