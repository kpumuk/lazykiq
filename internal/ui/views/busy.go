package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Busy shows active workers/processes
type Busy struct {
	width  int
	height int
	styles Styles
}

// NewBusy creates a new Busy view
func NewBusy() *Busy {
	return &Busy{}
}

// Init implements View
func (b *Busy) Init() tea.Cmd {
	return nil
}

// Update implements View
func (b *Busy) Update(msg tea.Msg) (View, tea.Cmd) {
	return b, nil
}

// View implements View
func (b *Busy) View() string {
	style := lipgloss.NewStyle().
		Width(b.width).
		Height(b.height).
		Padding(0, 1)

	content := b.styles.Text.Render("Active workers and processes will appear here.") + "\n\n"
	content += b.styles.Muted.Render("Press 1-6 to switch views, t to toggle theme")

	return style.Render(content)
}

// Name implements View
func (b *Busy) Name() string {
	return "Busy"
}

// ShortHelp implements View
func (b *Busy) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (b *Busy) SetSize(width, height int) View {
	b.width = width
	b.height = height
	return b
}

// SetStyles implements View
func (b *Busy) SetStyles(styles Styles) View {
	b.styles = styles
	return b
}
