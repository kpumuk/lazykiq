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

// BusyUpdateMsg carries busy data from App to Busy view
type BusyUpdateMsg struct {
	Data sidekiq.BusyData
}

// Busy shows active workers/processes
type Busy struct {
	width  int
	height int
	styles Styles
	data   sidekiq.BusyData
	table  *table.Table
	ready  bool
}

// NewBusy creates a new Busy view
func NewBusy() *Busy {
	return &Busy{}
}

// Init implements View
func (b *Busy) Init() tea.Cmd {
	return nil
}

// Update implements View
func (b *Busy) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case BusyUpdateMsg:
		b.data = msg.Data
		b.ready = true
		b.updateTableRows()
		return b, nil

	case tea.KeyMsg:
		if b.table != nil {
			b.table.Update(msg)
		}
		return b, nil
	}

	return b, nil
}

// View implements View
func (b *Busy) View() string {
	if !b.ready {
		return b.renderMessage("Loading...")
	}

	if len(b.data.Processes) == 0 && len(b.data.Jobs) == 0 {
		return b.renderMessage("No active processes")
	}

	var output strings.Builder

	// 1. Process list at top (outside the border)
	if len(b.data.Processes) > 0 {
		processList := b.renderProcessList()
		output.WriteString(processList)
		output.WriteString("\n")
	}

	// 2. Bordered "Active Jobs" box with table inside
	boxContent := b.renderJobsBox()
	output.WriteString(boxContent)

	return output.String()
}

// Name implements View
func (b *Busy) Name() string {
	return "Busy"
}

// ShortHelp implements View
func (b *Busy) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (b *Busy) SetSize(width, height int) View {
	b.width = width
	b.height = height
	b.updateTableSize()
	return b
}

// SetStyles implements View
func (b *Busy) SetStyles(styles Styles) View {
	b.styles = styles
	if b.table != nil {
		b.table.SetStyles(table.Styles{
			Text:      styles.Text,
			Muted:     styles.Muted,
			Header:    styles.TableHeader,
			Selected:  styles.TableSelected,
			Separator: styles.TableSeparator,
		})
	}
	return b
}

// renderProcessList renders the compact process list (outside the border)
func (b *Busy) renderProcessList() string {
	if len(b.data.Processes) == 0 {
		return ""
	}

	var lines []string
	for i, proc := range b.data.Processes {
		number := fmt.Sprintf("%d", i+1)
		busy := fmt.Sprintf("[%d/%d]", proc.Busy, proc.Concurrency)
		location := fmt.Sprintf("%s:%s", proc.Hostname, proc.PID)
		queues := strings.Join(proc.Queues, ", ")

		line := fmt.Sprintf("%s %s %s  %s", number, busy, location, queues)
		lines = append(lines, b.styles.Text.Render(line))
	}

	return b.styles.BoxPadding.Render(strings.Join(lines, "\n"))
}

// Table columns for job list
var jobColumns = []table.Column{
	{Title: "Process", Width: 18},
	{Title: "TID", Width: 6},
	{Title: "JID", Width: 24},
	{Title: "Queue", Width: 12},
	{Title: "Age", Width: 6},
	{Title: "Class", Width: 30},
	{Title: "Args", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size
func (b *Busy) updateTableSize() {
	if b.table == nil {
		return
	}
	// Calculate table height: total height - process list - box borders
	processListHeight := len(b.data.Processes) + 1
	tableHeight := b.height - processListHeight - 2
	if tableHeight < 3 {
		tableHeight = 3
	}
	// Table width: view width - box borders - padding
	tableWidth := b.width - 4
	b.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows
func (b *Busy) updateTableRows() {
	b.ensureTable()

	rows := make([][]string, 0, len(b.data.Jobs))
	for _, job := range b.data.Jobs {
		processID := job.ProcessIdentity
		parts := strings.Split(processID, ":")
		if len(parts) >= 2 {
			processID = parts[0] + ":" + parts[1]
		}

		row := []string{
			processID,
			job.ThreadID,
			job.JID,
			job.Queue,
			format.Duration(time.Now().Unix() - job.RunAt),
			job.Class,
			format.Args(job.Args),
		}
		rows = append(rows, row)
	}
	b.table.SetRows(rows)
	b.updateTableSize()
}

// ensureTable creates the table if it doesn't exist
func (b *Busy) ensureTable() {
	if b.table != nil {
		return
	}
	b.table = table.New(jobColumns)
	b.table.SetEmptyMessage("No active jobs")
	b.table.SetStyles(table.Styles{
		Text:      b.styles.Text,
		Muted:     b.styles.Muted,
		Header:    b.styles.TableHeader,
		Selected:  b.styles.TableSelected,
		Separator: b.styles.TableSeparator,
	})
}

// renderJobsBox renders the bordered box containing the jobs table
func (b *Busy) renderJobsBox() string {
	// Build dynamic title with stats
	processCount := len(b.data.Processes)
	totalThreads := 0
	busyThreads := 0
	totalRSS := int64(0)

	for _, proc := range b.data.Processes {
		totalThreads += proc.Concurrency
		busyThreads += proc.Busy
		totalRSS += proc.RSS
	}

	percentage := 0
	if totalThreads > 0 {
		percentage = (busyThreads * 100) / totalThreads
	}

	rssStr := format.Bytes(totalRSS)

	// Build styled title parts: "Active Jobs" on left, stats on right
	titleLeft := " " + b.styles.Title.Render("Active Jobs") + " "
	sep := b.styles.Muted.Render(" • ")
	titleRight := " " + b.styles.MetricLabel.Render("PRC: ") + b.styles.MetricValue.Render(fmt.Sprintf("%d", processCount)) +
		sep + b.styles.MetricLabel.Render("THR: ") + b.styles.MetricValue.Render(fmt.Sprintf("%d/%d (%d%%)", busyThreads, totalThreads, percentage)) +
		sep + b.styles.MetricLabel.Render("RSS: ") + b.styles.MetricValue.Render(rssStr) + " "

	// Calculate box dimensions
	processListHeight := len(b.data.Processes) + 1
	boxHeight := b.height - processListHeight
	if boxHeight < 5 {
		boxHeight = 5
	}
	boxWidth := b.width

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

	hBar := b.styles.BorderStyle.Render(string(border.Top))
	topBorder := b.styles.BorderStyle.Render(string(border.TopLeft)) +
		hBar +
		titleLeft +
		strings.Repeat(hBar, middlePad) +
		titleRight +
		hBar +
		b.styles.BorderStyle.Render(string(border.TopRight))

	// Side borders
	vBar := b.styles.BorderStyle.Render(string(border.Left))
	vBarRight := b.styles.BorderStyle.Render(string(border.Right))

	// Get table content
	tableContent := ""
	if b.table != nil {
		tableContent = b.table.View()
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
	bottomBorder := b.styles.BorderStyle.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		b.styles.BorderStyle.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}

func (b *Busy) renderMessage(msg string) string {
	// Header: "No processes" placeholder
	header := b.styles.BoxPadding.Render(b.styles.Muted.Render("No processes"))
	headerHeight := 2 // placeholder line + newline

	// Bordered box with centered message
	boxHeight := b.height - headerHeight
	box := messagebox.Render(messagebox.Styles{
		Title:  b.styles.Title,
		Muted:  b.styles.Muted,
		Border: b.styles.BorderStyle,
	}, "Active Jobs", msg, b.width, boxHeight)

	return header + "\n" + box
}
