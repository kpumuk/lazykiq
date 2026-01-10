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
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

type retriesPayload struct {
	jobs       []*sidekiq.SortedEntry
	firstEntry *sidekiq.SortedEntry
	lastEntry  *sidekiq.SortedEntry
}

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
)

// Retries shows failed jobs pending retry.
type Retries struct {
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
	pendingJobAction        retriesJobAction
	pendingJobEntry         *sidekiq.SortedEntry
	pendingJobTarget        string
}

// NewRetries creates a new Retries view.
func NewRetries(client sidekiq.API) *Retries {
	r := &Retries{
		client: client,
		lazy: lazytable.New(
			lazytable.WithTableOptions(
				table.WithColumns(retryJobColumns),
				table.WithEmptyMessage("No retries"),
			),
			lazytable.WithWindowPages(retriesWindowPages),
			lazytable.WithFallbackPageSize(retriesFallbackPageSize),
		),
	}
	r.lazy.SetFetcher(r.fetchWindow)
	r.lazy.SetErrorHandler(func(err error) tea.Msg {
		return ConnectionErrorMsg{Err: err}
	})
	return r
}

// Init implements View.
func (r *Retries) Init() tea.Cmd {
	r.reset()
	return r.lazy.RequestWindow(0, lazytable.CursorStart)
}

// Update implements View.
func (r *Retries) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if msg.RequestID != r.lazy.RequestID() {
			return r, nil
		}
		if payload, ok := msg.Result.Payload.(retriesPayload); ok {
			r.jobs = payload.jobs
			r.firstEntry = payload.firstEntry
			r.lastEntry = payload.lastEntry
		}
		r.ready = true
		var cmd tea.Cmd
		r.lazy, cmd = r.lazy.Update(msg)
		return r, cmd

	case RefreshMsg:
		if r.lazy.Loading() {
			return r, nil
		}
		return r, r.lazy.RequestWindow(r.lazy.WindowStart(), lazytable.CursorKeep)

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone || msg.Query == r.filter {
			return r, nil
		}
		r.filter = msg.Query
		r.updateEmptyMessage()
		r.lazy.Table().SetCursor(0)
		return r, r.lazy.RequestWindow(0, lazytable.CursorStart)

	case confirmdialog.ActionMsg:
		if !r.dangerousActionsEnabled || r.pendingJobEntry == nil || (r.pendingJobTarget != "" && msg.Target != r.pendingJobTarget) {
			return r, nil
		}
		action := r.pendingJobAction
		entry := r.pendingJobEntry
		r.pendingJobAction = retriesJobActionNone
		r.pendingJobEntry = nil
		r.pendingJobTarget = ""
		if !msg.Confirmed {
			return r, nil
		}
		switch action {
		case retriesJobActionNone:
			return r, nil
		case retriesJobActionDelete:
			return r, r.deleteJobCmd(entry)
		case retriesJobActionKill:
			return r, r.killJobCmd(entry)
		case retriesJobActionRetry:
			return r, r.retryNowJobCmd(entry)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return r, r.openFilterDialog()
		case "ctrl+u":
			if r.filter == "" {
				return r, nil
			}
			r.filter = ""
			r.updateEmptyMessage()
			r.lazy.Table().SetCursor(0)
			return r, r.lazy.RequestWindow(0, lazytable.CursorStart)
		case "c":
			if entry, ok := r.selectedEntry(); ok {
				return r, copyTextCmd(entry.JID())
			}
			return r, nil
		}

		switch msg.String() {
		case "alt+left", "[":
			if r.filter == "" {
				r.lazy.MovePage(-1)
				return r, r.lazy.MaybePrefetch()
			}
			return r, nil
		case "alt+right", "]":
			if r.filter == "" {
				r.lazy.MovePage(1)
				return r, r.lazy.MaybePrefetch()
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
				if entry, ok := r.selectedEntry(); ok {
					r.pendingJobAction = retriesJobActionDelete
					r.pendingJobEntry = entry
					r.pendingJobTarget = entry.JID()
					return r, r.openDeleteConfirm(entry)
				}
				return r, nil
			case "K":
				if entry, ok := r.selectedEntry(); ok {
					r.pendingJobAction = retriesJobActionKill
					r.pendingJobEntry = entry
					r.pendingJobTarget = entry.JID()
					return r, r.openKillConfirm(entry)
				}
				return r, nil
			case "R":
				if entry, ok := r.selectedEntry(); ok {
					r.pendingJobAction = retriesJobActionRetry
					r.pendingJobEntry = entry
					r.pendingJobTarget = entry.JID()
					return r, r.openRetryNowConfirm(entry)
				}
				return r, nil
			}
		}

		var cmd tea.Cmd
		r.lazy, cmd = r.lazy.Update(msg)
		return r, cmd
	}

	return r, nil
}

// View implements View.
func (r *Retries) View() string {
	if !r.ready {
		return r.renderMessage("Loading...")
	}

	if len(r.jobs) == 0 && r.lazy.Total() == 0 && r.filter == "" {
		return r.renderMessage("No retries")
	}

	return r.renderJobsBox()
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
		nextRetry = format.Duration(int64(r.firstEntry.At().Sub(now).Seconds()))
	}
	if r.lastEntry != nil {
		latestRetry = format.Duration(int64(r.lastEntry.At().Sub(now).Seconds()))
	}

	items := []ContextItem{
		{Label: "Next retry in", Value: nextRetry},
		{Label: "Latest retry in", Value: latestRetry},
		{Label: "Total items", Value: format.Number(r.lazy.Total())},
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
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (r *Retries) TableHelp() []key.Binding {
	return tableHelpBindings(r.lazy.Table().KeyMap)
}

// SetSize implements View.
func (r *Retries) SetSize(width, height int) View {
	r.width = width
	r.height = height
	r.updateTableSize()
	return r
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (r *Retries) SetDangerousActionsEnabled(enabled bool) {
	r.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (r *Retries) Dispose() {
	r.reset()
	r.filter = ""
	r.SetStyles(r.styles)
	r.updateTableSize()
}

// SetStyles implements View.
func (r *Retries) SetStyles(styles Styles) View {
	r.styles = styles
	r.frameStyles = frame.Styles{
		Focused: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterFocused,
			Border: styles.FocusBorder,
		},
		Blurred: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterBlurred,
			Border: styles.BorderStyle,
		},
	}
	r.filterStyle = filterdialog.Styles{
		Title:       styles.Title,
		Border:      styles.FocusBorder,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	}
	r.lazy.SetSpinnerStyle(styles.Muted)
	r.lazy.SetTableStyles(table.Styles{
		Text:           styles.Text,
		Muted:          styles.Muted,
		Header:         styles.TableHeader,
		Selected:       styles.TableSelected,
		Separator:      styles.TableSeparator,
		ScrollbarTrack: styles.ScrollbarTrack,
		ScrollbarThumb: styles.ScrollbarThumb,
	})
	return r
}

func (r *Retries) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	ctx = devtools.WithTracker(ctx, "retries.fetchWindow")

	if r.filter != "" {
		jobs, err := r.client.ScanRetryJobs(ctx, r.filter)
		if err != nil {
			return lazytable.FetchResult{}, err
		}
		firstEntry, lastEntry := sortedEntryBounds(jobs)
		return lazytable.FetchResult{
			Rows:        r.buildRows(jobs),
			Total:       int64(len(jobs)),
			WindowStart: 0,
			Payload: retriesPayload{
				jobs:       jobs,
				firstEntry: firstEntry,
				lastEntry:  lastEntry,
			},
		}, nil
	}

	if windowSize <= 0 {
		windowSize = max(retriesFallbackPageSize, 1) * retriesWindowPages
	}

	jobs, totalSize, err := r.client.GetRetryJobs(ctx, windowStart, windowSize)
	if err != nil {
		return lazytable.FetchResult{}, err
	}

	if totalSize > 0 {
		maxStart := max(int(totalSize)-windowSize, 0)
		if windowStart > maxStart {
			windowStart = maxStart
			jobs, totalSize, err = r.client.GetRetryJobs(ctx, windowStart, windowSize)
			if err != nil {
				return lazytable.FetchResult{}, err
			}
		}
	} else {
		windowStart = 0
	}

	var firstEntry *sidekiq.SortedEntry
	var lastEntry *sidekiq.SortedEntry
	if totalSize > 0 {
		firstEntry, lastEntry, err = r.client.GetRetryBounds(ctx)
		if err != nil {
			return lazytable.FetchResult{}, err
		}
	}

	return lazytable.FetchResult{
		Rows:        r.buildRows(jobs),
		Total:       totalSize,
		WindowStart: windowStart,
		Payload: retriesPayload{
			jobs:       jobs,
			firstEntry: firstEntry,
			lastEntry:  lastEntry,
		},
	}, nil
}

func (r *Retries) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  r.styles.Title,
		Muted:  r.styles.Muted,
		Border: r.styles.FocusBorder,
	}, "Retries", msg, r.width, r.height)
}

func (r *Retries) reset() {
	r.jobs = nil
	r.firstEntry = nil
	r.lastEntry = nil
	r.ready = false
	r.lazy.Reset()
	r.updateEmptyMessage()
}

func (r *Retries) selectedEntry() (*sidekiq.SortedEntry, bool) {
	idx := r.lazy.Table().Cursor()
	if idx < 0 || idx >= len(r.jobs) {
		return nil, false
	}
	return r.jobs[idx], true
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

// updateTableSize updates the table dimensions based on current view size.
func (r *Retries) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(r.height-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := r.width - 4
	r.lazy.SetSize(tableWidth, tableHeight)
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
		nextRetry := format.Duration(int64(now.Sub(job.At()).Seconds()))
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
				format.Args(job.DisplayArgs()),
				errorStr,
			},
		})
	}
	return rows
}

func (r *Retries) rowsMeta() string {
	start, end, total := r.lazy.Range()
	label := r.styles.MetricLabel.Render("rows: ")
	if total == 0 || len(r.jobs) == 0 {
		return label + r.styles.MetricValue.Render("0/0")
	}

	rangeLabel := fmt.Sprintf(
		"%s-%s/%s",
		format.Number(int64(start)),
		format.Number(int64(end)),
		format.Number(total),
	)
	return label + r.styles.MetricValue.Render(rangeLabel)
}

func (r *Retries) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(r.filterStyle),
				filterdialog.WithQuery(r.filter),
			),
		}
	}
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

func (r *Retries) deleteJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.deleteJobCmd")
		if err := r.client.DeleteRetryJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) killJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.killJobCmd")
		if err := r.client.KillRetryJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) retryNowJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "retries.retryNowJobCmd")
		if err := r.client.RetryNowRetryJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
func (r *Retries) renderJobsBox() string {
	return frame.New(
		frame.WithStyles(r.frameStyles),
		frame.WithTitle("Retries"),
		frame.WithFilter(r.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(r.rowsMeta()),
		frame.WithContent(r.lazy.View()),
		frame.WithPadding(1),
		frame.WithSize(r.width, r.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	).View()
}

func (r *Retries) jobName(entry *sidekiq.SortedEntry) string {
	if name := entry.DisplayClass(); name != "" {
		return name
	}
	return "selected"
}

// renderJobDetail renders the job detail view.
