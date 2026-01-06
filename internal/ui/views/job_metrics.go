package views

import (
	"context"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/charts"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/histogram"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/scatter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// jobMetricsDataMsg carries job metrics data.
type jobMetricsDataMsg struct {
	result sidekiq.MetricsJobDetailResult
}

// JobMetrics shows per-job execution metrics.
type JobMetrics struct {
	client  sidekiq.API
	width   int
	height  int
	styles  Styles
	jobName string
	periods []string
	period  string

	periodIdx int
	result    sidekiq.MetricsJobDetailResult
	processed *charts.ProcessedMetrics
	focused   int
}

// NewJobMetrics creates a new job metrics view.
func NewJobMetrics(client sidekiq.API) *JobMetrics {
	periods := []string{"1h", "2h", "4h", "8h"}
	return &JobMetrics{
		client:  client,
		periods: periods,
		period:  periods[0],
	}
}

// Init implements View.
func (j *JobMetrics) Init() tea.Cmd {
	if j.jobName == "" {
		return nil
	}
	return j.fetchCmd()
}

// Update implements View.
func (j *JobMetrics) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case jobMetricsDataMsg:
		j.result = msg.result
		// Pre-process histogram data once on arrival instead of every View() call
		j.processed = charts.ProcessHistogramData(j.result.Hist, j.result.BucketCount)
		return j, nil

	case RefreshMsg:
		return j, j.fetchCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			_, bottom := splitJobMetricsHeights(j.height)
			if bottom > 0 {
				j.focused = 1 - j.focused
				return j, nil
			}
			return j, nil
		case "[":
			return j.adjustPeriod(-1)
		case "]":
			return j.adjustPeriod(1)
		}
	}

	return j, nil
}

// View implements View.
func (j *JobMetrics) View() string {
	if j.width <= 0 || j.height <= 0 {
		return ""
	}

	title := "Metrics"
	if j.jobName != "" {
		title = j.jobName
	}

	// Check if we have data (no need for separate ready flag)
	if j.processed == nil || len(j.processed.SortedBuckets) == 0 {
		return messagebox.Render(messagebox.Styles{
			Title:  j.styles.Title,
			Muted:  j.styles.Muted,
			Border: j.styles.FocusBorder,
		}, title, "Loading...", j.width, j.height)
	}

	contentWidth := max(j.width-4, 0)
	if contentWidth == 0 || j.height <= 0 {
		return ""
	}

	topHeight, bottomHeight := splitJobMetricsHeights(j.height)
	meta := j.detailMeta()
	topChartHeight := max(topHeight-2, 0)
	bottomChartHeight := max(bottomHeight-2, 0)

	// Render top chart using histogram component
	labels := sidekiq.MetricsHistogramLabels
	if len(labels) > len(j.processed.BucketTotals) {
		labels = labels[:len(j.processed.BucketTotals)]
	}
	histogramChart := histogram.New(
		histogram.WithStyles(histogram.Styles{
			Axis:  j.styles.ChartAxis,
			Bar:   j.styles.ChartHistogram,
			Muted: j.styles.Muted,
		}),
		histogram.WithSize(contentWidth, topChartHeight),
		histogram.WithData(j.processed.BucketTotals, labels),
		histogram.WithEmptyMessage(j.noDataMessage()),
	)

	// Render bottom chart using scatter component
	scatterLabels := sidekiq.MetricsHistogramLabels
	if j.processed.BucketCount > 0 && len(scatterLabels) > j.processed.BucketCount {
		scatterLabels = scatterLabels[:j.processed.BucketCount]
	}
	scatterChart := scatter.New(
		scatter.WithStyles(scatter.Styles{
			Axis:  j.styles.ChartAxis,
			Label: j.styles.ChartLabel,
			Point: j.styles.ChartHistogram,
			Muted: j.styles.Muted,
		}),
		scatter.WithSize(contentWidth, bottomChartHeight),
		scatter.WithData(
			j.processed.ScatterPoints,
			j.processed.SortedBuckets,
			scatterLabels,
			j.processed.MaxCount,
			j.processed.MaxBucket,
		),
		scatter.WithEmptyMessage(j.noDataMessage()),
	)

	frameStyles := frame.Styles{
		Focused: frame.StyleState{
			Title:  j.styles.Title,
			Muted:  j.styles.Muted,
			Filter: j.styles.FilterFocused,
			Border: j.styles.FocusBorder,
		},
		Blurred: frame.StyleState{
			Title:  j.styles.Title,
			Muted:  j.styles.Muted,
			Filter: j.styles.FilterBlurred,
			Border: j.styles.BorderStyle,
		},
	}

	topFrame := frame.New(
		frame.WithStyles(frameStyles),
		frame.WithTitle("Execution Time Buckets"),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(histogramChart.View()),
		frame.WithPadding(1),
		frame.WithSize(j.width, topHeight),
		frame.WithMinHeight(5),
		frame.WithFocused(bottomHeight == 0 || j.focused == 0),
	)

	if bottomHeight <= 0 {
		return topFrame.View()
	}

	bottomFrame := frame.New(
		frame.WithStyles(frameStyles),
		frame.WithTitle("Execution Scatter"),
		frame.WithTitlePadding(0),
		frame.WithContent(scatterChart.View()),
		frame.WithPadding(1),
		frame.WithSize(j.width, bottomHeight),
		frame.WithMinHeight(5),
		frame.WithFocused(j.focused == 1),
	)

	return lipgloss.JoinVertical(lipgloss.Left, topFrame.View(), bottomFrame.View())
}

// Name implements View.
func (j *JobMetrics) Name() string {
	if j.jobName != "" {
		return j.jobName
	}
	return "Job Metrics"
}

// ShortHelp implements View.
func (j *JobMetrics) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (j *JobMetrics) ContextItems() []ContextItem {
	jobName := j.jobName
	if strings.TrimSpace(jobName) == "" {
		jobName = "-"
	}

	success := "-"
	failed := "-"
	avg := "-"
	rangeText := "-"
	if j.processed != nil && len(j.processed.SortedBuckets) > 0 {
		success = format.Number(j.result.Totals.Success())
		failed = format.Number(j.result.Totals.Failed)
		avg = format.Float(j.result.Totals.AvgSeconds(), 2) + "s"
		if value := formatMetricsRange(j.result.StartsAt, j.result.EndsAt); value != "" {
			rangeText = value
		}
	}

	return []ContextItem{
		{Label: "Job", Value: jobName},
		{Label: "Success", Value: success},
		{Label: "Failed", Value: failed},
		{Label: "Average", Value: avg},
		{Label: "Range", Value: rangeText},
	}
}

// HintBindings implements HintProvider.
func (j *JobMetrics) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"tab"}, "tab", "switch panel"),
		helpBinding([]string{"["}, "[", "prev period"),
		helpBinding([]string{"]"}, "]", "next period"),
	}
}

// HelpSections implements HelpProvider.
func (j *JobMetrics) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Job Metrics",
			Bindings: []key.Binding{
				helpBinding([]string{"tab"}, "tab", "switch panel"),
				helpBinding([]string{"shift+tab"}, "shift+tab", "switch panel"),
				helpBinding([]string{"["}, "[", "previous period"),
				helpBinding([]string{"]"}, "]", "next period"),
			},
		},
	}
}

// SetSize implements View.
func (j *JobMetrics) SetSize(width, height int) View {
	j.width = width
	j.height = height
	_, bottom := splitJobMetricsHeights(height)
	if bottom == 0 {
		j.focused = 0
	}
	return j
}

// SetStyles implements View.
func (j *JobMetrics) SetStyles(styles Styles) View {
	j.styles = styles
	return j
}

// SetJobMetrics sets the job name and period to display.
func (j *JobMetrics) SetJobMetrics(jobName, period string) {
	j.jobName = jobName
	if idx := slices.Index(j.periods, period); idx >= 0 {
		j.periodIdx = idx
		j.period = j.periods[idx]
	} else {
		j.periodIdx = len(j.periods) - 1
		j.period = j.periods[j.periodIdx]
	}
	j.result = sidekiq.MetricsJobDetailResult{}
	j.processed = nil
	j.focused = 0
}

// Dispose clears cached data when the view is removed from the stack.
func (j *JobMetrics) Dispose() {
	j.jobName = ""
	j.periodIdx = 0
	j.period = j.periods[0]
	j.result = sidekiq.MetricsJobDetailResult{}
	j.processed = nil
	j.focused = 0
}

func (j *JobMetrics) fetchCmd() tea.Cmd {
	jobName := j.jobName
	period := j.period
	client := j.client
	periods := j.periods
	return func() tea.Msg {
		ctx := context.Background()
		params, ok := sidekiq.MetricsPeriods[period]
		if !ok {
			params = sidekiq.MetricsPeriods[periods[0]]
		}
		result, err := client.GetMetricsJobDetail(ctx, jobName, params)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return jobMetricsDataMsg{result: result}
	}
}

func (j *JobMetrics) adjustPeriod(delta int) (View, tea.Cmd) {
	next := mathutil.Clamp(j.periodIdx+delta, 0, len(j.periods)-1)
	if next == j.periodIdx {
		return j, nil
	}
	j.periodIdx = next
	j.period = j.periods[next]
	return j, j.fetchCmd()
}

func (j *JobMetrics) detailMeta() string {
	if j.period == "" {
		return ""
	}
	return j.styles.MetricLabel.Render("period: ") + j.styles.MetricValue.Render(j.period)
}

func (j *JobMetrics) noDataMessage() string {
	jobName := j.jobName
	if jobName == "" {
		jobName = "unknown"
	}
	line1 := j.styles.Muted.Render("No data available for the job")
	line2 := j.styles.Muted.Bold(true).Render(jobName)
	line3 := j.styles.Muted.Render("Try increasing period")
	return line1 + "\n" + line2 + "\n" + line3
}

// splitJobMetricsHeights splits the total height between top and bottom panels.
func splitJobMetricsHeights(total int) (int, int) {
	top := max(total/2, 5)
	bottom := total - top
	if bottom < 5 {
		bottom = 5
		top = total - bottom
	}
	if top < 5 {
		top = max(total, 0)
		bottom = 0
	}
	return top, bottom
}
