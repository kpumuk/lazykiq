package charts

import (
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// AxisMap creates a mapping from source indices to target indices.
// Used to remap data series to fit available space.
func AxisMap(total, target int) []int {
	if total <= 0 || target <= 0 {
		return nil
	}
	mapping := make([]int, total)
	if total == 1 {
		for i := range mapping {
			mapping[i] = 0
		}
		return mapping
	}
	maxIdx := float64(target - 1)
	denom := float64(total - 1)
	for i := range total {
		mapping[i] = int(math.Round(float64(i) * maxIdx / denom))
	}
	return mapping
}

// RemapSeries aggregates values according to a target width.
// Values are bucketed together when the target is smaller than the source.
func RemapSeries(values []int64, target int) []int64 {
	if len(values) == 0 || target <= 0 {
		return nil
	}
	mapping := AxisMap(len(values), target)
	out := make([]int64, target)
	for i, v := range values {
		out[mapping[i]] += v
	}
	return out
}

// BuildValueYAxisLabels creates Y-axis labels for numeric values.
// Returns a map of row index to label string.
func BuildValueYAxisLabels(maxVal int64, height int) map[int]string {
	labels := make(map[int]string)
	if height <= 0 {
		return labels
	}
	if maxVal < 0 {
		maxVal = 0
	}
	tickCount := min(4, height)
	if tickCount < 2 {
		labels[height-1] = format.ShortNumber(0)
		return labels
	}
	for i := range tickCount {
		row := int(math.Round(float64(i) * float64(height-1) / float64(tickCount-1)))
		val := maxVal * int64(tickCount-1-i) / int64(tickCount-1)
		labels[row] = format.ShortNumber(val)
	}
	return labels
}

// MaxLabelWidth returns the maximum display width of labels in a map.
func MaxLabelWidth(labels map[int]string) int {
	maxWidth := 0
	for _, label := range labels {
		labelWidth := lipgloss.Width(label)
		if labelWidth > maxWidth {
			maxWidth = labelWidth
		}
	}
	return maxWidth
}

// MaxLabelWidthFromSlice returns the maximum display width of labels in a slice.
func MaxLabelWidthFromSlice(labels []string) int {
	maxWidth := 0
	for _, label := range labels {
		labelWidth := lipgloss.Width(label)
		if labelWidth > maxWidth {
			maxWidth = labelWidth
		}
	}
	return maxWidth
}

// ApplyYAxisLabels prepends Y-axis labels to chart lines.
// Each line gets a label if present in the labels map, or spacing otherwise.
func ApplyYAxisLabels(lines []string, labels map[int]string, width int, style lipgloss.Style) []string {
	if width <= 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		raw := labels[i]
		padWidth := max(width-lipgloss.Width(raw), 0)
		prefix := strings.Repeat(" ", padWidth)
		if raw != "" {
			raw = style.Render(raw)
		}
		out = append(out, prefix+raw+" "+line)
	}
	return out
}

// BuildBucketLabelLine creates a horizontal label line with evenly spaced labels.
// Labels are centered at their positions and non-overlapping.
func BuildBucketLabelLine(width int, labels []string) string {
	if width <= 0 {
		return ""
	}
	line := make([]rune, width)
	for i := range line {
		line[i] = ' '
	}
	if len(labels) == 0 || width < 2 {
		return string(line)
	}

	plotWidth := max(width-1, 1)
	positions := AxisMap(len(labels), plotWidth)
	lastEnd := -1
	for i, label := range labels {
		if label == "" {
			continue
		}
		pos := positions[i] + 1
		labelRunes := []rune(label)
		start := pos - len(labelRunes)/2
		start = max(start, 0)
		end := start + len(labelRunes)
		end = min(end, width)
		if start <= lastEnd+1 {
			continue
		}
		length := end - start
		if length <= 0 {
			continue
		}
		if length < len(labelRunes) {
			labelRunes = labelRunes[:length]
		}
		for j, r := range labelRunes {
			line[start+j] = r
		}
		lastEnd = start + len(labelRunes) - 1
	}
	return string(line)
}

// BuildTimeBucketLabels creates formatted time labels for time-series data.
// Adjusts format based on whether data spans multiple days.
func BuildTimeBucketLabels(buckets []time.Time) []string {
	if len(buckets) == 0 {
		return nil
	}

	start := buckets[0].UTC()
	end := buckets[len(buckets)-1].UTC()
	format := "15:04"
	if start.Format("2006-01-02") != end.Format("2006-01-02") {
		format = "Jan 2 15:04"
	}

	labels := make([]string, len(buckets))
	for i, bucket := range buckets {
		if bucket.IsZero() {
			continue
		}
		labels[i] = bucket.UTC().Format(format)
	}

	return labels
}

// RenderCentered centers content within a given width and height.
// Handles multi-line content by centering vertically and horizontally.
func RenderCentered(width, height int, value string) string {
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

	// Handle multi-line content
	contentLines := strings.Split(value, "\n")
	contentHeight := len(contentLines)
	startLine := max((height-contentHeight)/2, 0)

	maxWidthStyle := lipgloss.NewStyle()
	for i, contentLine := range contentLines {
		lineIdx := startLine + i
		if lineIdx >= height {
			break
		}
		trimmed := maxWidthStyle.MaxWidth(width).Render(contentLine)
		pad := max((width-lipgloss.Width(trimmed))/2, 0)
		lines[lineIdx] = strings.Repeat(" ", pad) + trimmed
	}

	return strings.Join(lines, "\n")
}
