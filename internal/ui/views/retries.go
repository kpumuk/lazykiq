package views

import (
	"context"
	"fmt"
	"strconv"
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
	retriesWindowPages      = 3
	retriesFallbackPageSize = 25
)

type retriesJobAction int

const (
	retriesJobActionNone retriesJobAction = iota
	retriesJobActionDelete
	retriesJobActionKill
	retriesJobActionRetry
	retriesJobActionDeleteAll
	retriesJobActionKillAll
	retriesJobActionRetryAll
)

// Retries shows failed jobs pending retry.
type Retries struct {
	client sidekiq.API
	sortedJobsView
	dangerousActionsEnabled bool
	pendingConfirm          pendingConfirm[retriesJobAction]
}

// NewRetries creates a new Retries view.
func NewRetries(client sidekiq.API) *Retries {
	r := &Retries{
		client: client,
		sortedJobsView: newSortedJobsView(
			"Retries",
			retryJobColumns,
			"No retries",
			retriesWindowPages,
			retriesFallbackPageSize,
		),
	}
	r.lazy.SetFetcher(r.fetchWindow)
	return r
}

// Init implements View.
func (r *Retries) Init() tea.Cmd {
	return r.init(r.reset)
}

// Update implements View.
func (r *Retries) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if handled, cmd := r.handleSortedEntriesData(msg); handled {
			return r, cmd
		}
		return r, nil

	case RefreshMsg:
		return r, r.refreshWindow()

	case filterdialog.ActionMsg:
		return r, r.handleFilterAction(msg, r.updateEmptyMessage)

	case confirmdialog.ActionMsg:
		action, entry, ok := r.pendingConfirm.Confirm(msg, r.dangerousActionsEnabled, retriesJobActionNone)
		if !ok {
			return r, nil
		}
		switch action {
		case retriesJobActionNone:
			return r, nil
		case retriesJobActionDelete:
			if entry == nil {
				return r, nil
			}
			return r, r.deleteJobCmd(entry)
		case retriesJobActionKill:
			if entry == nil {
				return r, nil
			}
			return r, r.killJobCmd(entry)
		case retriesJobActionRetry:
			if entry == nil {
				return r, nil
			}
			return r, r.retryNowJobCmd(entry)
		case retriesJobActionDeleteAll:
			return r, r.deleteAllCmd()
		case retriesJobActionKillAll:
			return r, r.killAllCmd()
		case retriesJobActionRetryAll:
			return r, r.retryAllCmd()
		}

	case tea.KeyPressMsg:
		if handled, cmd := r.handleKeyPress(msg, r.updateEmptyMessage); handled {
			return r, cmd
		}

		switch msg.String() {
		case "c":
			if entry, ok := r.selectedSortedEntry(); ok {
				return r, copyTextCmd(entry.JID())
			}
			return r, nil
		case "enter":
			// Show detail for selected job
			if idx := r.lazy.Table().Cursor(); idx >= 0 && idx < len(r.jobs) {
				return r, func() tea.Msg {
					return ShowJobDetailMsg{Job: r.jobs[idx].JobRecord}
				}
			}
			return r, nil
		}

		if r.dangerousActionsEnabled {
			switch msg.String() {
			case "D":
				if entry, ok := r.selectedSortedEntry(); ok {
					r.pendingConfirm.SetForEntry(retriesJobActionDelete, entry)
					return r, r.openDeleteConfirm(entry)
				}
				return r, nil
			case "K":
				if entry, ok := r.selectedSortedEntry(); ok {
					r.pendingConfirm.SetForEntry(retriesJobActionKill, entry)
					return r, r.openKillConfirm(entry)
				}
				return r, nil
			case "R":
				if entry, ok := r.selectedSortedEntry(); ok {
					r.pendingConfirm.SetForEntry(retriesJobActionRetry, entry)
					return r, r.openRetryNowConfirm(entry)
				}
				return r, nil
			case "ctrl+d":
				r.pendingConfirm.Set(retriesJobActionDeleteAll, nil, "retries.delete_all")
				return r, r.openDeleteAllConfirm()
			case "ctrl+k":
				r.pendingConfirm.Set(retriesJobActionKillAll, nil, "retries.kill_all")
				return r, r.openKillAllConfirm()
			case "ctrl+r":
				r.pendingConfirm.Set(retriesJobActionRetryAll, nil, "retries.retry_all")
				return r, r.openRetryAllConfirm()
			}
		}

		return r, r.updateKeyPress(msg)
	}

	return r, nil
}

// View implements View.
func (r *Retries) View() string {
	if !r.ready {
		return r.renderLoadingMessage()
	}

	return r.renderSortedJobsBox("Retries")
}

// Name implements View.
func (r *Retries) Name() string {
	return "Retries"
}

// ShortHelp implements View.
func (r *Retries) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (r *Retries) ContextItems() []ContextItem {
	now := time.Now()
	nextRetry := "-"
	latestRetry := "-"
	if r.firstEntry != nil {
		nextRetry = display.Duration(int64(r.firstEntry.At().Sub(now).Seconds()))
	}
	if r.lastEntry != nil {
		latestRetry = display.Duration(int64(r.lastEntry.At().Sub(now).Seconds()))
	}

	items := []ContextItem{
		{Label: "Next retry in", Value: nextRetry},
		{Label: "Latest retry in", Value: latestRetry},
		{Label: "Total items", Value: display.Number(r.lazy.Total())},
	}
	return items
}

// HintBindings implements HintProvider.
func (r *Retries) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// MutationBindings implements MutationHintProvider.
func (r *Retries) MutationBindings() []key.Binding {
	if !r.dangerousActionsEnabled {
		return nil
	}
	return []key.Binding{
		helpBinding([]string{"D"}, "shift+d", "delete job"),
		helpBinding([]string{"K"}, "shift+k", "kill job"),
		helpBinding([]string{"R"}, "shift+r", "retry now"),
		helpBinding([]string{"ctrl+d"}, "ctrl+d", "delete all"),
		helpBinding([]string{"ctrl+k"}, "ctrl+k", "kill all"),
		helpBinding([]string{"ctrl+r"}, "ctrl+r", "retry all"),
	}
}

// HelpSections implements HelpProvider.
func (r *Retries) HelpSections() []HelpSection {
	sections := []HelpSection{
		{
			Title: "Retries",
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
	if r.dangerousActionsEnabled {
		sections = append(sections, HelpSection{
			Title: "Dangerous Actions",
			Bindings: []key.Binding{
				helpBinding([]string{"D"}, "shift+d", "delete job"),
				helpBinding([]string{"K"}, "shift+k", "kill job"),
				helpBinding([]string{"R"}, "shift+r", "retry now"),
				helpBinding([]string{"ctrl+d"}, "ctrl+d", "delete all"),
				helpBinding([]string{"ctrl+k"}, "ctrl+k", "kill all"),
				helpBinding([]string{"ctrl+r"}, "ctrl+r", "retry all"),
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (r *Retries) TableHelp() []key.Binding {
	return r.tableHelp()
}

// SetSize implements View.
func (r *Retries) SetSize(width, height int) View {
	r.setSize(width, height)
	return r
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (r *Retries) SetDangerousActionsEnabled(enabled bool) {
	r.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (r *Retries) Dispose() {
	r.dispose(r.reset)
}

// CancelRequests stops in-flight fetches when the view is hidden.
func (r *Retries) CancelRequests() {
	r.cancelRequests()
}

// SetStyles implements View.
func (r *Retries) SetStyles(styles Styles) View {
	r.setStyles(styles)
	return r
}

func (r *Retries) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	return fetchSortedEntriesWindow(ctx, sortedEntriesFetchConfig{
		tracker:          "retries.fetchWindow",
		client:           r.client,
		kind:             sidekiq.SortedSetRetry,
		filter:           r.filter,
		windowStart:      windowStart,
		windowSize:       windowSize,
		fallbackPageSize: retriesFallbackPageSize,
		windowPages:      retriesWindowPages,
		buildRows:        r.buildRows,
	})
}

func (r *Retries) reset() {
	r.resetSortedJobs(r.updateEmptyMessage)
}

// Table columns for retry job list.
var retryJobColumns = []table.Column{
	{Title: "Next Retry", Width: 12},
	{Title: "Retries", Width: 7},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

func (r *Retries) updateEmptyMessage() {
	msg := "No retries"
	if r.filter != "" {
		msg = "No matches"
	}
	r.lazy.SetEmptyMessage(msg)
}

func (r *Retries) buildRows(jobs []*sidekiq.SortedEntry) []table.Row {
	rows := make([]table.Row, 0, len(jobs))
	now := time.Now()
	for _, job := range jobs {
		nextRetry := display.Duration(int64(now.Sub(job.At()).Seconds()))
		retryCount := strconv.Itoa(job.RetryCount())

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
				nextRetry,
				retryCount,
				r.styles.QueueText.Render(job.Queue()),
				job.DisplayClass(),
				display.Args(job.DisplayArgs()),
				errorStr,
			},
		})
	}
	return rows
}

func (r *Retries) openDeleteConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := r.jobName(entry)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				r.styles,
				"Delete job",
				fmt.Sprintf(
					"Are you sure you want to delete the %s job?\n\nThis action is not recoverable.",
					r.styles.Text.Bold(true).Render(jobName),
				),
				entry.JID(),
				r.styles.DangerAction,
			),
		}
	}
}

func (r *Retries) openKillConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := r.jobName(entry)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				r.styles,
				"Kill job",
				fmt.Sprintf(
					"Are you sure you want to kill the %s job?\n\nThis will move the job to the dead queue.",
					r.styles.Text.Bold(true).Render(jobName),
				),
				entry.JID(),
				r.styles.DangerAction,
			),
		}
	}
}

func (r *Retries) openRetryNowConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := r.jobName(entry)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				r.styles,
				"Retry job",
				fmt.Sprintf(
					"Retry the %s job now?\n\nThis will enqueue it immediately.",
					r.styles.Text.Bold(true).Render(jobName),
				),
				entry.JID(),
				r.styles.DangerAction,
			),
		}
	}
}

func (r *Retries) openDeleteAllConfirm() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				r.styles,
				"Delete all retries",
				"Are you sure you want to delete all retry jobs?\n\nThis action is not recoverable.",
				"retries.delete_all",
				r.styles.DangerAction,
			),
		}
	}
}

func (r *Retries) openKillAllConfirm() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				r.styles,
				"Kill all retries",
				"Are you sure you want to kill all retry jobs?\n\nThis will move them to the dead queue.",
				"retries.kill_all",
				r.styles.DangerAction,
			),
		}
	}
}

func (r *Retries) openRetryAllConfirm() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				r.styles,
				"Retry all retries",
				"Retry all retry jobs now?\n\nThis will enqueue them immediately.",
				"retries.retry_all",
				r.styles.DangerAction,
			),
		}
	}
}

func (r *Retries) deleteJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.deleteJobCmd")
		if err := r.client.DeleteSortedEntry(ctx, sidekiq.SortedSetRetry, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) deleteAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.deleteAllCmd")
		if err := r.client.DeleteAllSortedEntries(ctx, sidekiq.SortedSetRetry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) killAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.killAllCmd")
		if err := r.client.MoveAllSortedEntriesToDead(ctx, sidekiq.SortedSetRetry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) retryAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.retryAllCmd")
		if err := r.client.EnqueueAllSortedEntries(ctx, sidekiq.SortedSetRetry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) killJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.killJobCmd")
		if err := r.client.MoveSortedEntryToDead(ctx, sidekiq.SortedSetRetry, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) retryNowJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.retryNowJobCmd")
		if err := r.client.EnqueueSortedEntry(ctx, sidekiq.SortedSetRetry, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
// renderJobDetail renders the job detail view.
