package jobsbox

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds the styles needed by the jobs box
type Styles struct {
	Title  lipgloss.Style
	Border lipgloss.Style
}

// Render renders a bordered box with title on left and meta on right.
// Content (typically a table) is displayed inside with padding.
func Render(styles Styles, title, meta, content string, width, height int) string {
	if height < 5 {
		height = 5
	}

	border := lipgloss.RoundedBorder()

	// Build styled title
	titleLeft := " " + styles.Title.Render(title) + " "

	// Meta is pre-styled by caller
	titleRight := " " + meta + " "

	// Calculate dimensions
	innerWidth := width - 2

	// Top border with title on left, meta on right
	leftWidth := lipgloss.Width(titleLeft)
	rightWidth := lipgloss.Width(titleRight)
	middlePad := innerWidth - leftWidth - rightWidth - 2
	if middlePad < 0 {
		middlePad = 0
	}

	hBar := styles.Border.Render(string(border.Top))
	topBorder := styles.Border.Render(string(border.TopLeft)) +
		hBar +
		titleLeft +
		strings.Repeat(hBar, middlePad) +
		titleRight +
		hBar +
		styles.Border.Render(string(border.TopRight))

	// Side borders
	vBar := styles.Border.Render(string(border.Left))
	vBarRight := styles.Border.Render(string(border.Right))

	// Split content into lines
	lines := strings.Split(content, "\n")

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
	bottomBorder := styles.Border.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		styles.Border.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}
