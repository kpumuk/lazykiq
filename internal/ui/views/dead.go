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
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
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

type deadPayload struct {
	jobs       []*sidekiq.SortedEntry
	firstEntry *sidekiq.SortedEntry
	lastEntry  *sidekiq.SortedEntry
}

// Dead shows dead/morgue jobs.
type Dead struct {
	client                  sidekiq.API
	width                   int
	height                  int
	styles                  Styles
	jobs                    []*sidekiq.SortedEntry
	firstEntry              *sidekiq.SortedEntry
	lastEntry               *sidekiq.SortedEntry
	lazy                    lazytable.Model
	ready                   bool
	filter                  string
	dangerousActionsEnabled bool
	frameStyles             frame.Styles
	filterStyle             filterdialog.Styles
	pendingConfirm          pendingConfirm[deadJobAction]
}

// NewDead creates a new Dead view.
func NewDead(client sidekiq.API) *Dead {
	d := &Dead{
		client: client,
		lazy: lazytable.New(
			lazytable.WithTableOptions(
				table.WithColumns(deadJobColumns),
				table.WithEmptyMessage("No dead jobs"),
			),
			lazytable.WithWindowPages(deadWindowPages),
			lazytable.WithFallbackPageSize(deadFallbackPageSize),
		),
	}
	d.lazy.SetFetcher(d.fetchWindow)
	d.lazy.SetErrorHandler(func(err error) tea.Msg {
		return ConnectionErrorMsg{Err: err}
	})
	return d
}

// Init implements View.
func (d *Dead) Init() tea.Cmd {
	d.reset()
	return d.lazy.RequestWindow(0, lazytable.CursorStart)
}

// Update implements View.
func (d *Dead) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if msg.RequestID != d.lazy.RequestID() {
			return d, nil
		}
		if payload, ok := msg.Result.Payload.(deadPayload); ok {
			d.jobs = payload.jobs
			d.firstEntry = payload.firstEntry
			d.lastEntry = payload.lastEntry
		}
		d.ready = true
		var cmd tea.Cmd
		d.lazy, cmd = d.lazy.Update(msg)
		return d, cmd

	case RefreshMsg:
		if d.lazy.Loading() {
			return d, nil
		}
		return d, d.lazy.RequestWindow(d.lazy.WindowStart(), lazytable.CursorKeep)

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone || msg.Query == d.filter {
			return d, nil
		}
		d.filter = msg.Query
		d.updateEmptyMessage()
		d.lazy.Table().SetCursor(0)
		return d, d.lazy.RequestWindow(0, lazytable.CursorStart)

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

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return d, d.openFilterDialog()
		case "ctrl+u":
			if d.filter == "" {
				return d, nil
			}
			d.filter = ""
			d.updateEmptyMessage()
			d.lazy.Table().SetCursor(0)
			return d, d.lazy.RequestWindow(0, lazytable.CursorStart)
		case "c":
			if entry, ok := d.selectedEntry(); ok {
				return d, copyTextCmd(entry.JID())
			}
			return d, nil
		}

		switch msg.String() {
		case "alt+left", "[":
			if d.filter == "" {
				d.lazy.MovePage(-1)
				return d, d.lazy.MaybePrefetch()
			}
			return d, nil
		case "alt+right", "]":
			if d.filter == "" {
				d.lazy.MovePage(1)
				return d, d.lazy.MaybePrefetch()
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

		var cmd tea.Cmd
		d.lazy, cmd = d.lazy.Update(msg)
		return d, cmd
	}

	return d, nil
}

// View implements View.
func (d *Dead) View() string {
	if !d.ready {
		return d.renderMessage("Loading...")
	}

	if len(d.jobs) == 0 && d.lazy.Total() == 0 && d.filter == "" {
		return d.renderMessage("No dead jobs")
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
		lastFailed = format.Duration(int64(now.Sub(d.lastEntry.At()).Seconds()))
	}
	if d.firstEntry != nil {
		oldestFailed = format.Duration(int64(now.Sub(d.firstEntry.At()).Seconds()))
	}

	items := []ContextItem{
		{Label: "Last failed", Value: lastFailed},
		{Label: "Oldest failed", Value: oldestFailed},
		{Label: "Total items", Value: format.Number(d.lazy.Total())},
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
	return tableHelpBindings(d.lazy.Table().KeyMap)
}

// SetSize implements View.
func (d *Dead) SetSize(width, height int) View {
	d.width = width
	d.height = height
	d.updateTableSize()
	return d
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (d *Dead) SetDangerousActionsEnabled(enabled bool) {
	d.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (d *Dead) Dispose() {
	d.reset()
	d.filter = ""
	d.SetStyles(d.styles)
	d.updateTableSize()
}

// SetStyles implements View.
func (d *Dead) SetStyles(styles Styles) View {
	d.styles = styles
	d.frameStyles = frameStylesFromTheme(styles)
	d.filterStyle = filterDialogStylesFromTheme(styles)
	d.lazy.SetSpinnerStyle(styles.Muted)
	d.lazy.SetTableStyles(tableStylesFromTheme(styles))
	return d
}

func (d *Dead) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	ctx = devtools.WithTracker(ctx, "dead.fetchWindow")
	result, err := fetchSortedWindow(ctx, sortedWindowConfig{
		filter:           d.filter,
		windowStart:      windowStart,
		windowSize:       windowSize,
		fallbackPageSize: deadFallbackPageSize,
		windowPages:      deadWindowPages,
		scan:             d.client.ScanDeadJobs,
		fetch:            d.client.GetDeadJobs,
		bounds:           d.client.GetDeadBounds,
	})
	if err != nil {
		return lazytable.FetchResult{}, err
	}

	return lazytable.FetchResult{
		Rows:        d.buildRows(result.jobs),
		Total:       result.total,
		WindowStart: result.windowStart,
		Payload: deadPayload{
			jobs:       result.jobs,
			firstEntry: result.firstEntry,
			lastEntry:  result.lastEntry,
		},
	}, nil
}

func (d *Dead) renderMessage(msg string) string {
	return renderStatusMessage("Dead Jobs", msg, d.styles, d.width, d.height)
}

func (d *Dead) reset() {
	d.jobs = nil
	d.firstEntry = nil
	d.lastEntry = nil
	d.ready = false
	d.lazy.Reset()
	d.updateEmptyMessage()
}

func (d *Dead) selectedEntry() (*sidekiq.SortedEntry, bool) {
	idx := d.lazy.Table().Cursor()
	if idx < 0 || idx >= len(d.jobs) {
		return nil, false
	}
	return d.jobs[idx], true
}

// Table columns for dead job list.
var deadJobColumns = []table.Column{
	{Title: "Last Retry", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (d *Dead) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(d.height-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := d.width - 4
	d.lazy.SetSize(tableWidth, tableHeight)
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
		lastRetry := format.Duration(int64(now.Sub(job.At()).Seconds()))

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
				format.Args(job.DisplayArgs()),
				errorStr,
			},
		})
	}
	return rows
}

func (d *Dead) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(d.filterStyle),
				filterdialog.WithQuery(d.filter),
			),
		}
	}
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
		if err := d.client.DeleteDeadJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (d *Dead) deleteAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "dead.deleteAllCmd")
		if err := d.client.DeleteAllDeadJobs(ctx); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (d *Dead) retryNowJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "dead.retryNowJobCmd")
		if err := d.client.RetryNowDeadJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (d *Dead) retryAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "dead.retryAllCmd")
		if err := d.client.RetryAllDeadJobs(ctx); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (d *Dead) rowsMeta() string {
	start, end, total := d.lazy.Range()
	label := d.styles.MetricLabel.Render("rows: ")
	if total == 0 || len(d.jobs) == 0 {
		return label + d.styles.MetricValue.Render("0/0")
	}

	rangeLabel := fmt.Sprintf(
		"%s-%s/%s",
		format.Number(int64(start)),
		format.Number(int64(end)),
		format.Number(total),
	)
	return label + d.styles.MetricValue.Render(rangeLabel)
}

// renderJobsBox renders the bordered box containing the jobs table.
func (d *Dead) renderJobsBox() string {
	return frame.New(
		frame.WithStyles(d.frameStyles),
		frame.WithTitle("Dead Jobs"),
		frame.WithFilter(d.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(d.rowsMeta()),
		frame.WithContent(d.lazy.View()),
		frame.WithPadding(1),
		frame.WithSize(d.width, d.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	).View()
}

func (d *Dead) jobName(entry *sidekiq.SortedEntry) string {
	if name := entry.DisplayClass(); name != "" {
		return name
	}
	return "selected"
}

// renderJobDetail renders the job detail view.
