// Package navbar renders the bottom navigation bar.
package navbar

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ViewInfo holds information about a view for display in the navbar.
type ViewInfo struct {
	Name string
}

// Styles holds the styles needed by the navbar.
type Styles struct {
	Bar  lipgloss.Style
	Key  lipgloss.Style
	Item lipgloss.Style
	Quit lipgloss.Style
}

// DefaultStyles returns default styles for the navbar.
func DefaultStyles() Styles {
	return Styles{
		Bar:  lipgloss.NewStyle().Padding(0, 1),
		Key:  lipgloss.NewStyle().Padding(0, 1),
		Item: lipgloss.NewStyle().PaddingRight(1),
		Quit: lipgloss.NewStyle().PaddingRight(1),
	}
}

// Model defines state for the navbar component.
type Model struct {
	styles Styles
	views  []ViewInfo
	width  int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new navbar model.
func New(opts ...Option) Model {
	m := Model{
		styles: DefaultStyles(),
	}

	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) {
		m.styles = s
	}
}

// WithViews sets the views to display.
func WithViews(views []ViewInfo) Option {
	return func(m *Model) {
		m.views = views
	}
}

// WithWidth sets the width.
func WithWidth(w int) Option {
	return func(m *Model) {
		m.width = w
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetViews sets the views to display.
func (m *Model) SetViews(views []ViewInfo) {
	m.views = views
}

// SetWidth sets the width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the height of the navbar (always 1).
func (m Model) Height() int {
	return 1
}

// Init returns an initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(_ tea.Msg) (Model, tea.Cmd) {
	return m, nil
}

// View renders the navbar.
func (m Model) View() string {
	barStyle := m.styles.Bar.Width(m.width)

	items := ""
	for i, v := range m.views {
		key := m.styles.Key.Render(fmt.Sprintf("%d", i+1))
		name := m.styles.Item.Render(v.Name)
		items += key + name
	}

	// Add quit hint
	items += m.styles.Key.Render("q") + m.styles.Quit.Render("quit")

	return barStyle.Render(items)
}
