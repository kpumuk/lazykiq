package messagebox

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds the styles needed by the message box
type Styles struct {
	Title  lipgloss.Style
	Muted  lipgloss.Style
	Border lipgloss.Style
}

// Render renders a bordered box with a centered message inside.
// The message is centered both horizontally and vertically within the box.
func Render(styles Styles, title, message string, width, height int) string {
	if height < 5 {
		height = 5
	}

	border := lipgloss.RoundedBorder()

	// Top border with title
	titleText := " " + title + " "
	styledTitle := styles.Title.Render(titleText)
	titleWidth := lipgloss.Width(styledTitle)
	innerWidth := width - 2
	leftPad := 1
	rightPad := innerWidth - titleWidth - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	hBar := styles.Border.Render(string(border.Top))
	topBorder := styles.Border.Render(string(border.TopLeft)) +
		strings.Repeat(hBar, leftPad) +
		styledTitle +
		strings.Repeat(hBar, rightPad) +
		styles.Border.Render(string(border.TopRight))

	// Content with side borders - centered message
	vBar := styles.Border.Render(string(border.Left))
	vBarRight := styles.Border.Render(string(border.Right))

	contentHeight := height - 2 // minus top and bottom borders
	var middleLines []string

	// Center the message vertically
	msgText := styles.Muted.Render(message)
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
	bottomBorder := styles.Border.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		styles.Border.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}
