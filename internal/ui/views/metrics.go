package views

import (
	"context"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

const metricsTableLimit = 20

// metricsListMsg carries list metrics data.
type metricsListMsg struct {
	result sidekiq.MetricsTopJobsResult
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
	m := &Metrics{
		client:  client,
		periods: sidekiq.MetricsPeriodOrder,
		period:  sidekiq.MetricsPeriodOrder[0],
		table: table.New(
			table.WithColumns(metricsColumns),
			table.WithEmptyMessage("No recent metrics"),
		),
	}

	return m
}

// Init implements View.
func (m *Metrics) Init() tea.Cmd {
	m.ready = false
	m.table.SetCursor(0)
	return m.fetchListCmd()
}

// Update implements View.
func (m *Metrics) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case metricsListMsg:
		m.result = msg.result
		m.ready = true
		m.buildListRows()
		return m, nil

	case RefreshMsg:
		return m, m.fetchListCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return m, nil
		}
		if msg.Query == m.filter {
			return m, nil
		}
		m.filter = msg.Query
		m.table.SetCursor(0)
		return m, m.fetchListCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return m, m.openFilterDialog()
		case "ctrl+u":
			if m.filter != "" {
				m.filter = ""
				m.table.SetCursor(0)
				return m, m.fetchListCmd()
			}
			return m, nil
		}

		switch msg.String() {
		case "enter":
			if idx := m.table.Cursor(); idx >= 0 && idx < len(m.rows) {
				selected := m.rows[idx]
				return m, func() tea.Msg {
					return ShowJobMetricsMsg{Job: selected.class, Period: m.period}
				}
			}
			return m, nil
		case "[":
			return m.adjustPeriod(-1)
		case "]":
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
	m.frameStyles = frame.Styles{
		Focused: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterFocused,
			Border: styles.FocusBorder,
		},
		Blurred: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterBlurred,
			Border: styles.BorderStyle,
		},
	}
	m.filterStyle = filterdialog.Styles{
		Title:       styles.Title,
		Border:      styles.FocusBorder,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	}
	m.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	return m
}

var metricsColumns = []table.Column{
	{Title: "Job", Width: 36},
	{Title: "Success", Width: 12},
	{Title: "Failure", Width: 12},
	{Title: "Total (s)", Width: 12},
	{Title: "Avg (s)", Width: 12},
}

func (m *Metrics) fetchListCmd() tea.Cmd {
	period := m.period
	filter := m.filter
	return func() tea.Msg {
		ctx := context.Background()

		// Update periods based on detected Sidekiq version
		m.periods = m.client.MetricsPeriodOrder(ctx)
		if m.periodIdx >= len(m.periods) {
			m.periodIdx = len(m.periods) - 1
			m.period = m.periods[m.periodIdx]
		}

		params, ok := sidekiq.MetricsPeriods[period]
		if !ok {
			params = sidekiq.MetricsPeriods[m.periods[0]]
		}
		result, err := m.client.GetMetricsTopJobs(ctx, params, filter)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return metricsListMsg{result: result}
	}
}

func (m *Metrics) adjustPeriod(delta int) (View, tea.Cmd) {
	next := clampInt(m.periodIdx+delta, len(m.periods)-1)
	if next == m.periodIdx {
		return m, nil
	}
	m.periodIdx = next
	m.period = m.periods[next]
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

	if len(rows) > metricsTableLimit {
		rows = rows[:metricsTableLimit]
	}

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

	numericCellStyle := lipgloss.NewStyle().Align(lipgloss.Right)

	// First pass: format values into table rows and track max widths
	rows := make([]table.Row, len(m.rows))

	maxWidths := make([]int, len(metricsColumns))
	for i, row := range m.rows {
		success := format.Number(row.totals.Success())
		failure := format.Number(row.totals.Failed)
		totalSec := format.Float(row.totals.Seconds, 2)
		avgSec := format.Float(row.totals.AvgSeconds(), 2)

		rows[i] = table.Row{
			ID: row.class,
			Cells: []string{
				row.class,
				success,
				failure,
				totalSec,
				avgSec,
			},
		}

		for j := range metricsColumns {
			maxWidths[j] = max(metricsColumns[j].Width, maxWidths[j], len(rows[i].Cells[j]))
		}
	}

	// Second pass: apply right-alignment styling to headers and cells
	m.table.SetColumns([]table.Column{
		{Title: "Job", Width: maxWidths[0]},
		{Title: numericCellStyle.Width(maxWidths[1]).Render(metricsColumns[1].Title), Width: maxWidths[1] + 1},
		{Title: numericCellStyle.Width(maxWidths[2]).Render(metricsColumns[2].Title), Width: maxWidths[2] + 1},
		{Title: numericCellStyle.Width(maxWidths[3]).Render(metricsColumns[3].Title), Width: maxWidths[3] + 1},
		{Title: numericCellStyle.Width(maxWidths[4]).Render(metricsColumns[4].Title), Width: maxWidths[4] + 1},
	})

	for i := range rows {
		for j := 1; j < len(metricsColumns); j++ {
			rows[i].Cells[j] = numericCellStyle.Width(maxWidths[j]).Render(rows[i].Cells[j])
		}
	}

	m.table.SetRows(rows)
	m.updateTableSize()
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

func (m *Metrics) updateTableSize() {
	tableWidth := m.width - 4
	tableHeight := max(m.height-2, 3)
	m.table.SetSize(tableWidth, tableHeight)
}

func (m *Metrics) listMeta() string {
	sep := m.styles.Muted.Render(" • ")
	period := m.styles.MetricLabel.Render("period: ") + m.styles.MetricValue.Render(m.period)
	entries := []string{period}
	if rangeText := formatMetricsRange(m.result.StartsAt, m.result.EndsAt); rangeText != "" {
		entries = append(entries, m.styles.MetricLabel.Render("range: ")+m.styles.MetricValue.Render(rangeText))
	}
	return strings.Join(entries, sep)
}
