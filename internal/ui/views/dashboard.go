package views

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/stats"
	"github.com/kpumuk/lazykiq/internal/ui/components/timeseries"
	"github.com/kpumuk/lazykiq/internal/ui/display"
	"github.com/kpumuk/lazykiq/internal/ui/requestctx"
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
	requestctx.Scope
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

	realtimeChart chartRenderCache
	historyChart  chartRenderCache

	redisInfo sidekiq.RedisInfo

	redisInfoRequest requestctx.Controller
	historyRequest   requestctx.Controller
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
	if result, ok := msg.(ReadResultMsg); ok {
		if result.Owner != d || !result.Current() {
			return d, nil
		}
		if result.Err != nil {
			return d, nil
		}
		msg = result.Data
	}

	switch msg := msg.(type) {
	case stats.UpdateMsg:
		// Use stats from the shared metrics update (already fetched by app)
		var deltaProcessed int64
		var deltaFailed int64
		if d.hasLastTotals {
			deltaProcessed = msg.Data.Processed - d.lastProcessed
			deltaFailed = msg.Data.Failed - d.lastFailed
			deltaProcessed = max(deltaProcessed, 0)
			deltaFailed = max(deltaFailed, 0)
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
		d.realtimeChart.clear()
		return d, nil

	case DashboardRedisInfoMsg:
		d.redisInfo = msg.RedisInfo
		return d, nil

	case DashboardHistoryMsg:
		// Own the inputs so later changes to the reply cannot stale the cache.
		d.historyDates = slices.Clone(msg.history.Dates)
		d.historyProcessed = slices.Clone(msg.history.Processed)
		d.historyFailed = slices.Clone(msg.history.Failed)
		d.historyChart.clear()
		return d, nil

	case RefreshMsg:
		// Fetch Redis info on refresh (stats come via stats.UpdateMsg)
		return d, d.fetchRedisInfoCmd()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "tab":
			if d.focusedPane == dashboardPaneRealtime {
				d.focusedPane = dashboardPaneHistory
			} else {
				d.focusedPane = dashboardPaneRealtime
			}
			return d, nil
		case "{":
			return d.adjustHistoryRange(-1)
		case "}":
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

	// Each pane needs two border rows and at least one row for its summary.
	if d.height < 6 || d.width < 6 {
		return d.renderCompact()
	}
	topHeight := d.height / 2
	bottomHeight := d.height - topHeight

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
		{Label: "Connections", Value: display.ShortNumber(d.redisInfo.Connections)},
		{Label: "Memory", Value: orNA(d.redisInfo.UsedMemory)},
		{Label: "Peak", Value: orNA(d.redisInfo.UsedMemoryPeak)},
	}
}

// HintBindings implements HintProvider.
func (d *Dashboard) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"tab"}, "tab", "switch pane"),
		helpBinding([]string{"{", "}"}, "{ ⋰ }", "change period"),
	}
}

// HelpSections implements HelpProvider.
func (d *Dashboard) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Dashboard",
			Bindings: []key.Binding{
				helpBinding([]string{"tab"}, "tab", "switch pane"),
				helpBinding([]string{"{"}, "{", "previous range"),
				helpBinding([]string{"}"}, "}", "next range"),
			},
		},
	}
}

// SetSize implements View.
func (d *Dashboard) SetSize(width, height int) View {
	if d.width != width || d.height != height {
		d.realtimeChart.clear()
		d.historyChart.clear()
	}
	d.width = width
	d.height = height
	d.seedRealtimeSeries()
	d.trimRealtimeSeries()
	return d
}

// SetStyles implements View.
func (d *Dashboard) SetStyles(styles Styles) View {
	d.styles = styles
	d.realtimeChart.clear()
	d.historyChart.clear()
	return d
}

// CancelRequests stops in-flight dashboard fetches when the view is hidden.
func (d *Dashboard) CancelRequests() {
	d.realtimeChart.clear()
	d.historyChart.clear()
	d.redisInfoRequest.Cancel()
	d.historyRequest.Cancel()
}

func (d *Dashboard) renderCompact() string {
	realtime := "Loading..."
	if len(d.realtimeProcessed) > 0 {
		realtime = d.renderRealtimeLegend(d.width)
	}
	history := "Loading..."
	if len(d.historyProcessed) > 0 {
		history = d.renderHistoryLegend(d.width)
	}
	lines := []string{
		d.styles.Title.Render("Dashboard: ") + realtime,
		d.styles.Title.Render("History: ") + history,
	}
	if d.height == 1 {
		lines = lines[d.focusedPane : d.focusedPane+1]
	}
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, d.width)
	}
	return strings.Join(lines, "\n") + strings.Repeat("\n", max(d.height-len(lines), 0))
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
	ctx := d.redisInfoRequest.Start(devtools.WithTracker(d.Context(), "dashboard.fetchRedisInfoCmd"))
	client := d.client
	return readCmd(ctx, d, func() tea.Msg {
		redisInfo, err := client.GetRedisInfo(ctx)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return DashboardRedisInfoMsg{RedisInfo: redisInfo}
	})
}

func (d *Dashboard) fetchHistoryCmd() tea.Cmd {
	ctx := d.historyRequest.Start(devtools.WithTracker(d.Context(), "dashboard.fetchHistoryCmd"))
	client, days := d.client, d.historyRanges[d.historyRangeIdx]
	return readCmd(ctx, d, func() tea.Msg {
		history, err := client.GetStatsHistory(ctx, days)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return DashboardHistoryMsg{history: history}
	})
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
		frame.WithFocused(d.focusedPane == dashboardPaneHistory),
	)
	return box.View()
}

func (d *Dashboard) renderRealtimeContent(contentHeight int) string {
	width := d.chartContentWidth()
	if contentHeight < 1 || width < 1 {
		return ""
	}
	if contentHeight == 1 && len(d.realtimeProcessed) > 0 {
		return d.renderRealtimeLegend(width)
	}

	chartHeight := contentHeight - 1
	if chartHeight < 1 {
		chartHeight = contentHeight
	}

	chartOutput, cached := d.realtimeChart.get(width, chartHeight)
	if !cached {
		chart := timeseries.New(
			timeseries.WithSize(width, chartHeight),
			timeseries.WithSeries(
				timeseries.Series{
					Name:   "processed",
					Times:  d.realtimeTimes,
					Values: int64ToFloat64(d.realtimeProcessed),
					Style:  d.styles.ChartSuccess,
				},
				timeseries.Series{
					Name:   "failed",
					Times:  d.realtimeTimes,
					Values: int64ToFloat64(d.realtimeFailed),
					Style:  d.styles.ChartFailure,
				},
			),
			timeseries.WithStyles(timeseries.Styles{
				Axis:  d.styles.ChartAxis,
				Label: d.styles.ChartLabel,
			}),
			timeseries.WithXFormatter(realtimeTimeLabelFormatter()),
			timeseries.WithYFormatter(shortYLabelFormatter()),
			timeseries.WithXYSteps(2, 2),
			timeseries.WithEmptyMessage("Loading..."),
		)
		chartOutput = d.realtimeChart.store(width, chartHeight, chart.View())
	}

	// Don't show legend if no data
	if len(d.realtimeProcessed) == 0 {
		return chartOutput
	}

	legend := d.renderRealtimeLegend(width)
	return chartOutput + "\n" + legend
}

func (d *Dashboard) renderHistoryContent(contentHeight int) string {
	width := d.chartContentWidth()
	if contentHeight < 1 || width < 1 {
		return ""
	}
	if contentHeight == 1 && len(d.historyProcessed) > 0 {
		return d.renderHistoryLegend(width)
	}

	chartHeight := contentHeight - 1
	if chartHeight < 1 {
		chartHeight = contentHeight
	}

	chartOutput, cached := d.historyChart.get(width, chartHeight)
	if !cached {
		chart := timeseries.New(
			timeseries.WithSize(width, chartHeight),
			timeseries.WithSeries(
				timeseries.Series{
					Name:   "processed",
					Times:  d.historyDates,
					Values: int64ToFloat64(d.historyProcessed),
					Style:  d.styles.ChartSuccess,
				},
				timeseries.Series{
					Name:   "failed",
					Times:  d.historyDates,
					Values: int64ToFloat64(d.historyFailed),
					Style:  d.styles.ChartFailure,
				},
			),
			timeseries.WithStyles(timeseries.Styles{
				Axis:  d.styles.ChartAxis,
				Label: d.styles.ChartLabel,
			}),
			timeseries.WithXFormatter(historyTimeLabelFormatter()),
			timeseries.WithYFormatter(shortYLabelFormatter()),
			timeseries.WithXYSteps(2, 2),
			timeseries.WithEmptyMessage("Loading..."),
		)
		chartOutput = d.historyChart.store(width, chartHeight, chart.View())
	}

	// Don't show legend if no data
	if len(d.historyProcessed) == 0 {
		return chartOutput
	}

	legend := d.renderHistoryLegend(width)
	return chartOutput + "\n" + legend
}

func (d *Dashboard) renderRealtimeLegend(width int) string {
	sep := d.styles.Muted.Render(" | ")
	processed := d.styles.MetricLabel.Render("Processed: ") + d.styles.MetricValue.Render(display.ShortNumber(d.lastDeltaP))
	failed := d.styles.MetricLabel.Render("Failed: ") + d.styles.MetricValue.Render(display.ShortNumber(d.lastDeltaF))
	timestamp := d.styles.Muted.Render(d.lastPollAt.Format("15:04:05"))
	line := processed + sep + failed + sep + timestamp
	return ansi.Cut(line, 0, width)
}

func (d *Dashboard) renderHistoryLegend(width int) string {
	sep := d.styles.Muted.Render(" | ")
	processed := d.styles.MetricLabel.Render("Processed: ") + d.styles.MetricValue.Render(display.ShortNumber(sumSeries(d.historyProcessed)))
	failed := d.styles.MetricLabel.Render("Failed: ") + d.styles.MetricValue.Render(display.ShortNumber(sumSeries(d.historyFailed)))
	rangeLabel := d.styles.Muted.Render(d.historyDateRangeLabel())
	line := processed + sep + failed + sep + rangeLabel
	return ansi.Cut(line, 0, width)
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

func int64ToFloat64(values []int64) []float64 {
	result := make([]float64, len(values))
	for i, v := range values {
		result[i] = float64(v)
	}
	return result
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
		return display.CompactNumber(int64(v + 0.5))
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
