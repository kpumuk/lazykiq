package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all global keybindings
type KeyMap struct {
	Quit        key.Binding
	View1       key.Binding
	View2       key.Binding
	View3       key.Binding
	View4       key.Binding
	View5       key.Binding
	View6       key.Binding
	Tab         key.Binding
	ShiftTab    key.Binding
	Help        key.Binding
	ToggleTheme key.Binding
}

// DefaultKeyMap returns the default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		View1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "dashboard"),
		),
		View2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "queues"),
		),
		View3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "busy"),
		),
		View4: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "retries"),
		),
		View5: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "scheduled"),
		),
		View6: key.NewBinding(
			key.WithKeys("6"),
			key.WithHelp("6", "dead"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		ToggleTheme: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "toggle theme"),
		),
	}
}

// ShortHelp returns keybindings to show in the mini help view
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.View1, k.View2, k.View3, k.View4, k.View5, k.View6, k.Quit}
}

// FullHelp returns keybindings for the expanded help view
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.View1, k.View2, k.View3, k.View4, k.View5, k.View6},
		{k.Tab, k.ShiftTab, k.Help, k.Quit},
	}
}
