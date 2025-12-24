package views

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/jobsbox"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// DeadUpdateMsg carries dead jobs data from App to Dead view
type DeadUpdateMsg struct {
	Jobs        []*sidekiq.SortedEntry
	CurrentPage int
	TotalPages  int
	TotalSize   int64
}

// DeadPageRequestMsg requests a specific page of dead jobs
type DeadPageRequestMsg struct {
	Page int
}

// Dead shows dead/morgue jobs
type Dead struct {
	width       int
	height      int
	styles      Styles
	jobs        []*sidekiq.SortedEntry
	table       *table.Table
	ready       bool
	currentPage int
	totalPages  int
	totalSize   int64
}

// NewDead creates a new Dead view
func NewDead() *Dead {
	return &Dead{}
}

// Init implements View
func (d *Dead) Init() tea.Cmd {
	return nil
}

// Update implements View
func (d *Dead) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case DeadUpdateMsg:
		d.jobs = msg.Jobs
		d.currentPage = msg.CurrentPage
		d.totalPages = msg.TotalPages
		d.totalSize = msg.TotalSize
		d.ready = true
		d.updateTableRows()
		return d, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "alt+left", "[":
			if d.currentPage > 1 {
				return d, func() tea.Msg {
					return DeadPageRequestMsg{Page: d.currentPage - 1}
				}
			}
			return d, nil
		case "alt+right", "]":
			if d.currentPage < d.totalPages {
				return d, func() tea.Msg {
					return DeadPageRequestMsg{Page: d.currentPage + 1}
				}
			}
			return d, nil
		}

		// Pass other keys to table for navigation
		if d.table != nil {
			d.table.Update(msg)
		}
		return d, nil
	}

	return d, nil
}

// View implements View
func (d *Dead) View() string {
	if !d.ready {
		return d.renderMessage("Loading...")
	}

	if len(d.jobs) == 0 && d.totalSize == 0 {
		return d.renderMessage("No dead jobs")
	}

	return d.renderJobsBox()
}

func (d *Dead) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  d.styles.Title,
		Muted:  d.styles.Muted,
		Border: d.styles.BorderStyle,
	}, "Dead Jobs", msg, d.width, d.height)
}

// Name implements View
func (d *Dead) Name() string {
	return "Dead"
}

// ShortHelp implements View
func (d *Dead) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (d *Dead) SetSize(width, height int) View {
	d.width = width
	d.height = height
	d.updateTableSize()
	return d
}

// SetStyles implements View
func (d *Dead) SetStyles(styles Styles) View {
	d.styles = styles
	if d.table != nil {
		d.table.SetStyles(table.Styles{
			Text:      styles.Text,
			Muted:     styles.Muted,
			Header:    styles.TableHeader,
			Selected:  styles.TableSelected,
			Separator: styles.TableSeparator,
		})
	}
	return d
}

// Table columns for dead job list
var deadJobColumns = []table.Column{
	{Title: "Last Retry", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size
func (d *Dead) updateTableSize() {
	if d.table == nil {
		return
	}
	// Calculate table height: total height - box borders
	tableHeight := d.height - 2
	if tableHeight < 3 {
		tableHeight = 3
	}
	// Table width: view width - box borders - padding
	tableWidth := d.width - 4
	d.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows
func (d *Dead) updateTableRows() {
	d.ensureTable()

	rows := make([][]string, 0, len(d.jobs))
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

		row := []string{
			lastRetry,
			job.Queue(),
			job.DisplayClass(),
			format.Args(job.Args()),
			errorStr,
		}
		rows = append(rows, row)
	}
	d.table.SetRows(rows)
	d.updateTableSize()
}

// ensureTable creates the table if it doesn't exist
func (d *Dead) ensureTable() {
	if d.table != nil {
		return
	}
	d.table = table.New(deadJobColumns)
	d.table.SetEmptyMessage("No dead jobs")
	d.table.SetStyles(table.Styles{
		Text:      d.styles.Text,
		Muted:     d.styles.Muted,
		Header:    d.styles.TableHeader,
		Selected:  d.styles.TableSelected,
		Separator: d.styles.TableSeparator,
	})
}

// renderJobsBox renders the bordered box containing the jobs table
func (d *Dead) renderJobsBox() string {
	// Build meta: SIZE and PAGE info
	sep := d.styles.Muted.Render(" • ")
	sizeInfo := d.styles.MetricLabel.Render("SIZE: ") + d.styles.MetricValue.Render(format.Number(d.totalSize))
	pageInfo := d.styles.MetricLabel.Render("PAGE: ") + d.styles.MetricValue.Render(fmt.Sprintf("%d/%d", d.currentPage, d.totalPages))
	meta := sizeInfo + sep + pageInfo

	// Get table content
	content := ""
	if d.table != nil {
		content = d.table.View()
	}

	box := jobsbox.New(
		jobsbox.WithStyles(jobsbox.Styles{
			Title:  d.styles.Title,
			Border: d.styles.BorderStyle,
		}),
		jobsbox.WithTitle("Dead Jobs"),
		jobsbox.WithMeta(meta),
		jobsbox.WithContent(content),
		jobsbox.WithSize(d.width, d.height),
	)
	return box.View()
}

