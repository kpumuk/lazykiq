package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// RetriesUpdateMsg carries retry jobs data from App to Retries view
type RetriesUpdateMsg struct {
	Jobs        []*sidekiq.SortedEntry
	CurrentPage int
	TotalPages  int
	TotalSize   int64
}

// RetriesPageRequestMsg requests a specific page of retry jobs
type RetriesPageRequestMsg struct {
	Page int
}

// Retries shows failed jobs pending retry
type Retries struct {
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

// NewRetries creates a new Retries view
func NewRetries() *Retries {
	return &Retries{}
}

// Init implements View
func (r *Retries) Init() tea.Cmd {
	return nil
}

// Update implements View
func (r *Retries) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case RetriesUpdateMsg:
		r.jobs = msg.Jobs
		r.currentPage = msg.CurrentPage
		r.totalPages = msg.TotalPages
		r.totalSize = msg.TotalSize
		r.ready = true
		r.updateTableRows()
		return r, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "alt+left", "[":
			if r.currentPage > 1 {
				return r, func() tea.Msg {
					return RetriesPageRequestMsg{Page: r.currentPage - 1}
				}
			}
			return r, nil
		case "alt+right", "]":
			if r.currentPage < r.totalPages {
				return r, func() tea.Msg {
					return RetriesPageRequestMsg{Page: r.currentPage + 1}
				}
			}
			return r, nil
		}

		// Pass other keys to table for navigation
		if r.table != nil {
			r.table.Update(msg)
		}
		return r, nil
	}

	return r, nil
}

// View implements View
func (r *Retries) View() string {
	if !r.ready {
		return r.renderMessage("Loading...")
	}

	if len(r.jobs) == 0 && r.totalSize == 0 {
		return r.renderMessage("No retries")
	}

	return r.renderJobsBox()
}

func (r *Retries) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  r.styles.Title,
		Muted:  r.styles.Muted,
		Border: r.styles.BorderStyle,
	}, "Retries", msg, r.width, r.height)
}

// Name implements View
func (r *Retries) Name() string {
	return "Retries"
}

// ShortHelp implements View
func (r *Retries) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (r *Retries) SetSize(width, height int) View {
	r.width = width
	r.height = height
	r.updateTableSize()
	return r
}

// SetStyles implements View
func (r *Retries) SetStyles(styles Styles) View {
	r.styles = styles
	if r.table != nil {
		r.table.SetStyles(table.Styles{
			Text:      styles.Text,
			Muted:     styles.Muted,
			Header:    styles.TableHeader,
			Selected:  styles.TableSelected,
			Separator: styles.TableSeparator,
		})
	}
	return r
}

// Table columns for retry job list
var retryJobColumns = []table.Column{
	{Title: "Next Retry", Width: 12},
	{Title: "Retries", Width: 7},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size
func (r *Retries) updateTableSize() {
	if r.table == nil {
		return
	}
	// Calculate table height: total height - box borders
	tableHeight := r.height - 2
	if tableHeight < 3 {
		tableHeight = 3
	}
	// Table width: view width - box borders - padding
	tableWidth := r.width - 4
	r.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows
func (r *Retries) updateTableRows() {
	r.ensureTable()

	rows := make([][]string, 0, len(r.jobs))
	now := time.Now().Unix()
	for _, job := range r.jobs {
		// Format "next retry" as relative time (negative means in the past/due)
		nextRetry := format.Duration(now - job.At())

		// Format retry count
		retryCount := fmt.Sprintf("%d", job.RetryCount())

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
			nextRetry,
			retryCount,
			job.Queue(),
			job.DisplayClass(),
			format.Args(job.Args()),
			errorStr,
		}
		rows = append(rows, row)
	}
	r.table.SetRows(rows)
	r.updateTableSize()
}

// ensureTable creates the table if it doesn't exist
func (r *Retries) ensureTable() {
	if r.table != nil {
		return
	}
	r.table = table.New(retryJobColumns)
	r.table.SetEmptyMessage("No retries")
	r.table.SetStyles(table.Styles{
		Text:      r.styles.Text,
		Muted:     r.styles.Muted,
		Header:    r.styles.TableHeader,
		Selected:  r.styles.TableSelected,
		Separator: r.styles.TableSeparator,
	})
}

// renderJobsBox renders the bordered box containing the jobs table
func (r *Retries) renderJobsBox() string {
	// Build styled title parts: "Retries" on left, stats on right
	titleLeft := " " + r.styles.Title.Render("Retries") + " "

	// Build right side: SIZE and PAGE info
	sep := r.styles.Muted.Render(" • ")
	sizeInfo := r.styles.MetricLabel.Render("SIZE: ") + r.styles.MetricValue.Render(format.Number(r.totalSize))
	pageInfo := r.styles.MetricLabel.Render("PAGE: ") + r.styles.MetricValue.Render(fmt.Sprintf("%d/%d", r.currentPage, r.totalPages))
	titleRight := " " + sizeInfo + sep + pageInfo + " "

	// Calculate box dimensions
	boxHeight := r.height
	if boxHeight < 5 {
		boxHeight = 5
	}
	boxWidth := r.width

	// Build the border manually
	border := lipgloss.RoundedBorder()

	// Top border with title on left, stats on right
	leftWidth := lipgloss.Width(titleLeft)
	rightWidth := lipgloss.Width(titleRight)
	innerWidth := boxWidth - 2
	middlePad := innerWidth - leftWidth - rightWidth - 2
	if middlePad < 0 {
		middlePad = 0
	}

	hBar := r.styles.BorderStyle.Render(string(border.Top))
	topBorder := r.styles.BorderStyle.Render(string(border.TopLeft)) +
		hBar +
		titleLeft +
		strings.Repeat(hBar, middlePad) +
		titleRight +
		hBar +
		r.styles.BorderStyle.Render(string(border.TopRight))

	// Side borders
	vBar := r.styles.BorderStyle.Render(string(border.Left))
	vBarRight := r.styles.BorderStyle.Render(string(border.Right))

	// Get table content
	tableContent := ""
	if r.table != nil {
		tableContent = r.table.View()
	}
	lines := strings.Split(tableContent, "\n")

	var middleLines []string
	contentHeight := boxHeight - 2 // minus top and bottom borders

	for i := 0; i < contentHeight; i++ {
		var line string
		if i < len(lines) {
			line = lines[i]
		}

		// Add padding
		line = " " + line + " "
		lineWidth := lipgloss.Width(line)
		padding := innerWidth - lineWidth
		if padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		middleLines = append(middleLines, vBar+line+vBarRight)
	}

	// Bottom border
	bottomBorder := r.styles.BorderStyle.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		r.styles.BorderStyle.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}

