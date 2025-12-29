// Package views contains the main UI views.
package views

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Styles holds the view-related styles from the theme.
type Styles struct {
	Text           lipgloss.Style
	Muted          lipgloss.Style
	Title          lipgloss.Style
	MetricLabel    lipgloss.Style
	MetricValue    lipgloss.Style
	TableHeader    lipgloss.Style
	TableSelected  lipgloss.Style
	TableSeparator lipgloss.Style
	BoxPadding     lipgloss.Style
	BorderStyle    lipgloss.Style
	FocusBorder    lipgloss.Style
	NavKey         lipgloss.Style
	ChartSuccess   lipgloss.Style
	ChartFailure   lipgloss.Style
}

// RefreshMsg is broadcast by the app on the 5-second ticker.
// Views should respond by fetching their data.
type RefreshMsg struct{}

// ConnectionErrorMsg indicates a Redis connection error occurred.
// Views emit this when data fetching fails.
type ConnectionErrorMsg struct {
	Err error
}

// View defines the interface that all views must implement.
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
