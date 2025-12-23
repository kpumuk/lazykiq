package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Dead shows dead/morgue jobs
type Dead struct {
	width  int
	height int
	styles Styles
}

// NewDead creates a new Dead view
func NewDead() *Dead {
	return &Dead{}
}

// Init implements View
func (d *Dead) Init() tea.Cmd {
	return nil
}

// Update implements View
func (d *Dead) Update(msg tea.Msg) (View, tea.Cmd) {
	return d, nil
}

// View implements View
func (d *Dead) View() string {
	style := lipgloss.NewStyle().
		Width(d.width).
		Height(d.height).
		Padding(0, 1)

	content := d.styles.Text.Render("Dead/morgue jobs will appear here.") + "\n\n"
	content += d.styles.Muted.Render("Press 1-6 to switch views, t to toggle theme")

	return style.Render(content)
}

// Name implements View
func (d *Dead) Name() string {
	return "Dead"
}

// ShortHelp implements View
func (d *Dead) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (d *Dead) SetSize(width, height int) View {
	d.width = width
	d.height = height
	return d
}

// SetStyles implements View
func (d *Dead) SetStyles(styles Styles) View {
	d.styles = styles
	return d
}
