// Package timeseries provides a reusable multi-series timeseries chart component.
package timeseries

import (
	"slices"
	"time"

	"charm.land/lipgloss/v2"
	tslc "github.com/NimbleMarkets/ntcharts/v2/linechart/timeserieslinechart"

	"github.com/kpumuk/lazykiq/internal/ui/charts"
)

// Styles holds the visual styles for the timeseries chart.
type Styles struct {
	Axis  lipgloss.Style // Style for chart axes
	Label lipgloss.Style // Style for axis labels
}

// DefaultStyles returns sensible default styles.
func DefaultStyles() Styles {
	return Styles{
		Axis:  lipgloss.NewStyle(),
		Label: lipgloss.NewStyle(),
	}
}

// Series represents a single data series to plot.
type Series struct {
	Name   string         // Unique name for this series
	Times  []time.Time    // Time points
	Values []float64      // Values at each time point
	Style  lipgloss.Style // Line style for this series
}

// Model holds the timeseries chart state.
type Model struct {
	styles       Styles
	width        int
	height       int
	series       []Series
	xFormatter   func(int, float64) string
	yFormatter   func(int, float64) string
	xSteps       int
	ySteps       int
	minTime      *time.Time
	maxTime      *time.Time
	minValue     *float64
	maxValue     *float64
	emptyMessage string
}

// Option is a functional option for configuring the timeseries chart.
type Option func(*Model)

// New creates a new timeseries chart model with functional options.
func New(opts ...Option) Model {
	m := Model{
		styles:     DefaultStyles(),
		xSteps:     2,
		ySteps:     2,
		xFormatter: func(int, float64) string { return "" },
		yFormatter: func(int, float64) string { return "" },
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// WithStyles sets custom styles for the chart.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}

// WithSize sets the dimensions of the chart.
func WithSize(w, h int) Option {
	return func(m *Model) { m.width, m.height = w, h }
}

// WithSeries sets the data series to display.
func WithSeries(series ...Series) Option {
	return func(m *Model) { m.series = series }
}

// WithXFormatter sets the X-axis label formatter.
func WithXFormatter(formatter func(int, float64) string) Option {
	return func(m *Model) { m.xFormatter = formatter }
}

// WithYFormatter sets the Y-axis label formatter.
func WithYFormatter(formatter func(int, float64) string) Option {
	return func(m *Model) { m.yFormatter = formatter }
}

// WithXYSteps sets the number of label steps for X and Y axes.
func WithXYSteps(xSteps, ySteps int) Option {
	return func(m *Model) { m.xSteps, m.ySteps = xSteps, ySteps }
}

// WithTimeRange sets explicit time range (overrides auto-detection).
func WithTimeRange(minTime, maxTime time.Time) Option {
	return func(m *Model) { m.minTime, m.maxTime = &minTime, &maxTime }
}

// WithValueRange sets explicit value range (overrides auto-detection).
func WithValueRange(minValue, maxValue float64) Option {
	return func(m *Model) { m.minValue, m.maxValue = &minValue, &maxValue }
}

// WithEmptyMessage sets the message to display when there's no data.
func WithEmptyMessage(msg string) Option {
	return func(m *Model) { m.emptyMessage = msg }
}

// SetStyles updates the chart styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize updates the chart dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetSeries updates the data series.
func (m *Model) SetSeries(series ...Series) {
	m.series = series
}

// SetXFormatter updates the X-axis label formatter.
func (m *Model) SetXFormatter(formatter func(int, float64) string) {
	m.xFormatter = formatter
}

// SetYFormatter updates the Y-axis label formatter.
func (m *Model) SetYFormatter(formatter func(int, float64) string) {
	m.yFormatter = formatter
}

// SetXYSteps updates the number of label steps for X and Y axes.
func (m *Model) SetXYSteps(xSteps, ySteps int) {
	m.xSteps, m.ySteps = xSteps, ySteps
}

// SetTimeRange sets an explicit time range (overrides auto-detection).
func (m *Model) SetTimeRange(minTime, maxTime time.Time) {
	m.minTime, m.maxTime = &minTime, &maxTime
}

// SetValueRange sets an explicit value range (overrides auto-detection).
func (m *Model) SetValueRange(minValue, maxValue float64) {
	m.minValue, m.maxValue = &minValue, &maxValue
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

// View renders the timeseries chart to a string.
func (m Model) View() string {
	if m.width < 1 || m.height < 1 {
		return ""
	}

	// Find common length across all series
	n := m.commonLength()
	if n == 0 {
		return charts.RenderCentered(m.width, m.height, m.emptyMessage)
	}

	// Detect or use provided time range
	minTime, maxTime := m.detectTimeRange(n)
	if !maxTime.After(minTime) {
		maxTime = minTime.Add(time.Second)
	}

	// Detect or use provided value range
	minValue, maxValue := m.detectValueRange(n)

	// Create chart
	chart := tslc.New(m.width, m.height,
		tslc.WithXYSteps(m.xSteps, m.ySteps),
		tslc.WithXLabelFormatter(m.xFormatter),
		tslc.WithYLabelFormatter(m.yFormatter),
		tslc.WithAxesStyles(m.styles.Axis, m.styles.Label),
		tslc.WithTimeRange(minTime, maxTime),
		tslc.WithYRange(minValue, maxValue),
	)
	chart.AutoMinX = false
	chart.AutoMaxX = false
	chart.AutoMinY = false
	chart.AutoMaxY = false

	// Plot all series
	for _, series := range m.series {
		times := series.Times
		values := series.Values

		// Trim to common length
		if len(times) > n {
			times = times[len(times)-n:]
		}
		if len(values) > n {
			values = values[len(values)-n:]
		}

		// Set style for this series
		if series.Name == "" || series.Name == m.series[0].Name {
			// First series uses default style
			chart.SetStyle(series.Style)
		} else {
			// Subsequent series use named datasets
			chart.SetDataSetStyle(series.Name, series.Style)
		}

		// Push data points
		count := min(len(times), len(values))
		for i := range count {
			point := tslc.TimePoint{Time: times[i], Value: values[i]}
			if series.Name == "" || series.Name == m.series[0].Name {
				chart.Push(point)
			} else {
				chart.PushDataSet(series.Name, point)
			}
		}
	}

	chart.DrawBrailleAll()
	return chart.View()
}

// commonLength finds the minimum length across all series.
func (m Model) commonLength() int {
	if len(m.series) == 0 {
		return 0
	}
	n := len(m.series[0].Times)
	for _, series := range m.series {
		n = min(n, len(series.Times))
		n = min(n, len(series.Values))
	}
	return n
}

// detectTimeRange determines the time range from the data or uses provided range.
func (m Model) detectTimeRange(n int) (time.Time, time.Time) {
	if m.minTime != nil && m.maxTime != nil {
		return *m.minTime, *m.maxTime
	}

	if len(m.series) == 0 || n == 0 {
		return time.Now(), time.Now()
	}

	// Use first series for time range
	times := m.series[0].Times
	if len(times) > n {
		times = times[len(times)-n:]
	}

	minTime := times[0]
	maxTime := times[len(times)-1]

	if m.minTime != nil {
		minTime = *m.minTime
	}
	if m.maxTime != nil {
		maxTime = *m.maxTime
	}

	return minTime, maxTime
}

// detectValueRange determines the value range from the data or uses provided range.
func (m Model) detectValueRange(n int) (float64, float64) {
	if m.minValue != nil && m.maxValue != nil {
		return *m.minValue, *m.maxValue
	}

	minValue := 0.0
	maxValue := 1.0

	// Find max value across all series
	for _, series := range m.series {
		values := series.Values
		if len(values) > n {
			values = values[len(values)-n:]
		}
		if len(values) > 0 {
			maxValue = max(maxValue, slices.Max(values))
		}
	}

	if m.minValue != nil {
		minValue = *m.minValue
	}
	if m.maxValue != nil {
		maxValue = *m.maxValue
	}

	return minValue, maxValue
}
