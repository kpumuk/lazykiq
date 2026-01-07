package views

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

const deadPageSize = 25

// deadDataMsg carries dead jobs data internally.
type deadDataMsg struct {
	jobs        []*sidekiq.SortedEntry
	firstEntry  *sidekiq.SortedEntry
	lastEntry   *sidekiq.SortedEntry
	currentPage int
	totalPages  int
	totalSize   int64
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
	table                   table.Model
	ready                   bool
	currentPage             int
	totalPages              int
	totalSize               int64
	filter                  string
	dangerousActionsEnabled bool
	frameStyles             frame.Styles
	filterStyle             filterdialog.Styles
	pendingJobEntry         *sidekiq.SortedEntry
	pendingJobTarget        string
}

// NewDead creates a new Dead view.
func NewDead(client sidekiq.API) *Dead {
	return &Dead{
		client:      client,
		currentPage: 1,
		totalPages:  1,
		table: table.New(
			table.WithColumns(deadJobColumns),
			table.WithEmptyMessage("No dead jobs"),
		),
	}
}

// Init implements View.
func (d *Dead) Init() tea.Cmd {
	d.reset()
	return d.fetchDataCmd()
}

// Update implements View.
func (d *Dead) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case deadDataMsg:
		d.jobs = msg.jobs
		d.firstEntry = msg.firstEntry
		d.lastEntry = msg.lastEntry
		d.currentPage = msg.currentPage
		d.totalPages = msg.totalPages
		d.totalSize = msg.totalSize
		d.ready = true
		d.updateTableRows()
		return d, nil

	case RefreshMsg:
		return d, d.fetchDataCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return d, nil
		}
		if msg.Query == d.filter {
			return d, nil
		}
		d.filter = msg.Query
		d.currentPage = 1
		d.table.SetCursor(0)
		return d, d.fetchDataCmd()

	case confirmdialog.ActionMsg:
		if !d.dangerousActionsEnabled {
			return d, nil
		}
		if d.pendingJobEntry == nil {
			return d, nil
		}
		if d.pendingJobTarget != "" && msg.Target != d.pendingJobTarget {
			return d, nil
		}
		entry := d.pendingJobEntry
		d.pendingJobEntry = nil
		d.pendingJobTarget = ""
		if !msg.Confirmed {
			return d, nil
		}
		return d, d.deleteJobCmd(entry)

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return d, d.openFilterDialog()
		case "ctrl+u":
			if d.filter != "" {
				d.filter = ""
				d.currentPage = 1
				d.table.SetCursor(0)
				return d, d.fetchDataCmd()
			}
			return d, nil
		}

		switch msg.String() {
		case "alt+left", "[":
			if d.filter != "" {
				return d, nil
			}
			if d.currentPage > 1 {
				d.currentPage--
				return d, d.fetchDataCmd()
			}
			return d, nil
		case "alt+right", "]":
			if d.filter != "" {
				return d, nil
			}
			if d.currentPage < d.totalPages {
				d.currentPage++
				return d, d.fetchDataCmd()
			}
			return d, nil
		case "enter":
			// Show detail for selected job
			if idx := d.table.Cursor(); idx >= 0 && idx < len(d.jobs) {
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
					d.pendingJobEntry = entry
					d.pendingJobTarget = entry.JID()
					return d, d.openDeleteConfirm(entry)
				}
				return d, nil
			}
		}

		d.table, _ = d.table.Update(msg)
		return d, nil
	}

	return d, nil
}

// View implements View.
func (d *Dead) View() string {
	if !d.ready {
		return d.renderMessage("Loading...")
	}

	if len(d.jobs) == 0 && d.totalSize == 0 && d.filter == "" {
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
	now := time.Now().Unix()
	lastFailed := "-"
	oldestFailed := "-"
	if d.lastEntry != nil {
		lastFailed = format.Duration(now - d.lastEntry.At())
	}
	if d.firstEntry != nil {
		oldestFailed = format.Duration(now - d.firstEntry.At())
	}

	items := []ContextItem{
		{Label: "Last failed", Value: lastFailed},
		{Label: "Oldest failed", Value: oldestFailed},
		{Label: "Total items", Value: format.Number(d.totalSize)},
	}
	return items
}

// HintBindings implements HintProvider.
func (d *Dead) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "change page"),
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
				helpBinding([]string{"["}, "[", "previous page"),
				helpBinding([]string{"]"}, "]", "next page"),
				helpBinding([]string{"enter"}, "enter", "job detail"),
			},
		},
	}
	if d.dangerousActionsEnabled {
		sections = append(sections, HelpSection{
			Title: "Dangerous Actions",
			Bindings: []key.Binding{
				helpBinding([]string{"D"}, "shift+d", "delete job"),
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (d *Dead) TableHelp() []key.Binding {
	return tableHelpBindings(d.table.KeyMap)
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
	d.frameStyles = frame.Styles{
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
	d.filterStyle = filterdialog.Styles{
		Title:       styles.Title,
		Border:      styles.FocusBorder,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	}
	d.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	return d
}

// fetchDataCmd fetches dead jobs data from Redis.
func (d *Dead) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		if d.filter != "" {
			jobs, err := d.client.ScanDeadJobs(ctx, d.filter)
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}
			firstEntry, lastEntry := sortedEntryBounds(jobs)

			return deadDataMsg{
				jobs:        jobs,
				firstEntry:  firstEntry,
				lastEntry:   lastEntry,
				currentPage: 1,
				totalPages:  1,
				totalSize:   int64(len(jobs)),
			}
		}

		currentPage := d.currentPage
		totalPages := 1

		start := (currentPage - 1) * deadPageSize
		jobs, totalSize, err := d.client.GetDeadJobs(ctx, start, deadPageSize)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		var firstEntry *sidekiq.SortedEntry
		var lastEntry *sidekiq.SortedEntry
		if totalSize > 0 {
			firstEntry, lastEntry, err = d.client.GetDeadBounds(ctx)
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}
		}

		if totalSize > 0 {
			totalPages = int((totalSize + deadPageSize - 1) / deadPageSize)
		}

		if currentPage > totalPages {
			currentPage = totalPages
		}
		if currentPage < 1 {
			currentPage = 1
		}

		return deadDataMsg{
			jobs:        jobs,
			firstEntry:  firstEntry,
			lastEntry:   lastEntry,
			currentPage: currentPage,
			totalPages:  totalPages,
			totalSize:   totalSize,
		}
	}
}

func (d *Dead) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  d.styles.Title,
		Muted:  d.styles.Muted,
		Border: d.styles.FocusBorder,
	}, "Dead Jobs", msg, d.width, d.height)
}

func (d *Dead) reset() {
	d.currentPage = 1
	d.totalPages = 1
	d.totalSize = 0
	d.jobs = nil
	d.firstEntry = nil
	d.lastEntry = nil
	d.ready = false
	d.table.SetRows(nil)
	d.table.SetCursor(0)
}

func (d *Dead) selectedEntry() (*sidekiq.SortedEntry, bool) {
	idx := d.table.Cursor()
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
	d.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows.
func (d *Dead) updateTableRows() {
	if d.filter != "" {
		d.table.SetEmptyMessage("No matches")
	} else {
		d.table.SetEmptyMessage("No dead jobs")
	}

	rows := make([]table.Row, 0, len(d.jobs))
	now := time.Now().Unix()
	for _, job := range d.jobs {
		// Format "last retry" as relative time
		lastRetry := format.Duration(now - job.At())

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
				lastRetry,
				d.styles.QueueText.Render(job.Queue()),
				job.DisplayClass(),
				format.Args(job.DisplayArgs()),
				errorStr,
			},
		}
		rows = append(rows, row)
	}
	d.table.SetRows(rows)
	d.updateTableSize()
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
	jobName := entry.DisplayClass()
	if jobName == "" {
		jobName = "selected"
	}
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: confirmdialog.New(
				confirmdialog.WithStyles(confirmdialog.Styles{
					Title:           d.styles.Title,
					Border:          d.styles.FocusBorder,
					Text:            d.styles.Text,
					Muted:           d.styles.Muted,
					Button:          d.styles.Muted.Padding(0, 1),
					ButtonYesActive: d.styles.DangerAction,
					ButtonNoActive:  d.styles.NeutralAction,
				}),
				confirmdialog.WithTitle("Delete job"),
				confirmdialog.WithMessage(fmt.Sprintf(
					"Are you sure you want to delete the %s job?\n\nThis action is not recoverable.",
					d.styles.Text.Bold(true).Render(jobName),
				)),
				confirmdialog.WithTarget(entry.JID()),
			),
		}
	}
}

func (d *Dead) deleteJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := d.client.DeleteDeadJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
func (d *Dead) renderJobsBox() string {
	// Build meta: page only
	pageInfo := d.styles.MetricLabel.Render("page: ") + d.styles.MetricValue.Render(fmt.Sprintf("%d/%d", d.currentPage, d.totalPages))
	meta := pageInfo

	// Get table content
	content := d.table.View()

	box := frame.New(
		frame.WithStyles(d.frameStyles),
		frame.WithTitle("Dead Jobs"),
		frame.WithFilter(d.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(d.width, d.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

// renderJobDetail renders the job detail view.
