package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles holds the view-related styles from the theme
type Styles struct {
	Text  lipgloss.Style
	Muted lipgloss.Style
}

// View defines the interface that all views must implement
type View interface {
	// Init returns an initial command for the view
	Init() tea.Cmd

	// Update handles messages and returns the updated view and any commands
	Update(msg tea.Msg) (View, tea.Cmd)

	// View renders the view as a string
	View() string

	// Name returns the display name for this view (shown in navbar)
	Name() string

	// ShortHelp returns keybindings to show in the help view
	ShortHelp() []key.Binding

	// SetSize updates the view dimensions
	SetSize(width, height int) View

	// SetStyles updates the view styles
	SetStyles(styles Styles) View
}
