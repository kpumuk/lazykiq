package views

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// errorsDetailsDataMsg carries error detail data internally.
type errorsDetailsDataMsg struct {
	jobs []errorGroupJob
}

// ErrorsDetails shows all jobs for a selected error group.
type ErrorsDetails struct {
	client sidekiq.API
	width  int
	height int
	styles Styles

	ready       bool
	groupKey    errorSummaryKey
	groupJobs   []errorGroupJob
	table       table.Model
	filter      string
	frameStyles frame.Styles
	filterStyle filterdialog.Styles
}

// NewErrorsDetails creates a new ErrorsDetails view.
func NewErrorsDetails(client sidekiq.API) *ErrorsDetails {
	return &ErrorsDetails{
		client: client,
		table: table.New(
			table.WithColumns(errorDetailsColumns),
			table.WithEmptyMessage("No errors"),
		),
	}
}

// SetErrorGroup sets the selected error group and query.
func (e *ErrorsDetails) SetErrorGroup(displayClass, errorClass, queue, query string) {
	e.groupKey = errorSummaryKey{
		displayClass: displayClass,
		errorClass:   errorClass,
		queue:        queue,
	}
	e.filter = query
}

// Init implements View.
func (e *ErrorsDetails) Init() tea.Cmd {
	e.resetData()
	return e.fetchDataCmd()
}

// Update implements View.
func (e *ErrorsDetails) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case errorsDetailsDataMsg:
		e.groupJobs = msg.jobs
		e.ready = true
		e.updateTableRows()
		return e, nil

	case RefreshMsg:
		return e, e.fetchDataCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return e, nil
		}
		if msg.Query == e.filter {
			return e, nil
		}
		e.filter = msg.Query
		e.table.SetCursor(0)
		return e, e.fetchDataCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return e, e.openFilterDialog()
		case "ctrl+u":
			if e.filter != "" {
				e.filter = ""
				e.table.SetCursor(0)
				return e, e.fetchDataCmd()
			}
			return e, nil
		case "c":
			if idx := e.table.Cursor(); idx >= 0 && idx < len(e.groupJobs) {
				job := e.groupJobs[idx]
				if job.entry != nil {
					return e, copyTextCmd(job.entry.JID())
				}
			}
			return e, nil
		}

		switch msg.String() {
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
	}

	return e, nil
}

// View implements View.
func (e *ErrorsDetails) View() string {
	if !e.ready {
		return e.renderMessage("Loading...")
	}

	if len(e.groupJobs) == 0 && e.filter == "" {
		return e.renderMessage("No errors")
	}

	return e.renderDetailsBox()
}

// Name implements View.
func (e *ErrorsDetails) Name() string {
	if e.groupKey.errorClass != "" {
		return e.groupKey.errorClass
	}
	return "Jobs"
}

// ShortHelp implements View.
func (e *ErrorsDetails) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (e *ErrorsDetails) ContextItems() []ContextItem {
	items := []ContextItem{}
	if e.groupKey.displayClass != "" {
		items = append(items, ContextItem{Label: "Job", Value: e.groupKey.displayClass})
	}
	if e.groupKey.errorClass != "" {
		items = append(items, ContextItem{Label: "Error", Value: e.groupKey.errorClass})
	}
	if e.groupKey.queue != "" {
		items = append(items, ContextItem{Label: "Queue", Value: e.styles.QueueText.Render(e.groupKey.queue)})
	}
	if e.filter != "" {
		items = append(items, ContextItem{Label: "Filter", Value: e.filter})
	}
	return items
}

// HintBindings implements HintProvider.
func (e *ErrorsDetails) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// HelpSections implements HelpProvider.
func (e *ErrorsDetails) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Error Group",
			Bindings: []key.Binding{
				helpBinding([]string{"/"}, "/", "filter"),
				helpBinding([]string{"ctrl+u"}, "ctrl+u", "clear filter"),
				helpBinding([]string{"c"}, "c", "copy jid"),
				helpBinding([]string{"enter"}, "enter", "job detail"),
			},
		},
	}
}

// TableHelp implements TableHelpProvider.
func (e *ErrorsDetails) TableHelp() []key.Binding {
	return tableHelpBindings(e.table.KeyMap)
}

// SetSize implements View.
func (e *ErrorsDetails) SetSize(width, height int) View {
	e.width = width
	e.height = height
	e.updateTableSize()
	return e
}

// Dispose clears cached data when the view is removed from the stack.
func (e *ErrorsDetails) Dispose() {
	e.reset()
	e.filter = ""
	e.SetStyles(e.styles)
	e.updateTableSize()
}

// SetStyles implements View.
func (e *ErrorsDetails) SetStyles(styles Styles) View {
	e.styles = styles
	e.frameStyles = frameStylesFromTheme(styles)
	e.filterStyle = filterDialogStylesFromTheme(styles)
	e.table.SetStyles(tableStylesFromTheme(styles))
	return e
}

func (e *ErrorsDetails) renderMessage(msg string) string {
	return renderStatusMessage("Errors", msg, e.styles, e.width, e.height)
}

func (e *ErrorsDetails) resetData() {
	e.ready = false
	e.groupJobs = nil
	e.table.SetRows(nil)
	e.table.SetCursor(0)
}

func (e *ErrorsDetails) reset() {
	e.resetData()
	e.groupKey = errorSummaryKey{}
}

var errorDetailsColumns = []table.Column{
	{Title: "Set", Width: 5},
	{Title: "At", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (e *ErrorsDetails) updateTableSize() {
	tableWidth, tableHeight := framedTableSize(e.width, e.height)
	e.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts group data to table rows.
func (e *ErrorsDetails) updateTableRows() {
	if e.filter != "" {
		e.table.SetEmptyMessage("No matches")
	} else {
		e.table.SetEmptyMessage("No errors")
	}

	e.table.SetColumns(errorDetailsColumns)

	rows := make([]table.Row, 0, len(e.groupJobs))
	now := time.Now()
	for _, job := range e.groupJobs {
		if job.entry == nil {
			continue
		}
		when := format.Duration(int64(now.Sub(job.entry.At()).Seconds()))
		queue := job.entry.Queue()
		if queue == "" {
			queue = "unknown"
		}
		queue = e.styles.QueueText.Render(queue)
		message := errorDisplay(job.entry)
		rows = append(rows, table.Row{
			ID: job.entry.JID(),
			Cells: []string{
				job.source,
				when,
				queue,
				job.entry.DisplayClass(),
				format.Args(job.entry.DisplayArgs()),
				message,
			},
		})
	}
	e.table.SetRows(rows)
	e.updateTableSize()
}

func (e *ErrorsDetails) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(e.filterStyle),
				filterdialog.WithQuery(e.filter),
			),
		}
	}
}

func (e *ErrorsDetails) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "errors_details.fetchDataCmd")
		deadJobs, retryJobs, err := fetchErrorJobs(ctx, e.client, e.filter)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		_, groups := buildErrorSummary(deadJobs, retryJobs)
		jobs := groups[e.groupKey]

		return errorsDetailsDataMsg{jobs: jobs}
	}
}

// renderDetailsBox renders the bordered box containing the detail table.
func (e *ErrorsDetails) renderDetailsBox() string {
	content := e.table.View()

	box := frame.New(
		frame.WithStyles(e.frameStyles),
		frame.WithTitle(fmt.Sprintf("Error %s in %s", e.groupKey.errorClass, e.groupKey.displayClass)),
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
