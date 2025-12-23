package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Scheduled shows jobs scheduled for future execution
type Scheduled struct {
	width  int
	height int
	styles Styles
}

// NewScheduled creates a new Scheduled view
func NewScheduled() *Scheduled {
	return &Scheduled{}
}

// Init implements View
func (s *Scheduled) Init() tea.Cmd {
	return nil
}

// Update implements View
func (s *Scheduled) Update(msg tea.Msg) (View, tea.Cmd) {
	return s, nil
}

// View implements View
func (s *Scheduled) View() string {
	style := lipgloss.NewStyle().
		Width(s.width).
		Height(s.height).
		Padding(0, 1)

	content := s.styles.Text.Render("Jobs scheduled for future execution will appear here.") + "\n\n"
	content += s.styles.Muted.Render("Press 1-6 to switch views, t to toggle theme")

	return style.Render(content)
}

// Name implements View
func (s *Scheduled) Name() string {
	return "Scheduled"
}

// ShortHelp implements View
func (s *Scheduled) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (s *Scheduled) SetSize(width, height int) View {
	s.width = width
	s.height = height
	return s
}

// SetStyles implements View
func (s *Scheduled) SetStyles(styles Styles) View {
	s.styles = styles
	return s
}
