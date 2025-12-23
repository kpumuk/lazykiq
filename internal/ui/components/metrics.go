package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
)

// MetricsData holds the Sidekiq metrics values
type MetricsData struct {
	Processed int64
	Failed    int64
	Busy      int64
	Enqueued  int64
	Retries   int64
	Scheduled int64
	Dead      int64
}

// MetricsUpdateMsg is sent when metrics should be updated
type MetricsUpdateMsg struct {
	Data MetricsData
}

// Metrics displays the Sidekiq metrics bar at the top of the screen
type Metrics struct {
	width  int
	data   MetricsData
	styles *theme.Styles
}

// NewMetrics creates a new Metrics component
func NewMetrics(styles *theme.Styles) Metrics {
	return Metrics{
		styles: styles,
		data: MetricsData{
			Processed: 0,
			Failed:    0,
			Busy:      0,
			Enqueued:  0,
			Retries:   0,
			Scheduled: 0,
			Dead:      0,
		},
	}
}

// Init returns an initial command
func (m Metrics) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m Metrics) Update(msg tea.Msg) (Metrics, tea.Cmd) {
	switch msg := msg.(type) {
	case MetricsUpdateMsg:
		m.data = msg.Data
	}
	return m, nil
}

// View renders the metrics bar
func (m Metrics) View() string {
	s := m.styles

	barStyle := s.MetricsBar.Width(m.width)

	sep := s.MetricSep.Render(" │ ")

	metrics := []string{
		s.MetricLabel.Render("Processed: ") + s.MetricValue.Render(formatNumber(m.data.Processed)),
		s.MetricLabel.Render("Failed: ") + s.MetricValue.Render(formatNumber(m.data.Failed)),
		s.MetricLabel.Render("Busy: ") + s.MetricValue.Render(formatNumber(m.data.Busy)),
		s.MetricLabel.Render("Enqueued: ") + s.MetricValue.Render(formatNumber(m.data.Enqueued)),
		s.MetricLabel.Render("Retries: ") + s.MetricValue.Render(formatNumber(m.data.Retries)),
		s.MetricLabel.Render("Scheduled: ") + s.MetricValue.Render(formatNumber(m.data.Scheduled)),
		s.MetricLabel.Render("Dead: ") + s.MetricValue.Render(formatNumber(m.data.Dead)),
	}

	content := ""
	for i, metric := range metrics {
		if i > 0 {
			content += sep
		}
		content += metric
	}

	return barStyle.Render(content)
}

// SetWidth updates the width of the metrics bar
func (m Metrics) SetWidth(width int) Metrics {
	m.width = width
	return m
}

// SetStyles updates the styles
func (m Metrics) SetStyles(styles *theme.Styles) Metrics {
	m.styles = styles
	return m
}

// Height returns the height of the metrics bar
func (m Metrics) Height() int {
	return 1
}

// formatNumber formats a number with K/M suffixes for readability
func formatNumber(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
