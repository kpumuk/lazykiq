package views

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/filterinput"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// errorsSummaryDataMsg carries error summary data internally.
type errorsSummaryDataMsg struct {
	rows       []errorSummaryRow
	deadCount  int
	retryCount int
}

// ErrorsSummary shows a summary of errors grouped by job and error class.
type ErrorsSummary struct {
	client     sidekiq.API
	width      int
	height     int
	styles     Styles
	rows       []errorSummaryRow
	table      table.Model
	ready      bool
	deadCount  int
	retryCount int
	filter     filterinput.Model
}

// NewErrorsSummary creates a new ErrorsSummary view.
func NewErrorsSummary(client sidekiq.API) *ErrorsSummary {
	return &ErrorsSummary{
		client: client,
		filter: filterinput.New(),
		table: table.New(
			table.WithColumns(errorsSummaryColumns),
			table.WithEmptyMessage("No errors"),
		),
	}
}

// Init implements View.
func (e *ErrorsSummary) Init() tea.Cmd {
	e.reset()
	e.filter.Init()
	return e.fetchDataCmd()
}

// Update implements View.
func (e *ErrorsSummary) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case errorsSummaryDataMsg:
		e.rows = msg.rows
		e.deadCount = msg.deadCount
		e.retryCount = msg.retryCount
		e.ready = true
		e.updateTableRows()
		return e, nil

	case RefreshMsg:
		return e, e.fetchDataCmd()

	case filterinput.ActionMsg:
		if msg.Action != filterinput.ActionNone {
			e.table.SetCursor(0)
			return e, e.fetchDataCmd()
		}
		return e, nil

	case tea.KeyMsg:
		wasFocused := e.filter.Focused()
		var cmd tea.Cmd
		e.filter, cmd = e.filter.Update(msg)
		if wasFocused || msg.String() == "/" || msg.String() == "ctrl+u" || msg.String() == "esc" {
			return e, cmd
		}

		switch msg.String() {
		case "enter":
			if idx := e.table.Cursor(); idx >= 0 && idx < len(e.rows) {
				row := e.rows[idx]
				return e, func() tea.Msg {
					return ShowErrorDetailsMsg{
						DisplayClass: row.displayClass,
						ErrorClass:   row.errorClass,
						Queue:        row.queue,
						Query:        e.filter.Query(),
					}
				}
			}
			return e, nil
		}

		e.table, _ = e.table.Update(msg)
		return e, nil
	}

	return e, nil
}

// View implements View.
func (e *ErrorsSummary) View() string {
	if !e.ready {
		return e.renderMessage("Loading...")
	}

	total := e.deadCount + e.retryCount
	if len(e.rows) == 0 && total == 0 && e.filter.Query() == "" && !e.filter.Focused() {
		return e.renderMessage("No errors")
	}

	return e.renderSummaryBox()
}

// Name implements View.
func (e *ErrorsSummary) Name() string {
	return "Errors"
}

// ShortHelp implements View.
func (e *ErrorsSummary) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View.
func (e *ErrorsSummary) SetSize(width, height int) View {
	e.width = width
	e.height = height
	e.updateTableSize()
	return e
}

// Dispose clears cached data when the view is removed from the stack.
func (e *ErrorsSummary) Dispose() {
	e.reset()
	e.filter = filterinput.New()
	e.SetStyles(e.styles)
	e.updateTableSize()
}

// FilterFocused reports whether the filter input is capturing keys.
func (e *ErrorsSummary) FilterFocused() bool {
	return e.filter.Focused()
}

// SetStyles implements View.
func (e *ErrorsSummary) SetStyles(styles Styles) View {
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

// fetchDataCmd fetches dead and retry jobs and builds summary data.
func (e *ErrorsSummary) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		deadJobs, retryJobs, err := fetchErrorJobs(ctx, e.client, e.filter.Query())
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		rows, _ := buildErrorSummary(deadJobs, retryJobs)

		return errorsSummaryDataMsg{
			rows:       rows,
			deadCount:  len(deadJobs),
			retryCount: len(retryJobs),
		}
	}
}

func (e *ErrorsSummary) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  e.styles.Title,
		Muted:  e.styles.Muted,
		Border: e.styles.FocusBorder,
	}, "Errors", msg, e.width, e.height)
}

func (e *ErrorsSummary) reset() {
	e.ready = false
	e.rows = nil
	e.deadCount = 0
	e.retryCount = 0
	e.table.SetRows(nil)
	e.table.SetCursor(0)
}

var errorsSummaryColumns = []table.Column{
	{Title: "Job", Width: 30},
	{Title: "Error", Width: 40},
	{Title: "Queue", Width: 15},
	{Title: "Count", Width: 7},
	{Title: "Message", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (e *ErrorsSummary) updateTableSize() {
	tableHeight := max(e.height-3, 3)
	tableWidth := e.width - 4
	e.table.SetSize(tableWidth, tableHeight)
	e.filter.SetWidth(tableWidth)
}

// updateTableRows converts summary data to table rows.
func (e *ErrorsSummary) updateTableRows() {
	if e.filter.Query() != "" {
		e.table.SetEmptyMessage("No matches")
	} else {
		e.table.SetEmptyMessage("No errors")
	}

	e.table.SetColumns(errorsSummaryColumns)

	rows := make([]table.Row, 0, len(e.rows))
	for _, row := range e.rows {
		rowID := row.displayClass + "\x1f" + row.errorClass + "\x1f" + row.queue
		rows = append(rows, table.Row{
			ID: rowID,
			Cells: []string{
				row.displayClass,
				row.errorClass,
				row.queue,
				format.ShortNumber(row.count),
				row.errorMessage,
			},
		})
	}
	e.table.SetRows(rows)
	e.updateTableSize()
}

// renderSummaryBox renders the bordered box containing the summary table.
func (e *ErrorsSummary) renderSummaryBox() string {
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
