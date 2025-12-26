package errorpopup

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds the styles needed by the error popup.
type Styles struct {
	Title   lipgloss.Style
	Message lipgloss.Style
	Border  lipgloss.Style
}

// DefaultStyles returns default styles for the error popup.
func DefaultStyles() Styles {
	errorColor := lipgloss.Color("#FF0000")
	return Styles{
		Title:   lipgloss.NewStyle().Foreground(errorColor).Bold(true),
		Message: lipgloss.NewStyle().Faint(true),
		Border:  lipgloss.NewStyle().Foreground(errorColor),
	}
}

// Model defines state for the error popup component.
type Model struct {
	styles     Styles
	message    string
	background string
	width      int
	height     int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new error popup model.
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

// WithSize sets the width and height.
func WithSize(w, h int) Option {
	return func(m *Model) {
		m.width = w
		m.height = h
	}
}

// WithMessage sets the error message.
func WithMessage(msg string) Option {
	return func(m *Model) {
		m.message = msg
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize sets the width and height.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetMessage sets the error message to display.
func (m *Model) SetMessage(msg string) {
	m.message = msg
}

// SetBackground sets the background content to overlay on.
func (m *Model) SetBackground(content string) {
	m.background = content
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the current height.
func (m Model) Height() int {
	return m.height
}

// Message returns the current error message.
func (m Model) Message() string {
	return m.message
}

// HasError returns true if there is an error message to display.
func (m Model) HasError() bool {
	return m.message != ""
}

// View renders the error popup overlaid on the background content.
func (m Model) View() string {
	if m.message == "" {
		return m.background
	}

	// Error message content
	errorMessage := m.styles.Message.Render(m.message) + "\n\n" +
		m.styles.Message.Render("Retrying every 5 seconds...")

	// Create error panel with title on border
	errorPanel := m.renderErrorBox("Connection Error", errorMessage, 60)

	// Split content into lines
	contentLines := strings.Split(m.background, "\n")
	errorLines := strings.Split(errorPanel, "\n")

	// Calculate starting position to center the error panel
	errorHeight := len(errorLines)
	startRow := (m.height - errorHeight) / 2

	// Overlay error panel lines onto content lines
	for i, errorLine := range errorLines {
		row := startRow + i
		if row >= 0 && row < len(contentLines) {
			// Use lipgloss.PlaceHorizontal to center the error line within the content width
			centeredLine := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, errorLine)
			contentLines[row] = centeredLine
		}
	}

	return strings.Join(contentLines, "\n")
}

// renderErrorBox renders content in a box with title on the top border.
func (m Model) renderErrorBox(title, content string, width int) string {
	border := lipgloss.RoundedBorder()

	// Build top border with title
	titleText := " " + title + " "
	styledTitle := m.styles.Title.Render(titleText)
	titleWidth := lipgloss.Width(styledTitle)

	topLeft := m.styles.Border.Render(border.TopLeft)
	topRight := m.styles.Border.Render(border.TopRight)
	hBar := m.styles.Border.Render(border.Top)

	// Calculate remaining width for horizontal bars
	remainingWidth := width - 2 - titleWidth // -2 for corners
	leftPad := 1
	rightPad := remainingWidth - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	topBorder := topLeft + strings.Repeat(hBar, leftPad) + styledTitle + strings.Repeat(hBar, rightPad) + topRight

	// Build content area with side borders (no background)
	vBar := m.styles.Border.Render(border.Left)
	vBarRight := m.styles.Border.Render(border.Right)

	innerWidth := width - 2
	contentStyle := lipgloss.NewStyle().
		Width(innerWidth).
		Padding(0, 1)

	renderedContent := contentStyle.Render(content)
	contentLines := strings.Split(renderedContent, "\n")

	var middleLines []string
	for _, line := range contentLines {
		// Pad line to inner width with spaces (transparent background)
		lineWidth := lipgloss.Width(line)
		if lineWidth < innerWidth {
			line += strings.Repeat(" ", innerWidth-lineWidth)
		}
		middleLines = append(middleLines, vBar+line+vBarRight)
	}

	// Build bottom border
	bottomLeft := m.styles.Border.Render(border.BottomLeft)
	bottomRight := m.styles.Border.Render(border.BottomRight)
	bottomBorder := bottomLeft + strings.Repeat(hBar, width-2) + bottomRight

	// Combine all parts
	result := topBorder + "\n"
	result += strings.Join(middleLines, "\n") + "\n"
	result += bottomBorder

	return result
}
