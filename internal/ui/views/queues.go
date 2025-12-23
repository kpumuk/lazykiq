package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Queues shows the list of Sidekiq queues
type Queues struct {
	width  int
	height int
	styles Styles
}

// NewQueues creates a new Queues view
func NewQueues() *Queues {
	return &Queues{}
}

// Init implements View
func (q *Queues) Init() tea.Cmd {
	return nil
}

// Update implements View
func (q *Queues) Update(msg tea.Msg) (View, tea.Cmd) {
	return q, nil
}

// View implements View
func (q *Queues) View() string {
	style := lipgloss.NewStyle().
		Width(q.width).
		Height(q.height).
		Padding(0, 1)

	content := q.styles.Text.Render("List of queues and their job counts will appear here.") + "\n\n"
	content += q.styles.Muted.Render("Press 1-6 to switch views, t to toggle theme")

	return style.Render(content)
}

// Name implements View
func (q *Queues) Name() string {
	return "Queues"
}

// ShortHelp implements View
func (q *Queues) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (q *Queues) SetSize(width, height int) View {
	q.width = width
	q.height = height
	return q
}

// SetStyles implements View
func (q *Queues) SetStyles(styles Styles) View {
	q.styles = styles
	return q
}
