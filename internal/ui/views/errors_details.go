package views

import (
	"context"
	"fmt"
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

// errorsDetailsDataMsg carries error detail data internally.
type errorsDetailsDataMsg struct {
	jobs []errorGroupJob
}

// ErrorsDetails shows all jobs for a selected error group.
type ErrorsDetails struct {
	client *sidekiq.Client
	width  int
	height int
	styles Styles

	ready     bool
	groupKey  errorSummaryKey
	groupJobs []errorGroupJob
	table     table.Model
	filter    filterinput.Model
}

// NewErrorsDetails creates a new ErrorsDetails view.
func NewErrorsDetails(client *sidekiq.Client) *ErrorsDetails {
	return &ErrorsDetails{
		client: client,
		table: table.New(
			table.WithColumns(errorDetailsColumns),
			table.WithEmptyMessage("No errors"),
		),
		filter: filterinput.New(),
	}
}

// SetErrorGroup sets the selected error group and query.
func (e *ErrorsDetails) SetErrorGroup(displayClass, errorClass, queue, query string) {
	e.groupKey = errorSummaryKey{
		displayClass: displayClass,
		errorClass:   errorClass,
		queue:        queue,
	}
	e.filter = filterinput.New(filterinput.WithQuery(query))
	e.SetStyles(e.styles)
	e.updateTableSize()
}

// Init implements View.
func (e *ErrorsDetails) Init() tea.Cmd {
	e.resetData()
	e.filter.Init()
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

	if len(e.groupJobs) == 0 && e.filter.Query() == "" && !e.filter.Focused() {
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
	e.filter = filterinput.New()
	e.SetStyles(e.styles)
	e.updateTableSize()
}

// SetStyles implements View.
func (e *ErrorsDetails) SetStyles(styles Styles) View {
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

func (e *ErrorsDetails) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  e.styles.Title,
		Muted:  e.styles.Muted,
		Border: e.styles.FocusBorder,
	}, "Errors", msg, e.width, e.height)
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
	tableHeight := max(e.height-3, 3)
	tableWidth := e.width - 4
	e.table.SetSize(tableWidth, tableHeight)
	e.filter.SetWidth(tableWidth)
}

// updateTableRows converts group data to table rows.
func (e *ErrorsDetails) updateTableRows() {
	if e.filter.Query() != "" {
		e.table.SetEmptyMessage("No matches")
	} else {
		e.table.SetEmptyMessage("No errors")
	}

	e.table.SetColumns(errorDetailsColumns)

	rows := make([]table.Row, 0, len(e.groupJobs))
	now := time.Now().Unix()
	for _, job := range e.groupJobs {
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

func (e *ErrorsDetails) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		deadJobs, retryJobs, err := fetchErrorJobs(ctx, e.client, e.filter.Query())
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
		frame.WithTitle(fmt.Sprintf("Error %s in %s", e.groupKey.errorClass, e.groupKey.displayClass)),
		frame.WithTitlePadding(0),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(e.width, e.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}
