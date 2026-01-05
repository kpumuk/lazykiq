package views

import (
	"context"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// jobMetricsDataMsg carries job metrics data.
type jobMetricsDataMsg struct {
	result sidekiq.MetricsJobDetailResult
}

// JobMetrics shows per-job execution metrics.
type JobMetrics struct {
	client sidekiq.API
	width  int
	height int
	styles Styles

	ready     bool
	jobName   string
	periods   []string
	period    string
	periodIdx int
	result    sidekiq.MetricsJobDetailResult
	processed processedMetrics // Pre-computed histogram data
	focused   int

	chartAxisStyle    lipgloss.Style
	chartBarStyle     lipgloss.Style
	scatterAxisStyle  lipgloss.Style
	scatterLabelStyle lipgloss.Style
	scatterPointStyle lipgloss.Style
}

// NewJobMetrics creates a new job metrics view.
func NewJobMetrics(client sidekiq.API) *JobMetrics {
	periods := []string{"1h", "2h", "4h", "8h"}
	m := &JobMetrics{
		client:  client,
		periods: periods,
		period:  periods[0],
	}
	m.initChartStyles()
	return m
}

// Init implements View.
func (j *JobMetrics) Init() tea.Cmd {
	j.ready = false
	j.result = sidekiq.MetricsJobDetailResult{}
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
		j.processed = processHistogramData(j.result.Hist, j.result.BucketCount)
		j.ready = true
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

	if !j.ready {
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

	// Use pre-processed data computed on data arrival
	buckets := j.processed.sortedBuckets
	bucketTotals := j.processed.bucketTotals
	labels := sidekiq.MetricsHistogramLabels
	if len(labels) > len(bucketTotals) {
		labels = labels[:len(bucketTotals)]
	}

	meta := j.detailMeta()
	topChartHeight := max(topHeight-2, 0)
	bottomChartHeight := max(bottomHeight-2, 0)
	topChart := j.renderColumnsChart(contentWidth, topChartHeight, bucketTotals, labels)
	bottomChart := j.renderScatter(contentWidth, bottomChartHeight, buckets)

	topFrame := frame.New(
		frame.WithStyles(frame.Styles{
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
		}),
		frame.WithTitle("Execution Time Buckets"),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(topChart),
		frame.WithPadding(1),
		frame.WithSize(j.width, topHeight),
		frame.WithMinHeight(5),
		frame.WithFocused(bottomHeight == 0 || j.focused == 0),
	)

	if bottomHeight <= 0 {
		return topFrame.View()
	}

	bottomFrame := frame.New(
		frame.WithStyles(frame.Styles{
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
		}),
		frame.WithTitle("Execution Scatter"),
		frame.WithTitlePadding(0),
		frame.WithContent(bottomChart),
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
	if j.ready {
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
	j.initChartStyles()
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
	j.ready = false
	j.result = sidekiq.MetricsJobDetailResult{}
	j.processed = processedMetrics{}
	j.focused = 0
}

// Dispose clears cached data when the view is removed from the stack.
func (j *JobMetrics) Dispose() {
	j.jobName = ""
	j.ready = false
	j.periodIdx = 0
	j.period = j.periods[0]
	j.result = sidekiq.MetricsJobDetailResult{}
	j.processed = processedMetrics{}
	j.focused = 0
}

func (j *JobMetrics) fetchCmd() tea.Cmd {
	jobName := j.jobName
	period := j.period
	return func() tea.Msg {
		ctx := context.Background()
		params, ok := sidekiq.MetricsPeriods[period]
		if !ok {
			params = sidekiq.MetricsPeriods[j.periods[0]]
		}
		result, err := j.client.GetMetricsJobDetail(ctx, jobName, params)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return jobMetricsDataMsg{result: result}
	}
}

func (j *JobMetrics) adjustPeriod(delta int) (View, tea.Cmd) {
	next := clampInt(j.periodIdx+delta, len(j.periods)-1)
	if next == j.periodIdx {
		return j, nil
	}
	j.periodIdx = next
	j.period = j.periods[next]
	j.ready = false
	return j, j.fetchCmd()
}

func (j *JobMetrics) detailMeta() string {
	if j.period == "" {
		return ""
	}
	return j.styles.MetricLabel.Render("period: ") + j.styles.MetricValue.Render(j.period)
}

func (j *JobMetrics) noDataMessage(_ int) string {
	jobName := j.jobName
	if jobName == "" {
		jobName = "unknown"
	}
	line1 := j.styles.Muted.Render("No data available for the job")
	line2 := j.styles.Muted.Bold(true).Render(jobName)
	line3 := j.styles.Muted.Render("Try increasing period")
	return line1 + "\n" + line2 + "\n" + line3
}

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

func (j *JobMetrics) renderColumnsChart(width, height int, totals []int64, labels []string) string {
	if width < 2 || height < 2 {
		return ""
	}
	if len(totals) == 0 {
		return renderCentered(width, height, j.noDataMessage(width))
	}
	maxTotal := slices.Max(totals)
	if maxTotal == 0 {
		return renderCentered(width, height, j.noDataMessage(width))
	}

	showLabels := height >= 3
	chartHeight := height
	if showLabels {
		chartHeight--
	}
	chartHeight = max(chartHeight, 1)
	yLabels := buildValueYAxisLabels(maxTotal, chartHeight)
	labelWidth := maxLabelWidth(yLabels)
	chartWidth := max(width-labelWidth-1, 1)
	if chartWidth < 2 {
		return renderCentered(width, height, j.noDataMessage(width))
	}

	plotWidth := max(chartWidth-1, 1)
	series := remapSeries(totals, plotWidth)
	if len(series) == 0 {
		return renderCentered(width, height, j.noDataMessage(width))
	}
	maxVal := slices.Max(series)
	if maxVal == 0 {
		return renderCentered(width, height, j.noDataMessage(width))
	}

	maxHeight := float64(max(chartHeight-1, 1))
	scaled := make([]float64, len(series))
	for i, v := range series {
		scaled[i] = float64(v) * maxHeight / float64(maxVal)
	}

	canvasWidth := len(series) + 1
	c := canvas.New(canvasWidth, chartHeight, canvas.WithViewWidth(canvasWidth), canvas.WithViewHeight(chartHeight))
	axisStyle, barStyle := j.chartAxisStyle, j.chartBarStyle
	origin := canvas.Point{X: 0, Y: chartHeight - 1}
	graph.DrawXYAxis(&c, origin, axisStyle)
	baseline := max(chartHeight-2, 0)
	graph.DrawColumns(&c, canvas.Point{X: 1, Y: baseline}, scaled, barStyle)

	chartLines := strings.Split(c.View(), "\n")
	chartLines = applyYAxisLabels(chartLines, yLabels, labelWidth, j.styles.Muted)

	if showLabels {
		labelLine := j.styles.Muted.Render(buildBucketLabelLine(canvasWidth, labels))
		chartLines = append(chartLines, strings.Repeat(" ", labelWidth)+" "+labelLine)
	}
	return strings.Join(chartLines, "\n")
}

func (j *JobMetrics) renderScatter(width, height int, buckets []time.Time) string {
	if width < 2 || height < 2 {
		return ""
	}
	if len(buckets) == 0 {
		return renderCentered(width, height, j.noDataMessage(width))
	}

	// Use pre-processed data
	bucketCount := j.processed.bucketCount
	if bucketCount == 0 {
		return renderCentered(width, height, j.noDataMessage(width))
	}

	labels := sidekiq.MetricsHistogramLabels
	if len(labels) > bucketCount {
		labels = labels[:bucketCount]
	}

	points := j.processed.scatterPoints
	maxCount := j.processed.maxCount
	maxBucket := j.processed.maxBucket
	if len(points) == 0 || maxCount == 0 {
		return renderCentered(width, height, j.noDataMessage(width))
	}

	minX := 0.0
	maxX := float64(max(len(buckets)-1, 1))
	minY := 0.0
	if maxBucket < 0 {
		maxBucket = 0
	}
	if maxBucket < len(labels)-1 {
		labels = labels[:maxBucket+1]
	}
	maxY := float64(max(maxBucket, 1))
	yLabelWidth := maxLabelWidthFromSlice(labels)

	showLabels := height >= 3
	chartHeight := height
	chartHeight = max(chartHeight, 1)
	yStep := max(chartHeight/6, 1)
	xStep := 0
	if showLabels && chartHeight >= 2 {
		xStep = 1
	}
	axisStyle, labelStyle, pointStyle := j.scatterAxisStyle, j.scatterLabelStyle, j.scatterPointStyle
	lc := linechart.New(
		width, chartHeight,
		minX, maxX,
		minY, maxY,
		linechart.WithXYSteps(xStep, yStep),
		linechart.WithStyles(axisStyle, labelStyle, pointStyle),
		linechart.WithXLabelFormatter(func(_ int, _ float64) string { return "" }),
		linechart.WithYLabelFormatter(func(_ int, v float64) string {
			idx := clampInt(int(math.Round(v)), len(labels)-1)
			return labels[idx]
		}),
	)
	lc.DrawXYAxisAndLabel()

	sort.Slice(points, func(i, j int) bool {
		return points[i].Count < points[j].Count
	})

	for _, point := range points {
		lc.DrawRune(canvas.Float64Point{X: point.X, Y: point.Y}, scatterRune(point.Count, maxCount))
	}

	view := lc.View()
	chartLines := strings.Split(view, "\n")
	if showLabels {
		graphWidth := max(width-(yLabelWidth+1), 1)
		labelLine := buildBucketLabelLine(graphWidth+1, buildTimeBucketLabels(buckets))
		labelLine = strings.Repeat(" ", yLabelWidth) + j.styles.Muted.Render(labelLine)
		if xStep > 0 && len(chartLines) > 0 {
			chartLines[len(chartLines)-1] = labelLine
		} else {
			chartLines = append(chartLines, labelLine)
		}
	}
	return strings.Join(chartLines, "\n")
}

func (j *JobMetrics) initChartStyles() {
	j.chartAxisStyle = j.styles.ChartAxis
	j.chartBarStyle = j.styles.ChartHistogram
	j.scatterAxisStyle = j.styles.ChartAxis
	j.scatterLabelStyle = j.styles.ChartLabel
	j.scatterPointStyle = j.styles.ChartHistogram
}

type scatterPoint struct {
	X     float64
	Y     float64
	Count int64
}

// processedMetrics holds pre-computed data from histogram to avoid repeated processing in View().
type processedMetrics struct {
	sortedBuckets []time.Time    // Time buckets sorted chronologically
	bucketTotals  []int64        // Histogram totals per bucket (reversed for chart)
	scatterPoints []scatterPoint // Pre-built scatter points
	maxCount      int64          // Maximum count for scatter point sizing
	maxBucket     int            // Maximum bucket index with data
	bucketCount   int            // Number of histogram buckets
}

// processHistogramData performs single-pass processing of histogram data.
// Returns all computed values needed for rendering charts.
func processHistogramData(hist map[string][]int64, bucketCount int) processedMetrics {
	if len(hist) == 0 || bucketCount == 0 {
		return processedMetrics{}
	}

	// First pass: parse times, collect entries, and count non-zero values for pre-allocation
	type bucketEntry struct {
		time   time.Time
		values []int64
	}
	entries := make([]bucketEntry, 0, len(hist))
	nonZeroCount := 0

	for key, values := range hist {
		t, err := time.Parse(time.RFC3339, key)
		if err != nil {
			continue
		}
		entries = append(entries, bucketEntry{time: t, values: values})
		for _, count := range values {
			if count > 0 {
				nonZeroCount++
			}
		}
	}

	// Sort by time
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].time.Before(entries[j].time)
	})

	// Pre-allocate with exact/known capacities
	result := processedMetrics{
		sortedBuckets: make([]time.Time, 0, len(entries)),
		bucketTotals:  make([]int64, bucketCount),
		scatterPoints: make([]scatterPoint, 0, nonZeroCount),
		bucketCount:   bucketCount,
		maxBucket:     -1,
	}

	// Second pass: compute totals and scatter points (now in sorted order)
	for tIdx, entry := range entries {
		result.sortedBuckets = append(result.sortedBuckets, entry.time)

		for bIdx, count := range entry.values {
			if bIdx >= bucketCount {
				break
			}
			result.bucketTotals[bIdx] += count

			if count == 0 {
				continue
			}
			if count > result.maxCount {
				result.maxCount = count
			}
			bucketIdx := bucketCount - 1 - bIdx
			if bucketIdx > result.maxBucket {
				result.maxBucket = bucketIdx
			}
			result.scatterPoints = append(result.scatterPoints, scatterPoint{
				X:     float64(tIdx),
				Y:     float64(bucketIdx),
				Count: count,
			})
		}
	}

	// Reverse totals for chart display (matches original behavior)
	slices.Reverse(result.bucketTotals)

	return result
}

func axisMap(total, target int) []int {
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

func remapSeries(values []int64, target int) []int64 {
	if len(values) == 0 || target <= 0 {
		return nil
	}
	mapping := axisMap(len(values), target)
	out := make([]int64, target)
	for i, v := range values {
		out[mapping[i]] += v
	}
	return out
}

func buildValueYAxisLabels(maxVal int64, height int) map[int]string {
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

func maxLabelWidth(labels map[int]string) int {
	maxWidth := 0
	for _, label := range labels {
		labelWidth := lipgloss.Width(label)
		if labelWidth > maxWidth {
			maxWidth = labelWidth
		}
	}
	return maxWidth
}

func maxLabelWidthFromSlice(labels []string) int {
	maxWidth := 0
	for _, label := range labels {
		labelWidth := lipgloss.Width(label)
		if labelWidth > maxWidth {
			maxWidth = labelWidth
		}
	}
	return maxWidth
}

func applyYAxisLabels(lines []string, labels map[int]string, width int, style lipgloss.Style) []string {
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

func scatterRune(count, maxCount int64) rune {
	levels := []rune{'·', '◦', '•', '◯', '◉', '●'}
	if maxCount <= 0 {
		return levels[0]
	}
	if count < 0 {
		count = 0
	}
	ratio := math.Log1p(float64(count)) / math.Log1p(float64(maxCount))
	idx := clampInt(int(math.Round(ratio*float64(len(levels)-1))), len(levels)-1)
	return levels[idx]
}

func buildBucketLabelLine(width int, labels []string) string {
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
	positions := axisMap(len(labels), plotWidth)
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

func buildTimeBucketLabels(buckets []time.Time) []string {
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

	// Handle multi-line content
	contentLines := strings.Split(value, "\n")
	contentHeight := len(contentLines)
	startLine := max((height-contentHeight)/2, 0)

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
