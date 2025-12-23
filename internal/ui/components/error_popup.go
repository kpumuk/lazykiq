package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
)

// ErrorPopup displays a centered error dialog overlay
type ErrorPopup struct {
	width   int
	height  int
	styles  *theme.Styles
	message string
}

// NewErrorPopup creates a new error popup component
func NewErrorPopup(styles *theme.Styles) ErrorPopup {
	return ErrorPopup{
		styles: styles,
	}
}

// SetSize updates the dimensions of the popup area
func (e ErrorPopup) SetSize(width, height int) ErrorPopup {
	e.width = width
	e.height = height
	return e
}

// SetStyles updates the styles
func (e ErrorPopup) SetStyles(styles *theme.Styles) ErrorPopup {
	e.styles = styles
	return e
}

// SetMessage sets the error message to display
func (e ErrorPopup) SetMessage(msg string) ErrorPopup {
	e.message = msg
	return e
}

// Render overlays the error popup on top of the provided content
func (e ErrorPopup) Render(content string) string {
	if e.message == "" {
		return content
	}

	// Error message content
	errorMessage := e.styles.ViewMuted.Render(e.message) + "\n\n" +
		e.styles.ViewMuted.Render("Retrying every 5 seconds...")

	// Create error panel with title on border
	errorPanel := e.renderErrorBox("Connection Error", errorMessage, 60)

	// Split content into lines
	contentLines := strings.Split(content, "\n")
	errorLines := strings.Split(errorPanel, "\n")

	// Calculate starting position to center the error panel
	errorHeight := len(errorLines)
	startRow := (e.height - errorHeight) / 2

	// Overlay error panel lines onto content lines
	for i, errorLine := range errorLines {
		row := startRow + i
		if row >= 0 && row < len(contentLines) {
			// Use lipgloss.PlaceHorizontal to center the error line within the content width
			centeredLine := lipgloss.PlaceHorizontal(e.width, lipgloss.Center, errorLine)
			contentLines[row] = centeredLine
		}
	}

	return strings.Join(contentLines, "\n")
}

// renderErrorBox renders content in a box with title on the top border (red theme)
func (e ErrorPopup) renderErrorBox(title, content string, width int) string {
	border := lipgloss.RoundedBorder()
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		Bold(true)

	// Build top border with title
	titleText := " " + title + " "
	styledTitle := titleStyle.Render(titleText)
	titleWidth := lipgloss.Width(styledTitle)

	topLeft := borderStyle.Render(border.TopLeft)
	topRight := borderStyle.Render(border.TopRight)
	hBar := borderStyle.Render(border.Top)

	// Calculate remaining width for horizontal bars
	remainingWidth := width - 2 - titleWidth // -2 for corners
	leftPad := 1
	rightPad := remainingWidth - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	topBorder := topLeft + strings.Repeat(hBar, leftPad) + styledTitle + strings.Repeat(hBar, rightPad) + topRight

	// Build content area with side borders (no background)
	vBar := borderStyle.Render(border.Left)
	vBarRight := borderStyle.Render(border.Right)

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
	bottomLeft := borderStyle.Render(border.BottomLeft)
	bottomRight := borderStyle.Render(border.BottomRight)
	bottomBorder := bottomLeft + strings.Repeat(hBar, width-2) + bottomRight

	// Combine all parts
	result := topBorder + "\n"
	result += strings.Join(middleLines, "\n") + "\n"
	result += bottomBorder

	return result
}
