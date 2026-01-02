package views

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
)

var maxWidthStyle = lipgloss.NewStyle()

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
