package views

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

const (
	errorsDetailsWindowPages      = 3
	errorsDetailsFallbackPageSize = 25
)

type errorDetailsPayload struct {
	jobs []sidekiq.ErrorGroupEntry
}

// ErrorsDetails shows all jobs for a selected error group.
type ErrorsDetails struct {
	client sidekiq.API
	detailListView
	groupKey  sidekiq.ErrorGroupKey
	groupJobs []sidekiq.ErrorGroupEntry
}

// NewErrorsDetails creates a new ErrorsDetails view.
func NewErrorsDetails(client sidekiq.API) *ErrorsDetails {
	e := &ErrorsDetails{
		client: client,
		detailListView: newDetailListView(
			"Errors",
			errorDetailsColumns,
			"No errors",
			errorsDetailsWindowPages,
			errorsDetailsFallbackPageSize,
		),
	}
	e.lazy.SetFetcher(e.fetchWindow)
	return e
}

// SetErrorGroup sets the selected error group and query.
func (e *ErrorsDetails) SetErrorGroup(key sidekiq.ErrorGroupKey, query string) {
	e.groupKey = key
	e.filter = query
}

// Init implements View.
func (e *ErrorsDetails) Init() tea.Cmd {
	return e.init(e.resetData)
}

// Update implements View.
func (e *ErrorsDetails) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if handled, cmd := e.handleData(msg, func(result lazytable.FetchResult) {
			if payload, ok := result.Payload.(errorDetailsPayload); ok {
				e.groupJobs = payload.jobs
			}
		}); handled {
			return e, cmd
		}
		return e, nil

	case RefreshMsg:
		return e, e.refreshWindow()

	case filterdialog.ActionMsg:
		return e, e.handleFilterAction(msg, e.updateEmptyMessage)

	case tea.KeyPressMsg:
		if handled, cmd := e.handleKeyPress(msg, e.updateEmptyMessage); handled {
			return e, cmd
		}

		switch msg.String() {
		case "c":
			if job, ok := e.selectedEntry(); ok && job.Entry != nil {
				return e, copyTextCmd(job.Entry.JID())
			}
			return e, nil
		}

		switch msg.String() {
		case "enter":
			if job, ok := e.selectedEntry(); ok && job.Entry != nil {
				return e, func() tea.Msg {
					return ShowJobDetailMsg{Job: job.Entry.JobRecord}
				}
			}
			return e, nil
		}

		return e, e.updateKeyPress(msg)
	}

	return e, nil
}

// View implements View.
func (e *ErrorsDetails) View() string {
	if !e.ready {
		return e.renderLoadingMessage()
	}

	return e.renderDetailsBox()
}

// Name implements View.
func (e *ErrorsDetails) Name() string {
	if e.groupKey.ErrorClass != "" {
		return e.groupKey.ErrorClass
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
	if e.groupKey.DisplayClass != "" {
		items = append(items, ContextItem{Label: "Job", Value: e.groupKey.DisplayClass})
	}
	if e.groupKey.ErrorClass != "" {
		items = append(items, ContextItem{Label: "Error", Value: e.groupKey.ErrorClass})
	}
	if e.groupKey.Queue != "" {
		items = append(items, ContextItem{Label: "Queue", Value: e.styles.QueueText.Render(e.groupKey.Queue)})
	}
	items = append(items, ContextItem{Label: "Total", Value: display.Number(e.lazy.Total())})
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
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
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
				helpBinding([]string{"["}, "[", "page up"),
				helpBinding([]string{"]"}, "]", "page down"),
				helpBinding([]string{"g"}, "g", "jump to start"),
				helpBinding([]string{"G"}, "shift+g", "jump to end"),
				helpBinding([]string{"c"}, "c", "copy jid"),
				helpBinding([]string{"enter"}, "enter", "job detail"),
			},
		},
	}
}

// TableHelp implements TableHelpProvider.
func (e *ErrorsDetails) TableHelp() []key.Binding {
	return e.tableHelp()
}

// SetSize implements View.
func (e *ErrorsDetails) SetSize(width, height int) View {
	e.setSize(width, height)
	return e
}

// Dispose clears cached data when the view is removed from the stack.
func (e *ErrorsDetails) Dispose() {
	e.dispose(e.reset)
}

// SetStyles implements View.
func (e *ErrorsDetails) SetStyles(styles Styles) View {
	e.setStyles(styles)
	return e
}

// CancelRequests stops in-flight fetches when the view is hidden.
func (e *ErrorsDetails) CancelRequests() {
	e.cancelRequests()
}

func (e *ErrorsDetails) resetData() {
	e.resetShell()
	e.groupJobs = nil
	e.lazy.Table().SetColumns(errorDetailsColumns)
	e.lazy.Table().ScrollToStart()
	e.updateEmptyMessage()
}

func (e *ErrorsDetails) reset() {
	e.resetData()
	e.groupKey = sidekiq.ErrorGroupKey{}
}

var errorDetailsColumns = []table.Column{
	{Title: "Set", Width: 5},
	{Title: "At", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

func (e *ErrorsDetails) updateEmptyMessage() {
	msg := "No errors"
	if e.filter != "" {
		msg = "No matches"
	}
	e.lazy.SetEmptyMessage(msg)
}

func (e *ErrorsDetails) buildRows(jobs []sidekiq.ErrorGroupEntry) []table.Row {
	rows := make([]table.Row, 0, len(jobs))
	now := time.Now()
	for _, job := range jobs {
		if job.Entry == nil {
			continue
		}
		when := display.Duration(int64(now.Sub(job.Entry.At()).Seconds()))
		queue := job.Entry.Queue()
		if queue == "" {
			queue = "unknown"
		}
		queue = e.styles.QueueText.Render(queue)
		message := errorDisplay(job.Entry)
		rows = append(rows, table.Row{
			ID: job.Entry.JID(),
			Cells: []string{
				job.Source,
				when,
				queue,
				job.Entry.DisplayClass(),
				display.Args(job.Entry.DisplayArgs()),
				message,
			},
		})
	}
	return rows
}

func (e *ErrorsDetails) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	ctx = devtools.WithTracker(ctx, "errors_details.fetchWindow")

	window, err := e.client.GetErrorGroupWindow(ctx, e.groupKey, e.filter, windowStart, windowSize)
	if err != nil {
		return lazytable.FetchResult{}, err
	}

	return lazytable.FetchResult{
		Rows:        e.buildRows(window.Entries),
		Total:       window.Total,
		WindowStart: window.WindowStart,
		Payload: errorDetailsPayload{
			jobs: window.Entries,
		},
	}, nil
}

func (e *ErrorsDetails) selectedEntry() (sidekiq.ErrorGroupEntry, bool) {
	idx := e.lazy.Table().Cursor()
	if idx < 0 || idx >= len(e.groupJobs) {
		return sidekiq.ErrorGroupEntry{}, false
	}
	return e.groupJobs[idx], true
}

// renderDetailsBox renders the bordered box containing the detail table.
func (e *ErrorsDetails) renderDetailsBox() string {
	return e.renderBox(
		fmt.Sprintf("Error %s in %s", e.groupKey.ErrorClass, e.groupKey.DisplayClass),
		len(e.groupJobs),
	)
}
