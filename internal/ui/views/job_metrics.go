package views

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/linechart"
	oldgloss "github.com/charmbracelet/lipgloss"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/format"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
)

// jobMetricsDataMsg carries job metrics data.
type jobMetricsDataMsg struct {
	result sidekiq.MetricsJobDetailResult
}

// JobMetrics shows per-job execution metrics.
type JobMetrics struct {
	client *sidekiq.Client
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

	chartAxisStyle    oldgloss.Style
	chartBarStyle     oldgloss.Style
	scatterAxisStyle  oldgloss.Style
	scatterLabelStyle oldgloss.Style
	scatterPointStyle oldgloss.Style
}

// NewJobMetrics creates a new job metrics view.
func NewJobMetrics(client *sidekiq.Client) *JobMetrics {
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

	if !j.hasDetailData() {
		return messagebox.Render(messagebox.Styles{
			Title:  j.styles.Title,
			Muted:  j.styles.Muted,
			Border: j.styles.FocusBorder,
		}, title, "No job metrics found", j.width, j.height)
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
				Border: j.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  j.styles.Title,
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
				Border: j.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  j.styles.Title,
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
	if idx := indexOf(j.periods, period); idx >= 0 {
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
			params = sidekiq.MetricsPeriods[sidekiq.MetricsPeriodOrder[0]]
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

func (j *JobMetrics) hasDetailData() bool {
	if len(j.result.Hist) == 0 {
		return false
	}
	return j.result.Totals.Processed > 0 || j.result.Totals.Failed > 0
}

func (j *JobMetrics) detailMeta() string {
	if j.period == "" {
		return ""
	}
	return j.styles.MetricLabel.Render("period: ") + j.styles.MetricValue.Render(j.period)
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
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
	}
	maxTotal := slices.Max(totals)
	if maxTotal == 0 {
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
	}

	legend := j.renderBucketsLegend(width)
	showLegend := legend != "" && height >= 2
	showLabels := height >= 3
	chartHeight := height
	if showLegend {
		chartHeight--
	}
	if showLabels {
		chartHeight--
	}
	chartHeight = max(chartHeight, 1)
	yLabels := buildValueYAxisLabels(maxTotal, chartHeight)
	labelWidth := maxLabelWidth(yLabels)
	chartWidth := max(width-labelWidth-1, 1)
	if chartWidth < 2 {
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
	}

	plotWidth := max(chartWidth-1, 1)
	series := remapSeries(totals, plotWidth)
	if len(series) == 0 {
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
	}
	maxVal := slices.Max(series)
	if maxVal == 0 {
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
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
	if showLegend {
		chartLines = append(chartLines, legend)
	}
	return strings.Join(chartLines, "\n")
}

func (j *JobMetrics) renderScatter(width, height int, buckets []time.Time) string {
	if width < 2 || height < 2 {
		return ""
	}
	if len(buckets) == 0 {
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
	}

	// Use pre-processed data
	bucketCount := j.processed.bucketCount
	if bucketCount == 0 {
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
	}

	labels := sidekiq.MetricsHistogramLabels
	if len(labels) > bucketCount {
		labels = labels[:bucketCount]
	}

	points := j.processed.scatterPoints
	maxCount := j.processed.maxCount
	maxBucket := j.processed.maxBucket
	if len(points) == 0 || maxCount == 0 {
		return renderCentered(width, height, j.styles.Muted.Render("No data"))
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

	legend := j.renderScatterLegend(width)
	showLegend := legend != "" && height >= 2
	showLabels := height >= 3
	chartHeight := height
	if showLegend {
		chartHeight--
	}
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
	if showLegend {
		chartLines = append(chartLines, legend)
	}
	return strings.Join(chartLines, "\n")
}

func (j *JobMetrics) initChartStyles() {
	j.chartAxisStyle = oldgloss.NewStyle().Foreground(adaptiveColor(theme.DefaultTheme.TextMuted))
	j.chartBarStyle = oldgloss.NewStyle().Foreground(adaptiveColor(theme.DefaultTheme.Primary))
	j.scatterAxisStyle = oldgloss.NewStyle().Foreground(adaptiveColor(theme.DefaultTheme.TextMuted))
	j.scatterLabelStyle = oldgloss.NewStyle().Foreground(adaptiveColor(theme.DefaultTheme.TextMuted))
	j.scatterPointStyle = oldgloss.NewStyle().Foreground(adaptiveColor(theme.DefaultTheme.Primary))
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

func (j *JobMetrics) renderBucketsLegend(width int) string {
	if width < 1 {
		return ""
	}
	sep := j.styles.Muted.Render(" | ")
	success := j.styles.MetricLabel.Render("Success: ") + j.styles.MetricValue.Render(format.ShortNumber(j.result.Totals.Success()))
	failed := j.styles.MetricLabel.Render("Failed: ") + j.styles.MetricValue.Render(format.ShortNumber(j.result.Totals.Failed))
	avg := j.styles.MetricLabel.Render("Avg: ") + j.styles.MetricValue.Render(fmt.Sprintf("%.2fs", j.result.Totals.AvgSeconds()))
	line := success + sep + failed + sep + avg
	return maxWidthStyle.MaxWidth(width).Render(line)
}

func (j *JobMetrics) renderScatterLegend(width int) string {
	if width < 1 {
		return ""
	}
	rangeText := formatMetricsRange(j.result.StartsAt, j.result.EndsAt)
	if rangeText == "" {
		return ""
	}
	line := j.styles.MetricLabel.Render("Range: ") + j.styles.MetricValue.Render(rangeText)
	return maxWidthStyle.MaxWidth(width).Render(line)
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

func adaptiveColor(color compat.CompleteAdaptiveColor) oldgloss.AdaptiveColor {
	return oldgloss.AdaptiveColor{
		Light: colorToHex(color.Light.TrueColor),
		Dark:  colorToHex(color.Dark.TrueColor),
	}
}

func colorToHex(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
