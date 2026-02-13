package views

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

// metricsListMsg carries list metrics data.
type metricsListMsg struct {
	result  sidekiq.MetricsTopJobsResult
	periods []string
	period  string
}

type metricsRow struct {
	class  string
	totals sidekiq.MetricsJobTotals
}

// Metrics shows job execution metrics.
type Metrics struct {
	client sidekiq.API
	width  int
	height int
	styles Styles

	ready       bool
	result      sidekiq.MetricsTopJobsResult
	rows        []metricsRow
	resetScroll bool
	periods     []string
	period      string
	periodIdx   int
	filter      string
	frameStyles frame.Styles
	filterStyle filterdialog.Styles
	table       table.Model
}

// NewMetrics creates a new Metrics view.
func NewMetrics(client sidekiq.API) *Metrics {
	return &Metrics{
		client:  client,
		periods: sidekiq.MetricsPeriodOrder,
		period:  sidekiq.MetricsPeriodOrder[0],
		table: table.New(
			table.WithColumns(metricsColumns),
			table.WithEmptyMessage("No recent metrics"),
		),
	}
}

// Init implements View.
func (m *Metrics) Init() tea.Cmd {
	m.ready = false
	m.resetScroll = true
	m.table.GotoTop()
	return m.fetchListCmd()
}

// Update implements View.
func (m *Metrics) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case metricsListMsg:
		m.applyPeriodState(msg.periods, msg.period)
		m.result = msg.result
		m.ready = true
		m.buildListRows()
		if m.resetScroll {
			m.resetScroll = false
			m.table.GotoTop()
		}
		return m, nil

	case RefreshMsg:
		return m, m.fetchListCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone || msg.Query == m.filter {
			return m, nil
		}
		return m, m.setFilterAndReload(msg.Query)

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return m, m.openFilterDialog()
		case "ctrl+u":
			if m.filter == "" {
				return m, nil
			}
			return m, m.setFilterAndReload("")
		case "alt+left", "[":
			m.movePage(-1)
			return m, nil
		case "alt+right", "]":
			m.movePage(1)
			return m, nil
		case "enter":
			if selected, ok := m.selectedRow(); ok {
				return m, func() tea.Msg {
					return ShowJobMetricsMsg{Job: selected.class, Period: m.period}
				}
			}
			return m, nil
		case "{":
			return m.adjustPeriod(-1)
		case "}":
			return m.adjustPeriod(1)
		}

		m.table, _ = m.table.Update(msg)
		return m, nil
	}

	return m, nil
}

// View implements View.
func (m *Metrics) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	if !m.ready {
		return messagebox.Render(messagebox.Styles{
			Title:  m.styles.Title,
			Muted:  m.styles.Muted,
			Border: m.styles.FocusBorder,
		}, "Metrics", "Loading...", m.width, m.height)
	}

	meta := m.listMeta()
	content := m.table.View()

	box := frame.New(
		frame.WithStyles(m.frameStyles),
		frame.WithTitle("Metrics"),
		frame.WithFilter(m.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(m.width, m.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

// Name implements View.
func (m *Metrics) Name() string {
	return "Metrics"
}

// ShortHelp implements View.
func (m *Metrics) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (m *Metrics) ContextItems() []ContextItem {
	rangeText := "-"
	jobsText := "-"
	succeededText := "-"
	failedText := "-"
	if m.ready {
		if value := formatMetricsRange(m.result.StartsAt, m.result.EndsAt); value != "" {
			rangeText = value
		}
		jobs, succeeded, failed := m.aggregateTotals()
		jobsText = display.Number(jobs)
		succeededText = display.Number(succeeded)
		failedText = display.Number(failed)
	}

	return []ContextItem{
		{Label: "Jobs", Value: jobsText},
		{Label: "Succeeded", Value: succeededText},
		{Label: "Failed", Value: failedText},
		{Label: "Range", Value: rangeText},
	}
}

// HintBindings implements HintProvider.
func (m *Metrics) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"{", "}"}, "{ ⋰ }", "change period"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
		helpBinding([]string{"enter"}, "enter", "job metrics"),
	}
}

// HelpSections implements HelpProvider.
func (m *Metrics) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Metrics",
			Bindings: []key.Binding{
				helpBinding([]string{"/"}, "/", "filter"),
				helpBinding([]string{"ctrl+u"}, "ctrl+u", "clear filter"),
				helpBinding([]string{"{"}, "{", "previous period"),
				helpBinding([]string{"}"}, "}", "next period"),
				helpBinding([]string{"["}, "[", "page up"),
				helpBinding([]string{"]"}, "]", "page down"),
				helpBinding([]string{"enter"}, "enter", "job metrics"),
			},
		},
	}
}

// TableHelp implements TableHelpProvider.
func (m *Metrics) TableHelp() []key.Binding {
	return tableHelpBindings(m.table.KeyMap)
}

// SetSize implements View.
func (m *Metrics) SetSize(width, height int) View {
	m.width = width
	m.height = height
	m.updateTableSize()
	return m
}

// SetStyles implements View.
func (m *Metrics) SetStyles(styles Styles) View {
	m.styles = styles
	m.frameStyles = frameStylesFromTheme(styles)
	m.filterStyle = filterDialogStylesFromTheme(styles)
	m.table.SetStyles(tableStylesFromTheme(styles))
	return m
}

var metricsColumns = []table.Column{
	{Title: "Job", Width: 36},
	{Title: "Success", Width: 12, Align: table.AlignRight},
	{Title: "Failure", Width: 12, Align: table.AlignRight},
	{Title: "Total (s)", Width: 12, Align: table.AlignRight},
	{Title: "Avg (s)", Width: 12, Align: table.AlignRight},
	{Title: "", Width: 0},
}

func (m *Metrics) fetchListCmd() tea.Cmd {
	period := m.period
	filter := m.filter
	client := m.client
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "metrics.fetchListCmd")

		periods := normalizeMetricsPeriods(client.MetricsPeriodOrder(ctx))
		queryPeriod := period
		if !slices.Contains(periods, queryPeriod) {
			queryPeriod = periods[0]
		}

		params := sidekiq.MetricsPeriods[queryPeriod]
		result, err := client.GetMetricsTopJobs(ctx, params, filter)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return metricsListMsg{
			result:  result,
			periods: periods,
			period:  queryPeriod,
		}
	}
}

func (m *Metrics) applyPeriodState(periods []string, selected string) {
	m.periods = normalizeMetricsPeriods(periods)

	idx := slices.Index(m.periods, selected)
	if idx < 0 {
		idx = slices.Index(m.periods, m.period)
	}
	if idx < 0 {
		idx = 0
	}

	m.periodIdx = idx
	m.period = m.periods[idx]
}

func normalizeMetricsPeriods(periods []string) []string {
	if len(periods) == 0 {
		return append([]string(nil), sidekiq.MetricsPeriodOrder...)
	}

	normalized := make([]string, 0, len(periods))
	seen := make(map[string]struct{}, len(periods))
	for _, period := range periods {
		if _, ok := sidekiq.MetricsPeriods[period]; !ok {
			continue
		}
		if _, ok := seen[period]; ok {
			continue
		}
		seen[period] = struct{}{}
		normalized = append(normalized, period)
	}

	if len(normalized) == 0 {
		return append([]string(nil), sidekiq.MetricsPeriodOrder...)
	}

	return normalized
}

func (m *Metrics) adjustPeriod(delta int) (View, tea.Cmd) {
	next := mathutil.Clamp(m.periodIdx+delta, 0, len(m.periods)-1)
	if next == m.periodIdx {
		return m, nil
	}
	m.periodIdx = next
	m.period = m.periods[next]
	m.resetScroll = true
	m.table.GotoTop()
	return m, m.fetchListCmd()
}

func (m *Metrics) buildListRows() {
	rows := make([]metricsRow, 0, len(m.result.Jobs))
	for className, totals := range m.result.Jobs {
		rows = append(rows, metricsRow{class: className, totals: totals})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].totals.Seconds > rows[j].totals.Seconds
	})

	m.rows = rows
	m.updateTableRows()
}

func (m *Metrics) updateTableRows() {
	if m.filter != "" {
		m.table.SetEmptyMessage("No matches")
	} else {
		m.table.SetEmptyMessage("No recent metrics")
	}

	if len(m.rows) == 0 {
		m.table.SetRows(nil)
		m.updateTableSize()
		return
	}

	rows := make([]table.Row, len(m.rows))
	for i, row := range m.rows {
		rows[i] = table.Row{
			ID: row.class,
			Cells: []string{
				row.class,
				display.Number(row.totals.Success()),
				display.Number(row.totals.Failed),
				display.Float(row.totals.Seconds, 2),
				display.Float(row.totals.AvgSeconds(), 2),
				"",
			},
		}
	}

	m.table.SetRows(rows)
	m.updateTableSize()
}

func (m *Metrics) selectedRow() (metricsRow, bool) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.rows) {
		return metricsRow{}, false
	}
	return m.rows[idx], true
}

func (m *Metrics) movePage(delta int) {
	step := max(m.table.ViewportHeight()-1, 1)
	if delta < 0 {
		m.table.MoveUp(step)
	} else if delta > 0 {
		m.table.MoveDown(step)
	}
}

func (m *Metrics) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(m.filterStyle),
				filterdialog.WithQuery(m.filter),
			),
		}
	}
}

func (m *Metrics) setFilterAndReload(filter string) tea.Cmd {
	m.filter = filter
	m.resetScroll = true
	m.table.GotoTop()
	return m.fetchListCmd()
}

func (m *Metrics) updateTableSize() {
	tableWidth, tableHeight := framedTableSize(m.width, m.height)
	m.table.SetSize(tableWidth, tableHeight)
}

func (m *Metrics) listMeta() string {
	if m.period == "" {
		return ""
	}
	return m.styles.MetricLabel.Render("period: ") + m.styles.MetricValue.Render(m.period)
}

func (m *Metrics) aggregateTotals() (int64, int64, int64) {
	var processed int64
	var failed int64
	for _, totals := range m.result.Jobs {
		processed += totals.Processed
		failed += totals.Failed
	}
	succeeded := max(processed-failed, 0)
	return int64(len(m.result.Jobs)), succeeded, failed
}

func formatMetricsRange(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return ""
	}

	start = start.UTC()
	end = end.UTC()
	if start.Format("2006-01-02") == end.Format("2006-01-02") {
		return fmt.Sprintf("%s-%s UTC", start.Format("15:04"), end.Format("15:04"))
	}

	return fmt.Sprintf("%s-%s UTC", start.Format("Jan 2 15:04"), end.Format("Jan 2 15:04"))
}
