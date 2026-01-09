// Package histogram provides a reusable histogram chart component.
package histogram

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/graph"

	"github.com/kpumuk/lazykiq/internal/ui/charts"
)

// Styles holds the visual styles for the histogram.
type Styles struct {
	Axis  lipgloss.Style // Style for chart axes
	Bar   lipgloss.Style // Style for histogram bars
	Muted lipgloss.Style // Style for labels and secondary text
}

// DefaultStyles returns sensible default styles.
func DefaultStyles() Styles {
	return Styles{
		Axis:  lipgloss.NewStyle(),
		Bar:   lipgloss.NewStyle(),
		Muted: lipgloss.NewStyle(),
	}
}

// Model holds the histogram chart state.
type Model struct {
	styles       Styles
	width        int
	height       int
	totals       []int64
	labels       []string
	emptyMessage string
}

// Option is a functional option for configuring the histogram.
type Option func(*Model)

// New creates a new histogram model with functional options.
func New(opts ...Option) Model {
	m := Model{
		styles: DefaultStyles(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// WithStyles sets custom styles for the histogram.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}

// WithSize sets the dimensions of the histogram.
func WithSize(w, h int) Option {
	return func(m *Model) { m.width, m.height = w, h }
}

// WithData sets the data to display in the histogram.
func WithData(totals []int64, labels []string) Option {
	return func(m *Model) { m.totals, m.labels = totals, labels }
}

// WithEmptyMessage sets the message to display when there's no data.
func WithEmptyMessage(msg string) Option {
	return func(m *Model) { m.emptyMessage = msg }
}

// SetStyles updates the histogram styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize updates the histogram dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetData updates the histogram data.
func (m *Model) SetData(totals []int64, labels []string) {
	m.totals = totals
	m.labels = labels
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

// View renders the histogram to a string.
func (m Model) View() string {
	if m.width < 2 || m.height < 2 {
		return ""
	}
	empty := func() string {
		return charts.RenderCentered(m.width, m.height, m.emptyMessage)
	}
	if len(m.totals) == 0 {
		return empty()
	}
	maxTotal := slices.Max(m.totals)
	if maxTotal == 0 {
		return empty()
	}

	labels := m.labels
	if len(labels) > len(m.totals) {
		labels = labels[:len(m.totals)]
	}

	showLabels := m.height >= 3
	chartHeight := m.height
	if showLabels {
		chartHeight--
	}
	chartHeight = max(chartHeight, 1)

	yLabels := charts.BuildValueYAxisLabels(maxTotal, chartHeight)
	labelWidth := charts.MaxLabelWidth(yLabels)
	chartWidth := max(m.width-labelWidth-1, 1)
	if chartWidth < 2 {
		return empty()
	}

	plotWidth := max(chartWidth-1, 1)
	series := charts.RemapSeries(m.totals, plotWidth)
	if len(series) == 0 {
		return empty()
	}
	maxVal := slices.Max(series)
	if maxVal == 0 {
		return empty()
	}

	maxHeight := float64(max(chartHeight-1, 1))
	scaled := make([]float64, len(series))
	for i, v := range series {
		scaled[i] = float64(v) * maxHeight / float64(maxVal)
	}

	canvasWidth := len(series) + 1
	c := canvas.New(canvasWidth, chartHeight, canvas.WithViewWidth(canvasWidth), canvas.WithViewHeight(chartHeight))
	origin := canvas.Point{X: 0, Y: chartHeight - 1}
	graph.DrawXYAxis(&c, origin, m.styles.Axis)
	baseline := max(chartHeight-2, 0)
	graph.DrawColumns(&c, canvas.Point{X: 1, Y: baseline}, scaled, m.styles.Bar)

	chartLines := strings.Split(c.View(), "\n")
	chartLines = charts.ApplyYAxisLabels(chartLines, yLabels, labelWidth, m.styles.Muted)

	if showLabels {
		labelLine := m.styles.Muted.Render(charts.BuildBucketLabelLine(canvasWidth, labels))
		chartLines = append(chartLines, strings.Repeat(" ", labelWidth)+" "+labelLine)
	}
	return strings.Join(chartLines, "\n")
}
