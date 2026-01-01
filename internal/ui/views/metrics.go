package views

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/filterinput"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
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
	client *sidekiq.Client
	width  int
	height int
	styles Styles

	ready     bool
	result    sidekiq.MetricsTopJobsResult
	rows      []metricsRow
	periods   []string
	period    string
	periodIdx int
	filter    filterinput.Model
	table     table.Model
}

// NewMetrics creates a new Metrics view.
func NewMetrics(client *sidekiq.Client) *Metrics {
	periods := append([]string{}, sidekiq.MetricsPeriodOrder...)

	m := &Metrics{
		client:  client,
		periods: periods,
		period:  periods[0],
		filter:  filterinput.New(),
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
	m.filter.Init()
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

	case filterinput.ActionMsg:
		if msg.Action != filterinput.ActionNone {
			m.table.SetCursor(0)
			return m, m.fetchListCmd()
		}
		return m, nil

	case tea.KeyMsg:
		wasFocused := m.filter.Focused()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if wasFocused || msg.String() == "/" || msg.String() == "ctrl+u" || msg.String() == "esc" {
			return m, cmd
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
	content := m.filter.View() + "\n" + m.table.View()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.Title,
				Border: m.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.Title,
				Border: m.styles.BorderStyle,
			},
		}),
		frame.WithTitle("Metrics"),
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

// FilterFocused reports whether the filter input is capturing keys.
func (m *Metrics) FilterFocused() bool {
	return m.filter.Focused()
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
	m.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	m.filter.SetStyles(filterinput.Styles{
		Prompt:      styles.MetricLabel,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	})
	return m
}

var metricsColumns = []table.Column{
	{Title: "Job", Width: 36},
	{Title: "Success", Width: 9},
	{Title: "Failure", Width: 8},
	{Title: "Total (s)", Width: 10},
	{Title: "Avg (s)", Width: 9},
}

func (m *Metrics) fetchListCmd() tea.Cmd {
	period := m.period
	filter := m.filter.Query()
	return func() tea.Msg {
		ctx := context.Background()
		params, ok := sidekiq.MetricsPeriods[period]
		if !ok {
			params = sidekiq.MetricsPeriods[sidekiq.MetricsPeriodOrder[0]]
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
	if m.filter.Query() != "" {
		m.table.SetEmptyMessage("No matches")
	} else {
		m.table.SetEmptyMessage("No recent metrics")
	}

	rows := make([]table.Row, 0, len(m.rows))
	for _, row := range m.rows {
		success := format.Number(row.totals.Success())
		failure := format.Number(row.totals.Failed)
		totalSecs := fmt.Sprintf("%.2f", row.totals.Seconds)
		avgSecs := fmt.Sprintf("%.2f", row.totals.AvgSeconds())
		rows = append(rows, table.Row{
			ID: row.class,
			Cells: []string{
				row.class,
				success,
				failure,
				totalSecs,
				avgSecs,
			},
		})
	}

	m.table.SetRows(rows)
	m.updateTableSize()
}

func (m *Metrics) updateTableSize() {
	tableWidth := m.width - 4
	tableHeight := max(m.height-3, 3)
	m.table.SetSize(tableWidth, tableHeight)
	m.filter.SetWidth(tableWidth)
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
