// Package messagebox renders titled message boxes.
package messagebox

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
)

// Styles holds the styles needed by the message box.
type Styles struct {
	Title  lipgloss.Style
	Muted  lipgloss.Style
	Border lipgloss.Style
}

// DefaultStyles returns default styles for the message box.
func DefaultStyles() Styles {
	return Styles{
		Title:  lipgloss.NewStyle().Bold(true),
		Muted:  lipgloss.NewStyle().Faint(true),
		Border: lipgloss.NewStyle(),
	}
}

// Model defines state for the message box component.
type Model struct {
	styles  Styles
	title   string
	message string
	width   int
	height  int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new message box model.
func New(opts ...Option) Model {
	m := Model{
		styles: DefaultStyles(),
		height: 5,
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

// WithTitle sets the title.
func WithTitle(title string) Option {
	return func(m *Model) {
		m.title = title
	}
}

// WithMessage sets the message.
func WithMessage(msg string) Option {
	return func(m *Model) {
		m.message = msg
	}
}

// WithSize sets the width and height.
func WithSize(w, h int) Option {
	return func(m *Model) {
		m.width = w
		m.height = h
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetTitle sets the title.
func (m *Model) SetTitle(title string) {
	m.title = title
}

// SetMessage sets the message.
func (m *Model) SetMessage(msg string) {
	m.message = msg
}

// SetSize sets the width and height.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the current height.
func (m Model) Height() int {
	return m.height
}

// Title returns the current title.
func (m Model) Title() string {
	return m.title
}

// Message returns the current message.
func (m Model) Message() string {
	return m.message
}

// View renders the message box.
func (m Model) View() string {
	innerWidth := max(m.width-2, 0)
	contentHeight := max(m.height-2, 0)

	// Center the message vertically within contentHeight
	lines := make([]string, contentHeight)
	msgText := m.styles.Muted.Render(m.message)
	msgWidth := lipgloss.Width(msgText)
	centerRow := contentHeight / 2

	for i := range contentHeight {
		if i == centerRow {
			leftPadding := max((innerWidth-msgWidth)/2, 0)
			rightPadding := max(innerWidth-leftPadding-msgWidth, 0)
			lines[i] = strings.Repeat(" ", leftPadding) + msgText + strings.Repeat(" ", rightPadding)
		} else {
			lines[i] = strings.Repeat(" ", innerWidth)
		}
	}

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle(m.title),
		frame.WithTitlePadding(1),
		frame.WithContent(strings.Join(lines, "\n")),
		frame.WithSize(m.width, m.height),
		frame.WithMinHeight(5),
	)

	return box.View()
}

// Render is a convenience function for one-off rendering without creating a Model.
// It exists for backward compatibility and simpler use cases.
func Render(styles Styles, title, message string, width, height int) string {
	m := New(
		WithStyles(styles),
		WithTitle(title),
		WithMessage(message),
		WithSize(width, height),
	)
	return m.View()
}
