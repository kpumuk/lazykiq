// Package jobsbox renders titled jobs summary boxes.
package jobsbox

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Styles holds the styles needed by the jobs box component.
type Styles struct {
	Title  lipgloss.Style
	Border lipgloss.Style
}

// DefaultStyles returns default styles for the jobs box.
func DefaultStyles() Styles {
	return Styles{
		Title:  lipgloss.NewStyle().Bold(true),
		Border: lipgloss.NewStyle(),
	}
}

// Model defines state for the jobs box component.
type Model struct {
	styles  Styles
	title   string
	meta    string
	content string
	width   int
	height  int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new jobs box model.
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

// WithMeta sets the meta information.
func WithMeta(meta string) Option {
	return func(m *Model) {
		m.meta = meta
	}
}

// WithContent sets the content.
func WithContent(content string) Option {
	return func(m *Model) {
		m.content = content
	}
}

// WithSize sets width and height.
func WithSize(width, height int) Option {
	return func(m *Model) {
		m.width = width
		m.height = height
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

// SetMeta sets the meta information displayed on the right side of the title bar.
func (m *Model) SetMeta(meta string) {
	m.meta = meta
}

// SetContent sets the content displayed inside the box.
func (m *Model) SetContent(content string) {
	m.content = content
}

// SetSize sets the width and height of the box.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the current height.
func (m Model) Height() int {
	return m.height
}

// View renders the jobs box component.
func (m Model) View() string {
	height := m.height
	if height < 5 {
		height = 5
	}

	border := lipgloss.RoundedBorder()

	// Build styled title
	titleLeft := m.styles.Title.Render(m.title)

	// Meta is pre-styled by caller
	titleRight := m.styles.Border.Render("╖") + m.meta + m.styles.Border.Render("╓")

	// Calculate dimensions
	innerWidth := m.width - 2

	// Top border with title on left, meta on right
	leftWidth := lipgloss.Width(titleLeft)
	rightWidth := lipgloss.Width(titleRight)
	middlePad := innerWidth - leftWidth - rightWidth - 2
	if middlePad < 0 {
		middlePad = 0
	}

	hBar := m.styles.Border.Render(string(border.Top))
	topBorder := m.styles.Border.Render(string(border.TopLeft)) +
		hBar +
		titleLeft +
		strings.Repeat(hBar, middlePad) +
		titleRight +
		hBar +
		m.styles.Border.Render(string(border.TopRight))

	// Side borders
	vBar := m.styles.Border.Render(string(border.Left))
	vBarRight := m.styles.Border.Render(string(border.Right))

	// Split content into lines
	lines := strings.Split(m.content, "\n")

	var middleLines []string
	contentHeight := height - 2 // minus top and bottom borders

	for i := 0; i < contentHeight; i++ {
		var line string
		if i < len(lines) {
			line = lines[i]
		}

		// Add padding
		line = " " + line + " "
		lineWidth := lipgloss.Width(line)
		padding := innerWidth - lineWidth
		if padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		middleLines = append(middleLines, vBar+line+vBarRight)
	}

	// Bottom border
	bottomBorder := m.styles.Border.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		m.styles.Border.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}
