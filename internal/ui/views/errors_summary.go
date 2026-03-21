package views

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
	"github.com/kpumuk/lazykiq/internal/ui/requestctx"
)

// errorsSummaryDataMsg carries error summary data internally.
type errorsSummaryDataMsg struct {
	rows      []sidekiq.ErrorSummaryRow
	meta      sidekiq.ErrorSummaryMeta
	fetchedAt time.Time
}

const errorsSummaryRefreshInterval = time.Minute

var nowFuncErrorsSummary = time.Now

// ErrorsSummary shows a summary of errors grouped by job and error class.
type ErrorsSummary struct {
	client       sidekiq.API
	width        int
	height       int
	styles       Styles
	rows         []sidekiq.ErrorSummaryRow
	table        table.Model
	ready        bool
	refreshing   bool
	meta         sidekiq.ErrorSummaryMeta
	fetchedAt    time.Time
	filter       string
	frameStyles  frame.Styles
	filterStyle  filterdialog.Styles
	fetchRequest requestctx.Controller
}

// NewErrorsSummary creates a new ErrorsSummary view.
func NewErrorsSummary(client sidekiq.API) *ErrorsSummary {
	return &ErrorsSummary{
		client: client,
		table: table.New(
			table.WithColumns(errorsSummaryColumns),
			table.WithEmptyMessage("No errors"),
		),
	}
}

// Init implements View.
func (e *ErrorsSummary) Init() tea.Cmd {
	e.reset()
	return e.fetchDataCmd(true)
}

// Update implements View.
func (e *ErrorsSummary) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case errorsSummaryDataMsg:
		e.rows = msg.rows
		e.meta = msg.meta
		e.fetchedAt = msg.fetchedAt
		e.ready = true
		e.refreshing = false
		e.updateTableRows()
		return e, nil

	case RefreshMsg:
		return e, e.fetchDataCmd(false)

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return e, nil
		}
		if msg.Query == e.filter {
			return e, nil
		}
		e.filter = msg.Query
		e.table.SetCursor(0)
		return e, e.fetchDataCmd(true)

	case tea.KeyPressMsg:
		switch msg.String() {
		case "/":
			return e, e.openFilterDialog()
		case "ctrl+u":
			if e.filter != "" {
				e.filter = ""
				e.table.SetCursor(0)
				return e, e.fetchDataCmd(true)
			}
			return e, nil
		case "r":
			return e, e.fetchDataCmd(true)
		}

		switch msg.String() {
		case "enter":
			row, ok := e.selectedRow()
			if !ok {
				return e, nil
			}
			return e, func() tea.Msg {
				return ShowErrorDetailsMsg{
					Key:   errorGroupKeyForRow(row),
					Query: e.filter,
				}
			}
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

	total := e.meta.DeadCount + e.meta.RetryCount
	if len(e.rows) == 0 && total == 0 && e.filter == "" {
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

// ContextItems implements ContextProvider.
func (e *ErrorsSummary) ContextItems() []ContextItem {
	items := []ContextItem{
		{Label: "Updated", Value: e.updatedLabel()},
		{Label: "Dead", Value: display.Number(e.meta.DeadCount)},
		{Label: "Retry", Value: display.Number(e.meta.RetryCount)},
	}
	if e.filter != "" {
		items = append(items, ContextItem{Label: "Filter", Value: e.filter})
	}
	return items
}

// HintBindings implements HintProvider.
func (e *ErrorsSummary) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"r"}, "r", "refresh"),
		helpBinding([]string{"enter"}, "enter", "error details"),
	}
}

// HelpSections implements HelpProvider.
func (e *ErrorsSummary) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Errors",
			Bindings: []key.Binding{
				helpBinding([]string{"/"}, "/", "filter"),
				helpBinding([]string{"ctrl+u"}, "ctrl+u", "clear filter"),
				helpBinding([]string{"r"}, "r", "refresh"),
				helpBinding([]string{"enter"}, "enter", "error details"),
			},
		},
	}
}

// TableHelp implements TableHelpProvider.
func (e *ErrorsSummary) TableHelp() []key.Binding {
	return tableHelpBindings(e.table.KeyMap)
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
	e.filter = ""
	e.SetStyles(e.styles)
	e.updateTableSize()
}

// SetStyles implements View.
func (e *ErrorsSummary) SetStyles(styles Styles) View {
	e.styles = styles
	e.frameStyles = frameStylesFromTheme(styles)
	e.filterStyle = filterDialogStylesFromTheme(styles)
	e.table.SetStyles(tableStylesFromTheme(styles))
	return e
}

// CancelRequests stops in-flight fetches when the view is hidden.
func (e *ErrorsSummary) CancelRequests() {
	e.fetchRequest.Cancel()
	e.refreshing = false
}

// fetchDataCmd refreshes the exact cached summary snapshot.
func (e *ErrorsSummary) fetchDataCmd(force bool) tea.Cmd {
	if e.shouldSkipRefresh(force) {
		return nil
	}

	e.refreshing = true
	ctx := e.fetchRequest.Start(devtools.WithTracker(context.Background(), "errors.fetchDataCmd"))
	return func() tea.Msg {
		rows, meta, err := e.client.GetErrorSummary(ctx, e.filter)
		if err != nil {
			if requestctx.IsCanceled(err) {
				return nil
			}
			return ConnectionErrorMsg{Err: err}
		}

		return errorsSummaryDataMsg{
			rows:      rows,
			meta:      meta,
			fetchedAt: nowFuncErrorsSummary(),
		}
	}
}

func (e *ErrorsSummary) shouldSkipRefresh(force bool) bool {
	if force {
		return false
	}
	if e.refreshing {
		return true
	}
	if !e.ready || e.fetchedAt.IsZero() {
		return false
	}
	return nowFuncErrorsSummary().Sub(e.fetchedAt) < errorsSummaryRefreshInterval
}

func (e *ErrorsSummary) renderMessage(msg string) string {
	return renderStatusMessage("Errors", msg, e.styles, e.width, e.height)
}

func (e *ErrorsSummary) reset() {
	e.fetchRequest.Cancel()
	e.ready = false
	e.refreshing = false
	e.rows = nil
	e.meta = sidekiq.ErrorSummaryMeta{}
	e.fetchedAt = time.Time{}
	e.table.SetRows(nil)
	e.table.SetCursor(0)
}

var errorsSummaryColumns = []table.Column{
	{Title: "Job", Width: 30},
	{Title: "Error", Width: 40},
	{Title: "Queue", Width: 15},
	{Title: "Count", Width: 10, Align: table.AlignRight},
	{Title: "Message", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (e *ErrorsSummary) updateTableSize() {
	tableWidth, tableHeight := framedTableSize(e.width, e.height)
	e.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts summary data to table rows.
func (e *ErrorsSummary) updateTableRows() {
	if e.filter != "" {
		e.table.SetEmptyMessage("No matches")
	} else {
		e.table.SetEmptyMessage("No errors")
	}

	e.table.SetColumns(errorsSummaryColumns)

	rows := make([]table.Row, 0, len(e.rows))
	for _, row := range e.rows {
		rows = append(rows, table.Row{
			ID: errorGroupRowID(errorGroupKeyForRow(row)),
			Cells: []string{
				row.DisplayClass,
				row.ErrorClass,
				e.styles.QueueText.Render(row.Queue),
				display.Number(row.Count),
				row.ErrorMessage,
			},
		})
	}
	e.table.SetRows(rows)
	e.updateTableSize()
}

func (e *ErrorsSummary) selectedRow() (sidekiq.ErrorSummaryRow, bool) {
	idx := e.table.Cursor()
	if idx < 0 || idx >= len(e.rows) {
		return sidekiq.ErrorSummaryRow{}, false
	}
	return e.rows[idx], true
}

func (e *ErrorsSummary) updatedLabel() string {
	if e.fetchedAt.IsZero() {
		if e.refreshing {
			return "updating..."
		}
		return "-"
	}
	return display.Duration(int64(nowFuncErrorsSummary().Sub(e.fetchedAt).Seconds())) + " ago"
}

func (e *ErrorsSummary) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(e.filterStyle),
				filterdialog.WithQuery(e.filter),
			),
		}
	}
}

// renderSummaryBox renders the bordered box containing the summary table.
func (e *ErrorsSummary) renderSummaryBox() string {
	content := e.table.View()

	box := frame.New(
		frame.WithStyles(e.frameStyles),
		frame.WithTitle("Errors"),
		frame.WithFilter(e.filter),
		frame.WithTitlePadding(0),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(e.width, e.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}
