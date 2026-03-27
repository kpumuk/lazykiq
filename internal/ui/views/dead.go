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
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

const (
	deadWindowPages      = 3
	deadFallbackPageSize = 25
)

type deadJobAction int

const (
	deadJobActionNone deadJobAction = iota
	deadJobActionDelete
	deadJobActionRetry
	deadJobActionDeleteAll
	deadJobActionRetryAll
)

// Dead shows dead/morgue jobs.
type Dead struct {
	client sidekiq.API
	sortedJobsView
	dangerousActionsEnabled bool
	pendingConfirm          pendingConfirm[deadJobAction]
}

// NewDead creates a new Dead view.
func NewDead(client sidekiq.API) *Dead {
	d := &Dead{
		client: client,
		sortedJobsView: newSortedJobsView(
			"Dead Jobs",
			deadJobColumns,
			"No dead jobs",
			deadWindowPages,
			deadFallbackPageSize,
		),
	}
	d.lazy.SetFetcher(d.fetchWindow)
	return d
}

// Init implements View.
func (d *Dead) Init() tea.Cmd {
	return d.init(d.reset)
}

// Update implements View.
func (d *Dead) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if handled, cmd := d.handleSortedEntriesData(msg); handled {
			return d, cmd
		}
		return d, nil

	case RefreshMsg:
		return d, d.refreshWindow()

	case filterdialog.ActionMsg:
		return d, d.handleFilterAction(msg, d.updateEmptyMessage)

	case confirmdialog.ActionMsg:
		action, entry, ok := d.pendingConfirm.Confirm(msg, d.dangerousActionsEnabled, deadJobActionNone)
		if !ok {
			return d, nil
		}
		switch action {
		case deadJobActionNone:
			return d, nil
		case deadJobActionDelete:
			if entry == nil {
				return d, nil
			}
			return d, d.deleteJobCmd(entry)
		case deadJobActionRetry:
			if entry == nil {
				return d, nil
			}
			return d, d.retryNowJobCmd(entry)
		case deadJobActionDeleteAll:
			return d, d.deleteAllCmd()
		case deadJobActionRetryAll:
			return d, d.retryAllCmd()
		}

	case tea.KeyPressMsg:
		if handled, cmd := d.handleKeyPress(msg, d.updateEmptyMessage); handled {
			return d, cmd
		}

		switch msg.String() {
		case "c":
			if entry, ok := d.selectedEntry(); ok {
				return d, copyTextCmd(entry.JID())
			}
			return d, nil
		case "enter":
			// Show detail for selected job
			if idx := d.lazy.Table().Cursor(); idx >= 0 && idx < len(d.jobs) {
				return d, func() tea.Msg {
					return ShowJobDetailMsg{Job: d.jobs[idx].JobRecord}
				}
			}
			return d, nil
		}

		if d.dangerousActionsEnabled {
			switch msg.String() {
			case "D":
				if entry, ok := d.selectedEntry(); ok {
					d.pendingConfirm.SetForEntry(deadJobActionDelete, entry)
					return d, d.openDeleteConfirm(entry)
				}
				return d, nil
			case "R":
				if entry, ok := d.selectedEntry(); ok {
					d.pendingConfirm.SetForEntry(deadJobActionRetry, entry)
					return d, d.openRetryNowConfirm(entry)
				}
				return d, nil
			case "ctrl+d":
				d.pendingConfirm.Set(deadJobActionDeleteAll, nil, "dead.delete_all")
				return d, d.openDeleteAllConfirm()
			case "ctrl+r":
				d.pendingConfirm.Set(deadJobActionRetryAll, nil, "dead.retry_all")
				return d, d.openRetryAllConfirm()
			}
		}

		return d, d.updateKeyPress(msg)
	}

	return d, nil
}

// View implements View.
func (d *Dead) View() string {
	if !d.ready {
		return d.renderLoadingMessage()
	}

	return d.renderJobsBox()
}

// Name implements View.
func (d *Dead) Name() string {
	return "Dead"
}

// ShortHelp implements View.
func (d *Dead) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (d *Dead) ContextItems() []ContextItem {
	now := time.Now()
	lastFailed := "-"
	oldestFailed := "-"
	if d.lastEntry != nil {
		lastFailed = display.Duration(int64(now.Sub(d.lastEntry.At()).Seconds()))
	}
	if d.firstEntry != nil {
		oldestFailed = display.Duration(int64(now.Sub(d.firstEntry.At()).Seconds()))
	}

	items := []ContextItem{
		{Label: "Last failed", Value: lastFailed},
		{Label: "Oldest failed", Value: oldestFailed},
		{Label: "Total items", Value: display.Number(d.lazy.Total())},
	}
	return items
}

// HintBindings implements HintProvider.
func (d *Dead) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// MutationBindings implements MutationHintProvider.
func (d *Dead) MutationBindings() []key.Binding {
	if !d.dangerousActionsEnabled {
		return nil
	}
	return []key.Binding{
		helpBinding([]string{"D"}, "shift+d", "delete job"),
		helpBinding([]string{"R"}, "shift+r", "retry now"),
		helpBinding([]string{"ctrl+d"}, "ctrl+d", "delete all"),
		helpBinding([]string{"ctrl+r"}, "ctrl+r", "retry all"),
	}
}

// HelpSections implements HelpProvider.
func (d *Dead) HelpSections() []HelpSection {
	sections := []HelpSection{
		{
			Title: "Dead",
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
	if d.dangerousActionsEnabled {
		sections = append(sections, HelpSection{
			Title: "Dangerous Actions",
			Bindings: []key.Binding{
				helpBinding([]string{"D"}, "shift+d", "delete job"),
				helpBinding([]string{"R"}, "shift+r", "retry now"),
				helpBinding([]string{"ctrl+d"}, "ctrl+d", "delete all"),
				helpBinding([]string{"ctrl+r"}, "ctrl+r", "retry all"),
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (d *Dead) TableHelp() []key.Binding {
	return d.tableHelp()
}

// SetSize implements View.
func (d *Dead) SetSize(width, height int) View {
	d.setSize(width, height)
	return d
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (d *Dead) SetDangerousActionsEnabled(enabled bool) {
	d.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (d *Dead) Dispose() {
	d.dispose(d.reset)
}

// CancelRequests stops in-flight fetches when the view is hidden.
func (d *Dead) CancelRequests() {
	d.cancelRequests()
}

// SetStyles implements View.
func (d *Dead) SetStyles(styles Styles) View {
	d.setStyles(styles)
	return d
}

func (d *Dead) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	return fetchSortedEntriesWindow(ctx, sortedEntriesFetchConfig{
		tracker:          "dead.fetchWindow",
		client:           d.client,
		kind:             sidekiq.SortedSetDead,
		filter:           d.filter,
		windowStart:      windowStart,
		windowSize:       windowSize,
		fallbackPageSize: deadFallbackPageSize,
		windowPages:      deadWindowPages,
		buildRows:        d.buildRows,
	})
}

func (d *Dead) reset() {
	d.resetSortedJobs(d.updateEmptyMessage)
}

func (d *Dead) selectedEntry() (*sidekiq.SortedEntry, bool) {
	return d.selectedSortedEntry()
}

// Table columns for dead job list.
var deadJobColumns = []table.Column{
	{Title: "Last Retry", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

func (d *Dead) updateEmptyMessage() {
	msg := "No dead jobs"
	if d.filter != "" {
		msg = "No matches"
	}
	d.lazy.SetEmptyMessage(msg)
}

func (d *Dead) buildRows(jobs []*sidekiq.SortedEntry) []table.Row {
	rows := make([]table.Row, 0, len(jobs))
	now := time.Now()
	for _, job := range jobs {
		lastRetry := display.Duration(int64(now.Sub(job.At()).Seconds()))

		errorStr := ""
		if job.HasError() {
			errorStr = fmt.Sprintf("%s: %s", job.ErrorClass(), job.ErrorMessage())
			if len(errorStr) > 100 {
				errorStr = errorStr[:97] + "..."
			}
		}

		rows = append(rows, table.Row{
			ID: job.JID(),
			Cells: []string{
				lastRetry,
				d.styles.QueueText.Render(job.Queue()),
				job.DisplayClass(),
				display.Args(job.DisplayArgs()),
				errorStr,
			},
		})
	}
	return rows
}

func (d *Dead) openDeleteConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := d.jobName(entry)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				d.styles,
				"Delete job",
				fmt.Sprintf(
					"Are you sure you want to delete the %s job?\n\nThis action is not recoverable.",
					d.styles.Text.Bold(true).Render(jobName),
				),
				entry.JID(),
				d.styles.DangerAction,
			),
		}
	}
}

func (d *Dead) openRetryNowConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := d.jobName(entry)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				d.styles,
				"Retry job",
				fmt.Sprintf(
					"Retry the %s job now?\n\nThis will enqueue it immediately.",
					d.styles.Text.Bold(true).Render(jobName),
				),
				entry.JID(),
				d.styles.DangerAction,
			),
		}
	}
}

func (d *Dead) openDeleteAllConfirm() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				d.styles,
				"Delete all dead",
				"Are you sure you want to delete all dead jobs?\n\nThis action is not recoverable.",
				"dead.delete_all",
				d.styles.DangerAction,
			),
		}
	}
}

func (d *Dead) openRetryAllConfirm() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				d.styles,
				"Retry all dead",
				"Retry all dead jobs now?\n\nThis will enqueue them immediately.",
				"dead.retry_all",
				d.styles.DangerAction,
			),
		}
	}
}

func (d *Dead) deleteJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "dead.deleteJobCmd")
		if err := d.client.DeleteSortedEntry(ctx, sidekiq.SortedSetDead, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (d *Dead) deleteAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "dead.deleteAllCmd")
		if err := d.client.DeleteAllSortedEntries(ctx, sidekiq.SortedSetDead); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (d *Dead) retryNowJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "dead.retryNowJobCmd")
		if err := d.client.EnqueueSortedEntry(ctx, sidekiq.SortedSetDead, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (d *Dead) retryAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "dead.retryAllCmd")
		if err := d.client.EnqueueAllSortedEntries(ctx, sidekiq.SortedSetDead); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
func (d *Dead) renderJobsBox() string {
	return d.renderSortedJobsBox("Dead Jobs")
}

// renderJobDetail renders the job detail view.
