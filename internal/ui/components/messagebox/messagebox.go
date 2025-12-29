// Package messagebox renders titled message boxes.
package messagebox

import (
	"strings"

	"charm.land/lipgloss/v2"
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
	height := m.height
	if height < 5 {
		height = 5
	}

	border := lipgloss.RoundedBorder()

	// Top border with title
	titleText := " " + m.title + " "
	styledTitle := m.styles.Title.Render(titleText)
	titleWidth := lipgloss.Width(styledTitle)
	innerWidth := m.width - 2
	leftPad := 1
	rightPad := innerWidth - titleWidth - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	hBar := m.styles.Border.Render(string(border.Top))
	topBorder := m.styles.Border.Render(string(border.TopLeft)) +
		strings.Repeat(hBar, leftPad) +
		styledTitle +
		strings.Repeat(hBar, rightPad) +
		m.styles.Border.Render(string(border.TopRight))

	// Content with side borders - centered message
	vBar := m.styles.Border.Render(string(border.Left))
	vBarRight := m.styles.Border.Render(string(border.Right))

	contentHeight := height - 2 // minus top and bottom borders
	var middleLines []string

	// Center the message vertically
	msgText := m.styles.Muted.Render(m.message)
	msgWidth := lipgloss.Width(msgText)
	centerRow := contentHeight / 2

	for i := 0; i < contentHeight; i++ {
		var line string
		if i == centerRow {
			// Center horizontally
			leftPadding := (innerWidth - msgWidth) / 2
			if leftPadding < 0 {
				leftPadding = 0
			}
			rightPadding := innerWidth - leftPadding - msgWidth
			if rightPadding < 0 {
				rightPadding = 0
			}
			line = strings.Repeat(" ", leftPadding) + msgText + strings.Repeat(" ", rightPadding)
		} else {
			line = strings.Repeat(" ", innerWidth)
		}
		middleLines = append(middleLines, vBar+line+vBarRight)
	}

	// Bottom border
	bottomBorder := m.styles.Border.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		m.styles.Border.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
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
