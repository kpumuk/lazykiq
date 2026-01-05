package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tslc "github.com/NimbleMarkets/ntcharts/v2/linechart/timeserieslinechart"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/metrics"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

const (
	dashboardPaneRealtime = iota
	dashboardPaneHistory
)

// DashboardHistoryMsg carries historical dashboard data.
type DashboardHistoryMsg struct {
	history sidekiq.StatsHistory
}

// DashboardRedisInfoMsg carries Redis info for the dashboard.
type DashboardRedisInfoMsg struct {
	RedisInfo sidekiq.RedisInfo
}

// Dashboard is the main overview view.
type Dashboard struct {
	client sidekiq.API
	width  int
	height int
	styles Styles

	focusedPane     int
	historyRanges   []int
	historyRangeIdx int

	lastProcessed int64
	lastFailed    int64
	hasLastTotals bool
	lastPollAt    time.Time
	lastDeltaP    int64
	lastDeltaF    int64

	realtimeProcessed []int64
	realtimeFailed    []int64
	realtimeTimes     []time.Time

	historyDates     []time.Time
	historyProcessed []int64
	historyFailed    []int64

	redisInfo sidekiq.RedisInfo
}

// NewDashboard creates a new Dashboard view.
func NewDashboard(client sidekiq.API) *Dashboard {
	return &Dashboard{
		client:          client,
		focusedPane:     dashboardPaneRealtime,
		historyRanges:   []int{7, 30, 90, 180},
		historyRangeIdx: 1,
	}
}

// Init implements View.
func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(
		d.fetchRedisInfoCmd(),
		d.fetchHistoryCmd(),
	)
}

// Update implements View.
func (d *Dashboard) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case metrics.UpdateMsg:
		// Use stats from the shared metrics update (already fetched by app)
		var deltaProcessed int64
		var deltaFailed int64
		if d.hasLastTotals {
			deltaProcessed = msg.Data.Processed - d.lastProcessed
			deltaFailed = msg.Data.Failed - d.lastFailed
			if deltaProcessed < 0 {
				deltaProcessed = 0
			}
			if deltaFailed < 0 {
				deltaFailed = 0
			}
		}
		d.lastProcessed = msg.Data.Processed
		d.lastFailed = msg.Data.Failed
		if !d.hasLastTotals {
			d.hasLastTotals = true
			return d, nil
		}

		if deltaProcessed == 0 && deltaFailed == 0 {
			return d, nil
		}

		d.lastPollAt = msg.Data.UpdatedAt
		d.lastDeltaP = deltaProcessed
		d.lastDeltaF = deltaFailed
		d.realtimeProcessed = append(d.realtimeProcessed, deltaProcessed)
		d.realtimeFailed = append(d.realtimeFailed, deltaFailed)
		d.realtimeTimes = append(d.realtimeTimes, msg.Data.UpdatedAt)
		d.trimRealtimeSeries()
		return d, nil

	case DashboardRedisInfoMsg:
		d.redisInfo = msg.RedisInfo
		return d, nil

	case DashboardHistoryMsg:
		d.historyDates = msg.history.Dates
		d.historyProcessed = msg.history.Processed
		d.historyFailed = msg.history.Failed
		return d, nil

	case RefreshMsg:
		// Fetch Redis info on refresh (stats come via metrics.UpdateMsg)
		return d, d.fetchRedisInfoCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if d.focusedPane == dashboardPaneRealtime {
				d.focusedPane = dashboardPaneHistory
			} else {
				d.focusedPane = dashboardPaneRealtime
			}
			return d, nil
		case "[":
			return d.adjustHistoryRange(-1)
		case "]":
			return d.adjustHistoryRange(1)
		}
	}

	return d, nil
}

// View implements View.
func (d *Dashboard) View() string {
	if d.width <= 0 || d.height <= 0 {
		return ""
	}

	available := max(d.height, 2)
	topHeight := available / 2
	bottomHeight := available - topHeight

	realtimeBox := d.renderRealtimeBox(topHeight)
	historyBox := d.renderHistoryBox(bottomHeight)

	return lipgloss.JoinVertical(lipgloss.Left, realtimeBox, historyBox)
}

// Name implements View.
func (d *Dashboard) Name() string {
	return "Dashboard"
}

// ShortHelp implements View.
func (d *Dashboard) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (d *Dashboard) ContextItems() []ContextItem {
	redisVersion := orNA(d.redisInfo.Version)
	redisURL := strings.TrimSpace(d.client.DisplayRedisURL())
	redisValue := redisVersion
	if redisURL != "" {
		redisValue = fmt.Sprintf("%s (%s)", redisVersion, redisURL)
	}

	return []ContextItem{
		{Label: "Redis", Value: redisValue},
		{Label: "Uptime", Value: fmt.Sprintf("%d days", d.redisInfo.UptimeDays)},
		{Label: "Connections", Value: format.ShortNumber(d.redisInfo.Connections)},
		{Label: "Memory", Value: orNA(d.redisInfo.UsedMemory)},
		{Label: "Peak", Value: orNA(d.redisInfo.UsedMemoryPeak)},
	}
}

// HintBindings implements HintProvider.
func (d *Dashboard) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"tab"}, "tab", "switch pane"),
		helpBinding([]string{"["}, "[", "prev range"),
		helpBinding([]string{"]"}, "]", "next range"),
	}
}

// HelpSections implements HelpProvider.
func (d *Dashboard) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Dashboard",
			Bindings: []key.Binding{
				helpBinding([]string{"tab"}, "tab", "switch pane"),
				helpBinding([]string{"["}, "[", "previous range"),
				helpBinding([]string{"]"}, "]", "next range"),
			},
		},
	}
}

// SetSize implements View.
func (d *Dashboard) SetSize(width, height int) View {
	d.width = width
	d.height = height
	d.seedRealtimeSeries()
	d.trimRealtimeSeries()
	return d
}

// SetStyles implements View.
func (d *Dashboard) SetStyles(styles Styles) View {
	d.styles = styles
	return d
}

func (d *Dashboard) adjustHistoryRange(delta int) (View, tea.Cmd) {
	next := max(d.historyRangeIdx+delta, 0)
	if next >= len(d.historyRanges) {
		next = len(d.historyRanges) - 1
	}
	if next != d.historyRangeIdx {
		d.historyRangeIdx = next
		return d, d.fetchHistoryCmd()
	}
	return d, nil
}

func (d *Dashboard) fetchRedisInfoCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		redisInfo, err := d.client.GetRedisInfo(ctx)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return DashboardRedisInfoMsg{RedisInfo: redisInfo}
	}
}

func (d *Dashboard) fetchHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		days := d.historyRanges[d.historyRangeIdx]
		history, err := d.client.GetStatsHistory(ctx, days)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return DashboardHistoryMsg{history: history}
	}
}

func (d *Dashboard) renderRealtimeBox(height int) string {
	content := d.renderRealtimeContent(height - 2)
	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  d.styles.Title,
				Muted:  d.styles.Muted,
				Filter: d.styles.FilterFocused,
				Border: d.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  d.styles.Title,
				Muted:  d.styles.Muted,
				Filter: d.styles.FilterBlurred,
				Border: d.styles.BorderStyle,
			},
		}),
		frame.WithTitle("Dashboard"),
		frame.WithTitlePadding(0),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(d.width, height),
		frame.WithMinHeight(5),
		frame.WithFocused(d.focusedPane == dashboardPaneRealtime),
	)
	return box.View()
}

func (d *Dashboard) renderHistoryBox(height int) string {
	meta := d.styles.MetricLabel.Render("range: ") + d.styles.MetricValue.Render(d.historyRangeLabel())
	content := d.renderHistoryContent(height - 2)
	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  d.styles.Title,
				Muted:  d.styles.Muted,
				Filter: d.styles.FilterFocused,
				Border: d.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  d.styles.Title,
				Muted:  d.styles.Muted,
				Filter: d.styles.FilterBlurred,
				Border: d.styles.BorderStyle,
			},
		}),
		frame.WithTitle("History"),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(d.width, height),
		frame.WithMinHeight(5),
		frame.WithFocused(d.focusedPane == dashboardPaneHistory),
	)
	return box.View()
}

func (d *Dashboard) renderRealtimeContent(contentHeight int) string {
	width := d.chartContentWidth()
	if contentHeight < 1 || width < 1 {
		return ""
	}
	if len(d.realtimeProcessed) == 0 {
		return renderCenteredLoading(width, contentHeight)
	}

	chartHeight := contentHeight - 1
	if chartHeight < 1 {
		return renderCenteredLoading(width, contentHeight)
	}

	chart := d.renderTimeSeriesChart(width, chartHeight, d.realtimeTimes, d.realtimeProcessed, d.realtimeFailed, realtimeTimeLabelFormatter())
	legend := d.renderRealtimeLegend(width)
	return chart + "\n" + legend
}

func (d *Dashboard) renderHistoryContent(contentHeight int) string {
	width := d.chartContentWidth()
	if contentHeight < 1 || width < 1 {
		return ""
	}
	if len(d.historyProcessed) == 0 {
		return renderCenteredLoading(width, contentHeight)
	}

	chartHeight := contentHeight - 1
	if chartHeight < 1 {
		return renderCenteredLoading(width, contentHeight)
	}

	chart := d.renderTimeSeriesChart(width, chartHeight, d.historyDates, d.historyProcessed, d.historyFailed, historyTimeLabelFormatter())
	legend := d.renderHistoryLegend(width)
	return chart + "\n" + legend
}

func (d *Dashboard) renderRealtimeLegend(width int) string {
	sep := d.styles.Muted.Render(" | ")
	processed := d.styles.MetricLabel.Render("Processed: ") + d.styles.MetricValue.Render(format.ShortNumber(d.lastDeltaP))
	failed := d.styles.MetricLabel.Render("Failed: ") + d.styles.MetricValue.Render(format.ShortNumber(d.lastDeltaF))
	timestamp := d.styles.Muted.Render(d.lastPollAt.Format("15:04:05"))
	line := processed + sep + failed + sep + timestamp
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func (d *Dashboard) renderHistoryLegend(width int) string {
	sep := d.styles.Muted.Render(" | ")
	processed := d.styles.MetricLabel.Render("Processed: ") + d.styles.MetricValue.Render(format.ShortNumber(sumSeries(d.historyProcessed)))
	failed := d.styles.MetricLabel.Render("Failed: ") + d.styles.MetricValue.Render(format.ShortNumber(sumSeries(d.historyFailed)))
	rangeLabel := d.styles.Muted.Render(d.historyDateRangeLabel())
	line := processed + sep + failed + sep + rangeLabel
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func (d *Dashboard) historyRangeLabel() string {
	if d.historyRangeIdx < 0 || d.historyRangeIdx >= len(d.historyRanges) {
		return "1 month"
	}
	switch d.historyRanges[d.historyRangeIdx] {
	case 7:
		return "1 week"
	case 30:
		return "1 month"
	case 90:
		return "3 months"
	case 180:
		return "6 months"
	default:
		return fmt.Sprintf("%d days", d.historyRanges[d.historyRangeIdx])
	}
}

func (d *Dashboard) historyDateRangeLabel() string {
	if len(d.historyDates) == 0 {
		return ""
	}
	start := strings.ToUpper(d.historyDates[0].Format("Jan02"))
	end := strings.ToUpper(d.historyDates[len(d.historyDates)-1].Format("Jan02"))
	return start + ".." + end
}

func (d *Dashboard) chartContentWidth() int {
	width := d.width - 4
	if width < 1 {
		return 1
	}
	return width
}

func (d *Dashboard) trimRealtimeSeries() {
	maxPoints := d.chartContentWidth()
	d.realtimeProcessed = trimSeries(d.realtimeProcessed, maxPoints)
	d.realtimeFailed = trimSeries(d.realtimeFailed, maxPoints)
	d.realtimeTimes = trimTimes(d.realtimeTimes, maxPoints)
}

func trimSeries(values []int64, maxItems int) []int64 {
	if maxItems <= 0 {
		return nil
	}
	if len(values) <= maxItems {
		return values
	}
	return values[len(values)-maxItems:]
}

func (d *Dashboard) renderTimeSeriesChart(width, height int, times []time.Time, processed, failed []int64, xFormatter func(int, float64) string) string {
	if width < 1 || height < 1 {
		return ""
	}
	n := min(len(times), len(processed), len(failed))
	if n == 0 {
		return renderCenteredLoading(width, height)
	}

	times = times[len(times)-n:]
	processed = processed[len(processed)-n:]
	failed = failed[len(failed)-n:]

	minTime := times[0]
	maxTime := times[len(times)-1]
	if !maxTime.After(minTime) {
		maxTime = minTime.Add(time.Second)
	}

	maxVal := int64(1)
	for i := range n {
		if processed[i] > maxVal {
			maxVal = processed[i]
		}
		if failed[i] > maxVal {
			maxVal = failed[i]
		}
	}

	chart := tslc.New(width, height,
		tslc.WithXYSteps(2, 2),
		tslc.WithXLabelFormatter(xFormatter),
		tslc.WithYLabelFormatter(shortYLabelFormatter()),
		tslc.WithAxesStyles(d.styles.ChartAxis, d.styles.ChartLabel),
		tslc.WithTimeRange(minTime, maxTime),
		tslc.WithYRange(0, float64(maxVal)),
	)
	chart.AutoMinX = false
	chart.AutoMaxX = false
	chart.AutoMinY = false
	chart.AutoMaxY = false
	chart.SetStyle(d.styles.ChartSuccess)
	chart.SetDataSetStyle("failed", d.styles.ChartFailure)

	for i := range n {
		pointTime := times[i]
		chart.Push(tslc.TimePoint{Time: pointTime, Value: float64(processed[i])})
		chart.PushDataSet("failed", tslc.TimePoint{Time: pointTime, Value: float64(failed[i])})
	}

	chart.DrawBrailleAll()
	return chart.View()
}

func renderCenteredLoading(width, height int) string {
	if height < 1 {
		return ""
	}
	lines := make([]string, height)
	target := height / 2
	for i := range height {
		lines[i] = strings.Repeat(" ", width)
	}
	if width > 0 {
		trimmed := lipgloss.NewStyle().MaxWidth(width).Render("Loading...")
		padding := max((width-lipgloss.Width(trimmed))/2, 0)
		lines[target] = strings.Repeat(" ", padding) + trimmed
	}
	return strings.Join(lines, "\n")
}

func sumSeries(values []int64) int64 {
	var total int64
	for _, v := range values {
		total += v
	}
	return total
}

func orNA(value string) string {
	if value == "" {
		return "n/a"
	}
	return value
}

func trimTimes(values []time.Time, maxItems int) []time.Time {
	if maxItems <= 0 {
		return nil
	}
	if len(values) <= maxItems {
		return values
	}
	return values[len(values)-maxItems:]
}

func (d *Dashboard) seedRealtimeSeries() {
	if len(d.realtimeTimes) > 0 {
		return
	}
	maxPoints := d.chartContentWidth()
	if maxPoints <= 0 {
		return
	}
	// App ticker runs every 5 seconds
	const interval = 5 * time.Second
	start := time.Now().Add(-interval * time.Duration(maxPoints-1))
	for i := range maxPoints {
		d.realtimeTimes = append(d.realtimeTimes, start.Add(interval*time.Duration(i)))
		d.realtimeProcessed = append(d.realtimeProcessed, 0)
		d.realtimeFailed = append(d.realtimeFailed, 0)
	}
}

func shortYLabelFormatter() func(int, float64) string {
	return func(_ int, v float64) string {
		return format.CompactNumber(int64(v + 0.5))
	}
}

func realtimeTimeLabelFormatter() func(int, float64) string {
	return func(_ int, v float64) string {
		return time.Unix(int64(v), 0).UTC().Format("15:04")
	}
}

func historyTimeLabelFormatter() func(int, float64) string {
	return func(_ int, v float64) string {
		return strings.ToUpper(time.Unix(int64(v), 0).UTC().Format("Jan02"))
	}
}
