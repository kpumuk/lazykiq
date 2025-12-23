package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Dashboard is the main overview view
type Dashboard struct {
	width  int
	height int
	styles Styles
}

// NewDashboard creates a new Dashboard view
func NewDashboard() *Dashboard {
	return &Dashboard{}
}

// Init implements View
func (d *Dashboard) Init() tea.Cmd {
	return nil
}

// Update implements View
func (d *Dashboard) Update(msg tea.Msg) (View, tea.Cmd) {
	return d, nil
}

// View implements View
func (d *Dashboard) View() string {
	style := lipgloss.NewStyle().
		Width(d.width).
		Height(d.height).
		Padding(0, 1)

	content := d.styles.Text.Render("Overview of Sidekiq status will appear here.") + "\n\n"
	content += d.styles.Muted.Render("Press 1-6 to switch views, t to toggle theme")

	return style.Render(content)
}

// Name implements View
func (d *Dashboard) Name() string {
	return "Dashboard"
}

// ShortHelp implements View
func (d *Dashboard) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (d *Dashboard) SetSize(width, height int) View {
	d.width = width
	d.height = height
	return d
}

// SetStyles implements View
func (d *Dashboard) SetStyles(styles Styles) View {
	d.styles = styles
	return d
}
