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
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// retriesDataMsg is internal to the Retries view.
type retriesDataMsg struct {
	jobs        []*sidekiq.SortedEntry
	firstEntry  *sidekiq.SortedEntry
	lastEntry   *sidekiq.SortedEntry
	currentPage int
	totalPages  int
	totalSize   int64
}

const retriesPageSize = 25

type retriesJobAction int

const (
	retriesJobActionNone retriesJobAction = iota
	retriesJobActionDelete
	retriesJobActionKill
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
	table                   table.Model
	ready                   bool
	currentPage             int
	totalPages              int
	totalSize               int64
	filter                  string
	dangerousActionsEnabled bool
	frameStyles             frame.Styles
	filterStyle             filterdialog.Styles
	pendingJobAction        retriesJobAction
	pendingJobEntry         *sidekiq.SortedEntry
	pendingJobTarget        string
	devTracker              *devtools.Tracker
	devKey                  string
}

// NewRetries creates a new Retries view.
func NewRetries(client sidekiq.API) *Retries {
	return &Retries{
		client:      client,
		currentPage: 1,
		totalPages:  1,
		table: table.New(
			table.WithColumns(retryJobColumns),
			table.WithEmptyMessage("No retries"),
		),
	}
}

// Init implements View.
func (r *Retries) Init() tea.Cmd {
	r.reset()
	return r.fetchDataCmd()
}

// Update implements View.
func (r *Retries) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case retriesDataMsg:
		r.jobs = msg.jobs
		r.firstEntry = msg.firstEntry
		r.lastEntry = msg.lastEntry
		r.currentPage = msg.currentPage
		r.totalPages = msg.totalPages
		r.totalSize = msg.totalSize
		r.ready = true
		r.updateTableRows()
		return r, nil

	case RefreshMsg:
		return r, r.fetchDataCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return r, nil
		}
		if msg.Query == r.filter {
			return r, nil
		}
		r.filter = msg.Query
		r.currentPage = 1
		r.table.SetCursor(0)
		return r, r.fetchDataCmd()

	case confirmdialog.ActionMsg:
		if !r.dangerousActionsEnabled {
			return r, nil
		}
		if r.pendingJobEntry == nil {
			return r, nil
		}
		if r.pendingJobTarget != "" && msg.Target != r.pendingJobTarget {
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
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return r, r.openFilterDialog()
		case "ctrl+u":
			if r.filter != "" {
				r.filter = ""
				r.currentPage = 1
				r.table.SetCursor(0)
				return r, r.fetchDataCmd()
			}
			return r, nil
		case "c":
			if entry, ok := r.selectedEntry(); ok {
				return r, copyTextCmd(entry.JID())
			}
			return r, nil
		}

		switch msg.String() {
		case "alt+left", "[":
			if r.filter != "" {
				return r, nil
			}
			if r.currentPage > 1 {
				r.currentPage--
				return r, r.fetchDataCmd()
			}
			return r, nil
		case "alt+right", "]":
			if r.filter != "" {
				return r, nil
			}
			if r.currentPage < r.totalPages {
				r.currentPage++
				return r, r.fetchDataCmd()
			}
			return r, nil
		case "enter":
			// Show detail for selected job
			if idx := r.table.Cursor(); idx >= 0 && idx < len(r.jobs) {
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
			}
		}

		// Pass other keys to table for navigation
		r.table, _ = r.table.Update(msg)
		return r, nil
	}

	return r, nil
}

// View implements View.
func (r *Retries) View() string {
	if !r.ready {
		return r.renderMessage("Loading...")
	}

	if len(r.jobs) == 0 && r.totalSize == 0 && r.filter == "" {
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
	now := time.Now().Unix()
	nextRetry := "-"
	latestRetry := "-"
	if r.firstEntry != nil {
		nextRetry = format.Duration(r.firstEntry.At() - now)
	}
	if r.lastEntry != nil {
		latestRetry = format.Duration(r.lastEntry.At() - now)
	}

	items := []ContextItem{
		{Label: "Next retry in", Value: nextRetry},
		{Label: "Latest retry in", Value: latestRetry},
		{Label: "Total items", Value: format.Number(r.totalSize)},
	}
	return items
}

// HintBindings implements HintProvider.
func (r *Retries) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "change page"),
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
				helpBinding([]string{"["}, "[", "previous page"),
				helpBinding([]string{"]"}, "]", "next page"),
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
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (r *Retries) TableHelp() []key.Binding {
	return tableHelpBindings(r.table.KeyMap)
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
	r.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	return r
}

// SetDevelopment configures development tracking.
func (r *Retries) SetDevelopment(tracker *devtools.Tracker, key string) {
	r.devTracker = tracker
	r.devKey = key
}

// fetchDataCmd fetches retry jobs data from Redis.
func (r *Retries) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, finish := devContext(r.devTracker, r.devKey)
		defer finish()

		if r.filter != "" {
			jobs, err := r.client.ScanRetryJobs(ctx, r.filter)
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}
			firstEntry, lastEntry := sortedEntryBounds(jobs)

			return retriesDataMsg{
				jobs:        jobs,
				firstEntry:  firstEntry,
				lastEntry:   lastEntry,
				currentPage: 1,
				totalPages:  1,
				totalSize:   int64(len(jobs)),
			}
		}

		currentPage := r.currentPage
		totalPages := 1

		start := (currentPage - 1) * retriesPageSize
		jobs, totalSize, err := r.client.GetRetryJobs(ctx, start, retriesPageSize)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		var firstEntry *sidekiq.SortedEntry
		var lastEntry *sidekiq.SortedEntry
		if totalSize > 0 {
			firstEntry, lastEntry, err = r.client.GetRetryBounds(ctx)
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}
		}

		if totalSize > 0 {
			totalPages = int((totalSize + retriesPageSize - 1) / retriesPageSize)
		}

		if currentPage > totalPages {
			currentPage = totalPages
		}
		if currentPage < 1 {
			currentPage = 1
		}

		return retriesDataMsg{
			jobs:        jobs,
			firstEntry:  firstEntry,
			lastEntry:   lastEntry,
			currentPage: currentPage,
			totalPages:  totalPages,
			totalSize:   totalSize,
		}
	}
}

func (r *Retries) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  r.styles.Title,
		Muted:  r.styles.Muted,
		Border: r.styles.FocusBorder,
	}, "Retries", msg, r.width, r.height)
}

func (r *Retries) reset() {
	r.currentPage = 1
	r.totalPages = 1
	r.totalSize = 0
	r.jobs = nil
	r.firstEntry = nil
	r.lastEntry = nil
	r.ready = false
	r.table.SetRows(nil)
	r.table.SetCursor(0)
}

func (r *Retries) selectedEntry() (*sidekiq.SortedEntry, bool) {
	idx := r.table.Cursor()
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
	r.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows.
func (r *Retries) updateTableRows() {
	if r.filter != "" {
		r.table.SetEmptyMessage("No matches")
	} else {
		r.table.SetEmptyMessage("No retries")
	}

	rows := make([]table.Row, 0, len(r.jobs))
	now := time.Now().Unix()
	for _, job := range r.jobs {
		// Format "next retry" as relative time (negative means in the past/due)
		nextRetry := format.Duration(now - job.At())

		// Format retry count
		retryCount := strconv.Itoa(job.RetryCount())

		// Format error
		errorStr := ""
		if job.HasError() {
			errorStr = fmt.Sprintf("%s: %s", job.ErrorClass(), job.ErrorMessage())
			// Truncate if too long
			if len(errorStr) > 100 {
				errorStr = errorStr[:97] + "..."
			}
		}

		row := table.Row{
			ID: job.JID(),
			Cells: []string{
				nextRetry,
				retryCount,
				r.styles.QueueText.Render(job.Queue()),
				job.DisplayClass(),
				format.Args(job.DisplayArgs()),
				errorStr,
			},
		}
		rows = append(rows, row)
	}
	r.table.SetRows(rows)
	r.updateTableSize()
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
	jobName := entry.DisplayClass()
	if jobName == "" {
		jobName = "selected"
	}
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: confirmdialog.New(
				confirmdialog.WithStyles(confirmdialog.Styles{
					Title:           r.styles.Title,
					Border:          r.styles.FocusBorder,
					Text:            r.styles.Text,
					Muted:           r.styles.Muted,
					Button:          r.styles.Muted.Padding(0, 1),
					ButtonYesActive: r.styles.DangerAction,
					ButtonNoActive:  r.styles.NeutralAction,
				}),
				confirmdialog.WithTitle("Delete job"),
				confirmdialog.WithMessage(fmt.Sprintf(
					"Are you sure you want to delete the %s job?\n\nThis action is not recoverable.",
					r.styles.Text.Bold(true).Render(jobName),
				)),
				confirmdialog.WithTarget(entry.JID()),
			),
		}
	}
}

func (r *Retries) openKillConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := entry.DisplayClass()
	if jobName == "" {
		jobName = "selected"
	}
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: confirmdialog.New(
				confirmdialog.WithStyles(confirmdialog.Styles{
					Title:           r.styles.Title,
					Border:          r.styles.FocusBorder,
					Text:            r.styles.Text,
					Muted:           r.styles.Muted,
					Button:          r.styles.Muted.Padding(0, 1),
					ButtonYesActive: r.styles.DangerAction,
					ButtonNoActive:  r.styles.NeutralAction,
				}),
				confirmdialog.WithTitle("Kill job"),
				confirmdialog.WithMessage(fmt.Sprintf(
					"Are you sure you want to kill the %s job?\n\nThis will move the job to the dead queue.",
					r.styles.Text.Bold(true).Render(jobName),
				)),
				confirmdialog.WithTarget(entry.JID()),
			),
		}
	}
}

func (r *Retries) deleteJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := r.client.DeleteRetryJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (r *Retries) killJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := r.client.KillRetryJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
func (r *Retries) renderJobsBox() string {
	// Build meta: page only
	pageInfo := r.styles.MetricLabel.Render("page: ") + r.styles.MetricValue.Render(fmt.Sprintf("%d/%d", r.currentPage, r.totalPages))
	meta := pageInfo

	// Get table content
	content := r.table.View()

	box := frame.New(
		frame.WithStyles(r.frameStyles),
		frame.WithTitle("Retries"),
		frame.WithFilter(r.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(r.width, r.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

// renderJobDetail renders the job detail view.
