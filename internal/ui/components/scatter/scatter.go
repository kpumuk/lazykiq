// Package scatter provides a reusable scatter plot chart component.
package scatter

import (
	"math"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"

	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/ui/charts"
)

// Styles holds the visual styles for the scatter plot.
type Styles struct {
	Axis  lipgloss.Style // Style for chart axes
	Label lipgloss.Style // Style for axis labels
	Point lipgloss.Style // Style for scatter points
	Muted lipgloss.Style // Style for secondary text
}

// DefaultStyles returns sensible default styles.
func DefaultStyles() Styles {
	return Styles{
		Axis:  lipgloss.NewStyle(),
		Label: lipgloss.NewStyle(),
		Point: lipgloss.NewStyle(),
		Muted: lipgloss.NewStyle(),
	}
}

// Model holds the scatter plot state.
type Model struct {
	styles       Styles
	width        int
	height       int
	points       []charts.ScatterPoint
	timeBuckets  []time.Time
	yLabels      []string
	maxCount     int64
	maxBucket    int
	emptyMessage string
}

// Option is a functional option for configuring the scatter plot.
type Option func(*Model)

// New creates a new scatter plot model with functional options.
func New(opts ...Option) Model {
	m := Model{
		styles: DefaultStyles(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// WithStyles sets custom styles for the scatter plot.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}

// WithSize sets the dimensions of the scatter plot.
func WithSize(w, h int) Option {
	return func(m *Model) { m.width, m.height = w, h }
}

// WithData sets the data to display in the scatter plot.
func WithData(points []charts.ScatterPoint, timeBuckets []time.Time, yLabels []string, maxCount int64, maxBucket int) Option {
	return func(m *Model) {
		m.points = points
		m.timeBuckets = timeBuckets
		m.yLabels = yLabels
		m.maxCount = maxCount
		m.maxBucket = maxBucket
	}
}

// WithEmptyMessage sets the message to display when there's no data.
func WithEmptyMessage(msg string) Option {
	return func(m *Model) { m.emptyMessage = msg }
}

// SetStyles updates the scatter plot styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize updates the scatter plot dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetData updates the scatter plot data.
func (m *Model) SetData(points []charts.ScatterPoint, timeBuckets []time.Time, yLabels []string, maxCount int64, maxBucket int) {
	m.points = points
	m.timeBuckets = timeBuckets
	m.yLabels = yLabels
	m.maxCount = maxCount
	m.maxBucket = maxBucket
}

// SetEmptyMessage updates the empty state message.
func (m *Model) SetEmptyMessage(msg string) {
	m.emptyMessage = msg
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the current height.
func (m Model) Height() int {
	return m.height
}

// View renders the scatter plot to a string.
func (m Model) View() string {
	if m.width < 2 || m.height < 2 {
		return ""
	}
	empty := func() string {
		return charts.RenderCentered(m.width, m.height, m.emptyMessage)
	}
	if len(m.points) == 0 || len(m.timeBuckets) == 0 {
		return empty()
	}
	if m.maxCount == 0 {
		return empty()
	}

	labels := m.yLabels
	if m.maxBucket >= 0 && m.maxBucket < len(labels)-1 {
		labels = labels[:m.maxBucket+1]
	}

	minX := 0.0
	maxX := float64(max(len(m.timeBuckets)-1, 1))
	minY := 0.0
	maxY := float64(max(m.maxBucket, 1))
	yLabelWidth := charts.MaxLabelWidthFromSlice(labels)

	showLabels := m.height >= 3
	chartHeight := max(m.height, 1)
	yStep := max(chartHeight/6, 1)
	xStep := 0
	if showLabels && chartHeight >= 2 {
		xStep = 1
	}

	lc := linechart.New(
		m.width, chartHeight,
		minX, maxX,
		minY, maxY,
		linechart.WithXYSteps(xStep, yStep),
		linechart.WithStyles(m.styles.Axis, m.styles.Label, m.styles.Point),
		linechart.WithXLabelFormatter(func(_ int, _ float64) string { return "" }),
		linechart.WithYLabelFormatter(func(_ int, v float64) string {
			idx := mathutil.Clamp(int(math.Round(v)), 0, len(labels)-1)
			return labels[idx]
		}),
	)
	lc.DrawXYAxisAndLabel()

	// Sort points by count for proper rendering order (smaller points first)
	sortedPoints := make([]charts.ScatterPoint, len(m.points))
	copy(sortedPoints, m.points)
	sort.Slice(sortedPoints, func(i, j int) bool {
		return sortedPoints[i].Count < sortedPoints[j].Count
	})

	for _, point := range sortedPoints {
		lc.DrawRune(canvas.Float64Point{X: point.X, Y: point.Y}, scatterRune(point.Count, m.maxCount))
	}

	view := lc.View()
	chartLines := strings.Split(view, "\n")

	if showLabels {
		graphWidth := max(m.width-(yLabelWidth+1), 1)
		timeLabels := charts.BuildTimeBucketLabels(m.timeBuckets)
		labelLine := charts.BuildBucketLabelLine(graphWidth+1, timeLabels)
		labelLine = strings.Repeat(" ", yLabelWidth) + m.styles.Muted.Render(labelLine)
		if xStep > 0 && len(chartLines) > 0 {
			chartLines[len(chartLines)-1] = labelLine
		} else {
			chartLines = append(chartLines, labelLine)
		}
	}
	return strings.Join(chartLines, "\n")
}

// scatterRune returns a rune representing point density on a scatter plot.
// Uses logarithmic scaling to show magnitude differences.
func scatterRune(count, maxCount int64) rune {
	levels := []rune{'·', '◦', '•', '◯', '◉', '●'}
	if maxCount <= 0 {
		return levels[0]
	}
	if count < 0 {
		count = 0
	}
	ratio := math.Log1p(float64(count)) / math.Log1p(float64(maxCount))
	idx := mathutil.Clamp(int(math.Round(ratio*float64(len(levels)-1))), 0, len(levels)-1)
	return levels[idx]
}
