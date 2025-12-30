package views

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/filterinput"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// errorsDataMsg carries error summary data internally.
type errorsDataMsg struct {
	rows       []errorSummaryRow
	groups     map[errorSummaryKey][]errorGroupJob
	deadCount  int
	retryCount int
}

type errorSummaryRow struct {
	displayClass string
	errorClass   string
	queue        string
	count        int64
	errorMessage string
}

type errorGroupJob struct {
	entry  *sidekiq.SortedEntry
	source string
}

type errorsState int

const (
	errorsStateSummary errorsState = iota
	errorsStateGroup
)

// Errors shows a summary of errors grouped by job and error class.
type Errors struct {
	client     *sidekiq.Client
	width      int
	height     int
	styles     Styles
	rows       []errorSummaryRow
	groups     map[errorSummaryKey][]errorGroupJob
	groupJobs  []errorGroupJob
	table      table.Model
	ready      bool
	deadCount  int
	retryCount int
	filter     filterinput.Model
	state      errorsState
	groupKey   errorSummaryKey
}

// NewErrors creates a new Errors view.
func NewErrors(client *sidekiq.Client) *Errors {
	return &Errors{
		client: client,
		filter: filterinput.New(),
		table: table.New(
			table.WithColumns(errorsSummaryColumns),
			table.WithEmptyMessage("No errors"),
		),
	}
}

// fetchDataCmd fetches dead and retry jobs and builds summary data.
func (e *Errors) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		deadJobs, retryJobs, err := e.fetchJobs(ctx)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		rows, groups := buildErrorSummary(deadJobs, retryJobs)

		return errorsDataMsg{
			rows:       rows,
			groups:     groups,
			deadCount:  len(deadJobs),
			retryCount: len(retryJobs),
		}
	}
}

func (e *Errors) fetchJobs(ctx context.Context) ([]*sidekiq.SortedEntry, []*sidekiq.SortedEntry, error) {
	query := e.filter.Query()
	if query != "" {
		deadJobs, err := e.client.ScanDeadJobs(ctx, query)
		if err != nil {
			return nil, nil, err
		}

		retryJobs, err := e.client.ScanRetryJobs(ctx, query)
		if err != nil {
			return nil, nil, err
		}

		return deadJobs, retryJobs, nil
	}

	deadJobs, _, err := e.client.GetDeadJobs(ctx, 0, -1)
	if err != nil {
		return nil, nil, err
	}

	retryJobs, _, err := e.client.GetRetryJobs(ctx, 0, -1)
	if err != nil {
		return nil, nil, err
	}

	return deadJobs, retryJobs, nil
}

// Init implements View.
func (e *Errors) Init() tea.Cmd {
	e.reset()
	e.filter.Init()
	return e.fetchDataCmd()
}

// Update implements View.
func (e *Errors) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case errorsDataMsg:
		e.rows = msg.rows
		e.groups = msg.groups
		e.deadCount = msg.deadCount
		e.retryCount = msg.retryCount
		e.ready = true
		switch e.state {
		case errorsStateGroup:
			e.refreshGroupRows()
		case errorsStateSummary:
			e.updateTableRows()
		}
		return e, nil

	case RefreshMsg:
		return e, e.fetchDataCmd()

	case filterinput.ActionMsg:
		if msg.Action != filterinput.ActionNone {
			e.state = errorsStateSummary
			e.groupJobs = nil
			e.table.SetCursor(0)
			return e, e.fetchDataCmd()
		}
		return e, nil

	case tea.KeyMsg:
		wasFocused := e.filter.Focused()
		var cmd tea.Cmd
		e.filter, cmd = e.filter.Update(msg)
		if wasFocused || msg.String() == "/" || msg.String() == "ctrl+u" || (msg.String() == "esc" && e.state != errorsStateGroup) {
			return e, cmd
		}

		switch e.state {
		case errorsStateGroup:
			switch msg.String() {
			case "esc":
				e.state = errorsStateSummary
				e.groupJobs = nil
				e.updateTableRows()
				return e, cmd
			case "enter":
				if idx := e.table.Cursor(); idx >= 0 && idx < len(e.groupJobs) {
					job := e.groupJobs[idx]
					if job.entry != nil {
						return e, func() tea.Msg {
							return ShowJobDetailMsg{Job: job.entry.JobRecord}
						}
					}
				}
				return e, nil
			}

			e.table, _ = e.table.Update(msg)
			return e, nil
		case errorsStateSummary:
			switch msg.String() {
			case "enter":
				if idx := e.table.Cursor(); idx >= 0 && idx < len(e.rows) {
					row := e.rows[idx]
					e.groupKey = errorSummaryKey{
						displayClass: row.displayClass,
						errorClass:   row.errorClass,
						queue:        row.queue,
					}
					e.state = errorsStateGroup
					e.refreshGroupRows()
				}
				return e, nil
			}
		}

		e.table, _ = e.table.Update(msg)
		return e, nil
	}

	return e, nil
}

// View implements View.
func (e *Errors) View() string {
	if !e.ready {
		return e.renderMessage("Loading...")
	}

	total := e.deadCount + e.retryCount
	if len(e.rows) == 0 && total == 0 && e.filter.Query() == "" && !e.filter.Focused() {
		return e.renderMessage("No errors")
	}

	return e.renderSummaryBox()
}

func (e *Errors) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  e.styles.Title,
		Muted:  e.styles.Muted,
		Border: e.styles.FocusBorder,
	}, "Errors", msg, e.width, e.height)
}

// Name implements View.
func (e *Errors) Name() string {
	return "Errors"
}

// ShortHelp implements View.
func (e *Errors) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View.
func (e *Errors) SetSize(width, height int) View {
	e.width = width
	e.height = height
	e.updateTableSize()
	return e
}

func (e *Errors) reset() {
	e.ready = false
	e.state = errorsStateSummary
	e.groupJobs = nil
	e.groupKey = errorSummaryKey{}
	e.rows = nil
	e.groups = nil
	e.deadCount = 0
	e.retryCount = 0
	e.table.SetRows(nil)
	e.table.SetCursor(0)
}

// Dispose clears cached data when the view is removed from the stack.
func (e *Errors) Dispose() {
	e.reset()
	e.filter = filterinput.New()
	e.SetStyles(e.styles)
	e.updateTableSize()
}

// FilterFocused reports whether the filter input is capturing keys.
func (e *Errors) FilterFocused() bool {
	return e.filter.Focused()
}

// SetStyles implements View.
func (e *Errors) SetStyles(styles Styles) View {
	e.styles = styles
	e.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	e.filter.SetStyles(filterinput.Styles{
		Prompt:      styles.MetricLabel,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	})
	return e
}

var errorsSummaryColumns = []table.Column{
	{Title: "Job", Width: 30},
	{Title: "Error", Width: 40},
	{Title: "Queue", Width: 15},
	{Title: "Count", Width: 7},
	{Title: "Message", Width: 60},
}

var errorGroupColumns = []table.Column{
	{Title: "Set", Width: 5},
	{Title: "At", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Message", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (e *Errors) updateTableSize() {
	tableHeight := max(e.height-3, 3)
	tableWidth := e.width - 4
	e.table.SetSize(tableWidth, tableHeight)
	e.filter.SetWidth(tableWidth)
}

// updateTableRows converts summary data to table rows.
func (e *Errors) updateTableRows() {
	if e.filter.Query() != "" {
		e.table.SetEmptyMessage("No matches")
	} else {
		e.table.SetEmptyMessage("No errors")
	}

	e.table.SetColumns(errorsSummaryColumns)

	rows := make([]table.Row, 0, len(e.rows))
	for _, row := range e.rows {
		rows = append(rows, table.Row{
			row.displayClass,
			row.errorClass,
			row.queue,
			format.Number(row.count),
			row.errorMessage,
		})
	}
	e.table.SetRows(rows)
	e.updateTableSize()
}

// renderSummaryBox renders the bordered box containing the summary table.
func (e *Errors) renderSummaryBox() string {
	content := e.filter.View() + "\n" + e.table.View()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  e.styles.Title,
				Border: e.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  e.styles.Title,
				Border: e.styles.BorderStyle,
			},
		}),
		frame.WithTitle("Errors"),
		frame.WithTitlePadding(0),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(e.width, e.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

type errorSummaryKey struct {
	displayClass string
	errorClass   string
	queue        string
}

func buildErrorSummary(deadJobs, retryJobs []*sidekiq.SortedEntry) ([]errorSummaryRow, map[errorSummaryKey][]errorGroupJob) {
	rowsByKey := make(map[errorSummaryKey]*errorSummaryRow)
	groups := make(map[errorSummaryKey][]errorGroupJob)

	addJobs := func(jobs []*sidekiq.SortedEntry, source string) {
		for _, job := range jobs {
			if job == nil || job.JobRecord == nil {
				continue
			}

			displayClass := strings.TrimSpace(job.DisplayClass())
			if displayClass == "" {
				displayClass = "unknown"
			}

			errorClass := strings.TrimSpace(job.ErrorClass())
			if errorClass == "" {
				errorClass = "unknown"
			}

			queue := strings.TrimSpace(job.Queue())
			if queue == "" {
				queue = "unknown"
			}

			errorMessage := errorMessageOnly(job)

			key := errorSummaryKey{
				displayClass: displayClass,
				errorClass:   errorClass,
				queue:        queue,
			}
			groups[key] = append(groups[key], errorGroupJob{
				entry:  job,
				source: source,
			})
			if row, ok := rowsByKey[key]; ok {
				row.count++
				continue
			}

			rowsByKey[key] = &errorSummaryRow{
				displayClass: displayClass,
				errorClass:   errorClass,
				queue:        queue,
				count:        1,
				errorMessage: errorMessage,
			}
		}
	}

	addJobs(deadJobs, "dead")
	addJobs(retryJobs, "retry")

	rows := make([]errorSummaryRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, *row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].displayClass != rows[j].displayClass {
			return rows[i].displayClass < rows[j].displayClass
		}
		if rows[i].errorClass != rows[j].errorClass {
			return rows[i].errorClass < rows[j].errorClass
		}
		if rows[i].queue != rows[j].queue {
			return rows[i].queue < rows[j].queue
		}
		return rows[i].errorMessage < rows[j].errorMessage
	})

	return rows, groups
}

func (e *Errors) refreshGroupRows() {
	e.table.SetColumns(errorGroupColumns)

	jobs, ok := e.groups[e.groupKey]
	if !ok {
		e.state = errorsStateSummary
		e.groupJobs = nil
		e.updateTableRows()
		return
	}

	e.groupJobs = jobs
	rows := make([]table.Row, 0, len(jobs))
	now := time.Now().Unix()
	for _, job := range jobs {
		if job.entry == nil {
			continue
		}
		when := format.Duration(now - job.entry.At())
		queue := job.entry.Queue()
		if queue == "" {
			queue = "unknown"
		}
		message := errorDisplay(job.entry)
		rows = append(rows, table.Row{
			job.source,
			when,
			queue,
			job.entry.DisplayClass(),
			format.Args(job.entry.DisplayArgs()),
			message,
		})
	}
	e.table.SetRows(rows)
	e.updateTableSize()
}

func errorDisplay(job *sidekiq.SortedEntry) string {
	if job == nil || job.JobRecord == nil {
		return ""
	}
	if !job.HasError() {
		return ""
	}
	errorStr := fmt.Sprintf("%s: %s", job.ErrorClass(), job.ErrorMessage())
	if len(errorStr) > 100 {
		errorStr = errorStr[:99] + "…"
	}
	return errorStr
}

func errorMessageOnly(job *sidekiq.SortedEntry) string {
	if job == nil || job.JobRecord == nil {
		return "unknown"
	}
	message := strings.TrimSpace(job.ErrorMessage())
	if message == "" {
		return "unknown"
	}
	return message
}
