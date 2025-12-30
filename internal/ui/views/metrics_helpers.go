package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func clampInt(value, maxValue int) int {
	return min(max(value, 0), maxValue)
}

func formatMetricsRange(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return ""
	}

	start = start.UTC()
	end = end.UTC()
	if start.Format("2006-01-02") == end.Format("2006-01-02") {
		return fmt.Sprintf("%s-%s UTC", start.Format("15:04"), end.Format("15:04"))
	}

	return fmt.Sprintf("%s-%s UTC", start.Format("Jan 2 15:04"), end.Format("Jan 2 15:04"))
}

func renderCentered(width, height int, value string) string {
	if height < 1 {
		return ""
	}
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	if width <= 0 {
		return strings.Join(lines, "\n")
	}
	trimmed := lipgloss.NewStyle().MaxWidth(width).Render(value)
	pad := max((width-lipgloss.Width(trimmed))/2, 0)
	lines[height/2] = strings.Repeat(" ", pad) + trimmed
	return strings.Join(lines, "\n")
}
