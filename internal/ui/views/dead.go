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
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

const deadPageSize = 25

// deadDataMsg carries dead jobs data internally.
type deadDataMsg struct {
	jobs        []*sidekiq.SortedEntry
	currentPage int
	totalPages  int
	totalSize   int64
}

// Dead shows dead/morgue jobs.
type Dead struct {
	client      sidekiq.API
	width       int
	height      int
	styles      Styles
	jobs        []*sidekiq.SortedEntry
	table       table.Model
	ready       bool
	currentPage int
	totalPages  int
	totalSize   int64
	filter      string
	frameStyles frame.Styles
	filterStyle filterdialog.Styles
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

// SetSize implements View.
func (d *Dead) SetSize(width, height int) View {
	d.width = width
	d.height = height
	d.updateTableSize()
	return d
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

			return deadDataMsg{
				jobs:        jobs,
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
	d.ready = false
	d.table.SetRows(nil)
	d.table.SetCursor(0)
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
				job.Queue(),
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

// renderJobsBox renders the bordered box containing the jobs table.
func (d *Dead) renderJobsBox() string {
	// Build meta: SIZE and PAGE info
	sep := d.styles.Muted.Render(" • ")
	sizeInfo := d.styles.MetricLabel.Render("SIZE: ") + d.styles.MetricValue.Render(format.ShortNumber(d.totalSize))
	pageInfo := d.styles.MetricLabel.Render("PAGE: ") + d.styles.MetricValue.Render(fmt.Sprintf("%d/%d", d.currentPage, d.totalPages))
	meta := sizeInfo + sep + pageInfo

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
