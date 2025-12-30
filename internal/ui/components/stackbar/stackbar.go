// Package stackbar renders the view stack breadcrumb bar.
package stackbar

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Styles holds the styles needed by the stack bar.
type Styles struct {
	Bar   lipgloss.Style
	Item  lipgloss.Style
	Arrow lipgloss.Style
}

// DefaultStyles returns default styles for the stack bar.
func DefaultStyles() Styles {
	return Styles{
		Bar:   lipgloss.NewStyle().Padding(0, 1),
		Item:  lipgloss.NewStyle().Padding(0, 1),
		Arrow: lipgloss.NewStyle(),
	}
}

// Model defines state for the stack bar component.
type Model struct {
	styles Styles
	stack  []string
	width  int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new stack bar model.
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

// WithStack sets the stack labels.
func WithStack(stack []string) Option {
	return func(m *Model) {
		m.stack = stack
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

// SetStack sets the stack labels.
func (m *Model) SetStack(stack []string) {
	m.stack = stack
}

// SetWidth sets the width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the height of the stack bar (always 1).
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

// View renders the stack bar.
func (m Model) View() string {
	barStyle := m.styles.Bar
	if m.width > 0 {
		barStyle = barStyle.Width(m.width)
	}

	var items strings.Builder
	for i, label := range m.stack {
		if i > 0 {
			items.WriteString(" ")
		}
		items.WriteString(formatLabel(m.styles, label))
	}

	return barStyle.Render(items.String())
}

func formatLabel(styles Styles, label string) string {
	return styles.Item.Render(label) + styles.Arrow.Render("")
}
