package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Retries shows failed jobs pending retry
type Retries struct {
	width  int
	height int
	styles Styles
}

// NewRetries creates a new Retries view
func NewRetries() *Retries {
	return &Retries{}
}

// Init implements View
func (r *Retries) Init() tea.Cmd {
	return nil
}

// Update implements View
func (r *Retries) Update(msg tea.Msg) (View, tea.Cmd) {
	return r, nil
}

// View implements View
func (r *Retries) View() string {
	style := lipgloss.NewStyle().
		Width(r.width).
		Height(r.height).
		Padding(0, 1)

	content := r.styles.Text.Render("Failed jobs pending retry will appear here.") + "\n\n"
	content += r.styles.Muted.Render("Press 1-6 to switch views, t to toggle theme")

	return style.Render(content)
}

// Name implements View
func (r *Retries) Name() string {
	return "Retries"
}

// ShortHelp implements View
func (r *Retries) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (r *Retries) SetSize(width, height int) View {
	r.width = width
	r.height = height
	return r
}

// SetStyles implements View
func (r *Retries) SetStyles(styles Styles) View {
	r.styles = styles
	return r
}
