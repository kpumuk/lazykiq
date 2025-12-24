package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
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
		return d.renderMessageBox("Loading...")
	}

	if len(d.jobs) == 0 && d.totalSize == 0 {
		return d.renderMessageBox("No dead jobs")
	}

	return d.renderJobsBox()
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
	// Build styled title parts: "Dead Jobs" on left, stats on right
	titleLeft := " " + d.styles.Title.Render("Dead Jobs") + " "

	// Build right side: SIZE and PAGE info
	sep := d.styles.Muted.Render(" • ")
	sizeInfo := d.styles.MetricLabel.Render("SIZE: ") + d.styles.MetricValue.Render(format.Number(d.totalSize))
	pageInfo := d.styles.MetricLabel.Render("PAGE: ") + d.styles.MetricValue.Render(fmt.Sprintf("%d/%d", d.currentPage, d.totalPages))
	titleRight := " " + sizeInfo + sep + pageInfo + " "

	// Calculate box dimensions
	boxHeight := d.height
	if boxHeight < 5 {
		boxHeight = 5
	}
	boxWidth := d.width

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

	hBar := d.styles.BorderStyle.Render(string(border.Top))
	topBorder := d.styles.BorderStyle.Render(string(border.TopLeft)) +
		hBar +
		titleLeft +
		strings.Repeat(hBar, middlePad) +
		titleRight +
		hBar +
		d.styles.BorderStyle.Render(string(border.TopRight))

	// Side borders
	vBar := d.styles.BorderStyle.Render(string(border.Left))
	vBarRight := d.styles.BorderStyle.Render(string(border.Right))

	// Get table content
	tableContent := ""
	if d.table != nil {
		tableContent = d.table.View()
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
	bottomBorder := d.styles.BorderStyle.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		d.styles.BorderStyle.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}

// renderMessageBox renders a bordered box with centered message
func (d *Dead) renderMessageBox(message string) string {
	title := "Dead Jobs"
	boxHeight := d.height
	if boxHeight < 5 {
		boxHeight = 5
	}
	boxWidth := d.width

	// Build the border
	border := lipgloss.RoundedBorder()

	// Top border with title
	titleText := " " + title + " "
	styledTitle := d.styles.Title.Render(titleText)
	titleWidth := lipgloss.Width(styledTitle)
	innerWidth := boxWidth - 2
	leftPad := 1
	rightPad := innerWidth - titleWidth - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	hBar := d.styles.BorderStyle.Render(string(border.Top))
	topBorder := d.styles.BorderStyle.Render(string(border.TopLeft)) +
		strings.Repeat(hBar, leftPad) +
		styledTitle +
		strings.Repeat(hBar, rightPad) +
		d.styles.BorderStyle.Render(string(border.TopRight))

	// Content with side borders - centered message
	vBar := d.styles.BorderStyle.Render(string(border.Left))
	vBarRight := d.styles.BorderStyle.Render(string(border.Right))

	contentHeight := boxHeight - 2 // minus top and bottom borders
	var middleLines []string

	// Center the message vertically
	msgText := d.styles.Muted.Render(message)
	msgWidth := lipgloss.Width(msgText)
	centerRow := contentHeight / 2

	for i := 0; i < contentHeight; i++ {
		var line string
		if i == centerRow {
			// Center horizontally
			leftPadding := (innerWidth - msgWidth) / 2
			if leftPadding < 0 {
				leftPadding = 0
			}
			rightPadding := innerWidth - leftPadding - msgWidth
			if rightPadding < 0 {
				rightPadding = 0
			}
			line = strings.Repeat(" ", leftPadding) + msgText + strings.Repeat(" ", rightPadding)
		} else {
			line = strings.Repeat(" ", innerWidth)
		}
		middleLines = append(middleLines, vBar+line+vBarRight)
	}

	// Bottom border
	bottomBorder := d.styles.BorderStyle.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		d.styles.BorderStyle.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}
