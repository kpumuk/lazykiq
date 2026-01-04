package dialogs

import "charm.land/bubbles/v2/key"

// KeyMap defines keyboard bindings for dialog management.
type KeyMap struct {
	Close key.Binding
}

// DefaultKeyMap returns the default dialog key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Close: key.NewBinding(
			key.WithKeys("esc", "alt+esc"),
		),
	}
}

// KeyBindings returns dialog key bindings.
func (k KeyMap) KeyBindings() []key.Binding {
	return []key.Binding{
		k.Close,
	}
}
