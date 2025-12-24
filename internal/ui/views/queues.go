package views

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// QueueInfo holds pre-fetched queue information for display
type QueueInfo struct {
	Name    string
	Size    int64
	Latency float64
}

// QueuesUpdateMsg carries queues data from App to Queues view
type QueuesUpdateMsg struct {
	Queues        []*QueueInfo
	Jobs          []*sidekiq.JobRecord
	CurrentPage   int
	TotalPages    int
	SelectedQueue int
}

// QueuesPageRequestMsg requests a specific page of jobs
type QueuesPageRequestMsg struct {
	Page int
}

// QueuesQueueSelectMsg requests selection of a specific queue by index (0-indexed)
type QueuesQueueSelectMsg struct {
	Index int
}

// Queues shows the list of Sidekiq queues
type Queues struct {
	width         int
	height        int
	styles        Styles
	queues        []*QueueInfo
	jobs          []*sidekiq.JobRecord
	table         *table.Table
	ready         bool
	currentPage   int
	totalPages    int
	selectedQueue int
}

// NewQueues creates a new Queues view
func NewQueues() *Queues {
	return &Queues{}
}

// Init implements View
func (q *Queues) Init() tea.Cmd {
	return nil
}

// Update implements View
func (q *Queues) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case QueuesUpdateMsg:
		q.queues = msg.Queues
		q.jobs = msg.Jobs
		q.currentPage = msg.CurrentPage
		q.totalPages = msg.TotalPages
		q.selectedQueue = msg.SelectedQueue
		q.ready = true
		q.updateTableRows()
		return q, nil

	case tea.KeyMsg:
		// Handle queue selection with alt+1-9
		switch msg.String() {
		case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8", "alt+9":
			// Extract the digit from the key
			idx := int(msg.String()[4] - '1') // "alt+1" -> 0, "alt+2" -> 1, etc.
			if idx >= 0 && idx < len(q.queues) {
				return q, func() tea.Msg {
					return QueuesQueueSelectMsg{Index: idx}
				}
			}
			return q, nil
		case "alt+left", "[":
			if q.currentPage > 1 {
				return q, func() tea.Msg {
					return QueuesPageRequestMsg{Page: q.currentPage - 1}
				}
			}
			return q, nil
		case "alt+right", "]":
			if q.currentPage < q.totalPages {
				return q, func() tea.Msg {
					return QueuesPageRequestMsg{Page: q.currentPage + 1}
				}
			}
			return q, nil
		}

		// Pass other keys to table for navigation
		if q.table != nil {
			q.table.Update(msg)
		}
		return q, nil
	}

	return q, nil
}

// View implements View
func (q *Queues) View() string {
	if !q.ready {
		return q.renderMessage("Loading...")
	}

	if len(q.queues) == 0 {
		return q.renderMessage("No queues")
	}

	var output strings.Builder

	// 1. Queue list at top (outside the border)
	queueList := q.renderQueueList()
	output.WriteString(queueList)
	output.WriteString("\n")

	// 2. Bordered "Jobs" box with table inside
	boxContent := q.renderJobsBox()
	output.WriteString(boxContent)

	return output.String()
}

// Name implements View
func (q *Queues) Name() string {
	return "Queues"
}

// ShortHelp implements View
func (q *Queues) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (q *Queues) SetSize(width, height int) View {
	q.width = width
	q.height = height
	q.updateTableSize()
	return q
}

// SetStyles implements View
func (q *Queues) SetStyles(styles Styles) View {
	q.styles = styles
	if q.table != nil {
		q.table.SetStyles(table.Styles{
			Text:      styles.Text,
			Muted:     styles.Muted,
			Header:    styles.TableHeader,
			Selected:  styles.TableSelected,
			Separator: styles.TableSeparator,
		})
	}
	return q
}

// renderQueueList renders the compact queue list (outside the border)
func (q *Queues) renderQueueList() string {
	if len(q.queues) == 0 {
		return ""
	}

	// First pass: find max widths for alignment
	maxNameLen := 0
	maxSizeLen := 0
	maxLatencyLen := 0
	for _, queue := range q.queues {
		if len(queue.Name) > maxNameLen {
			maxNameLen = len(queue.Name)
		}
		sizeStr := fmt.Sprintf("%d", queue.Size)
		if len(sizeStr) > maxSizeLen {
			maxSizeLen = len(sizeStr)
		}
		latencyStr := formatLatency(queue.Latency)
		if len(latencyStr) > maxLatencyLen {
			maxLatencyLen = len(latencyStr)
		}
	}

	var lines []string
	for i, queue := range q.queues {
		// Hotkey with grey background (like navbar), bold if selected
		hotkeyText := fmt.Sprintf("%d", i+1)
		var hotkey string
		if i == q.selectedQueue {
			hotkey = q.styles.NavKey.Bold(true).Render(hotkeyText)
		} else {
			hotkey = q.styles.NavKey.Render(hotkeyText)
		}

		// Queue name (left-aligned)
		name := q.styles.Text.Render(fmt.Sprintf("%-*s", maxNameLen, queue.Name))

		// Size and latency (right-aligned)
		sizeStr := fmt.Sprintf("%*d", maxSizeLen, queue.Size)
		latencyStr := fmt.Sprintf("%*s", maxLatencyLen, formatLatency(queue.Latency))
		stats := q.styles.Muted.Render(fmt.Sprintf("  %s  %s", sizeStr, latencyStr))

		lines = append(lines, hotkey+name+stats)
	}

	return q.styles.BoxPadding.Render(strings.Join(lines, "\n"))
}

// formatLatency formats latency in seconds as a readable string
func formatLatency(seconds float64) string {
	if seconds < 1 {
		return fmt.Sprintf("%.0fms", seconds*1000)
	}
	return format.Duration(int64(seconds))
}

// Table columns for queue job list
var queueJobColumns = []table.Column{
	{Title: "#", Width: 6},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 60},
	{Title: "Context", Width: 40},
}

// updateTableSize updates the table dimensions based on current view size
func (q *Queues) updateTableSize() {
	if q.table == nil {
		return
	}
	// Calculate table height: total height - queue list - box borders
	queueListHeight := len(q.queues) + 1
	tableHeight := q.height - queueListHeight - 2
	if tableHeight < 3 {
		tableHeight = 3
	}
	// Table width: view width - box borders - padding
	tableWidth := q.width - 4
	q.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows
func (q *Queues) updateTableRows() {
	q.ensureTable()

	rows := make([][]string, 0, len(q.jobs))
	for _, job := range q.jobs {
		row := []string{
			fmt.Sprintf("%d", job.Position),
			job.DisplayClass(),
			format.Args(job.Args()),
			formatContext(job.Context()),
		}
		rows = append(rows, row)
	}
	q.table.SetRows(rows)
	q.updateTableSize()
}

// formatContext formats the context map as a string
func formatContext(ctx map[string]interface{}) string {
	if len(ctx) == 0 {
		return ""
	}
	b, err := json.Marshal(ctx)
	if err != nil {
		return ""
	}
	return string(b)
}

// ensureTable creates the table if it doesn't exist
func (q *Queues) ensureTable() {
	if q.table != nil {
		return
	}
	q.table = table.New(queueJobColumns)
	q.table.SetEmptyMessage("No jobs in queue")
	q.table.SetStyles(table.Styles{
		Text:      q.styles.Text,
		Muted:     q.styles.Muted,
		Header:    q.styles.TableHeader,
		Selected:  q.styles.TableSelected,
		Separator: q.styles.TableSeparator,
	})
}

// renderJobsBox renders the bordered box containing the jobs table
func (q *Queues) renderJobsBox() string {
	// Build dynamic title with queue name and size
	queueName := ""
	queueSize := int64(0)
	if q.selectedQueue < len(q.queues) {
		queueName = q.queues[q.selectedQueue].Name
		queueSize = q.queues[q.selectedQueue].Size
	}

	// Build styled title parts: "Jobs in <queue>" on left, stats on right
	titleLeft := " " + q.styles.Title.Render(fmt.Sprintf("Jobs in %s", queueName)) + " "

	// Build right side: SIZE and PAGE info
	sep := q.styles.Muted.Render(" • ")
	sizeInfo := q.styles.MetricLabel.Render("SIZE: ") + q.styles.MetricValue.Render(format.Number(queueSize))
	pageInfo := q.styles.MetricLabel.Render("PAGE: ") + q.styles.MetricValue.Render(fmt.Sprintf("%d/%d", q.currentPage, q.totalPages))
	titleRight := " " + sizeInfo + sep + pageInfo + " "

	// Calculate box dimensions
	queueListHeight := len(q.queues) + 1
	boxHeight := q.height - queueListHeight
	if boxHeight < 5 {
		boxHeight = 5
	}
	boxWidth := q.width

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

	hBar := q.styles.BorderStyle.Render(string(border.Top))
	topBorder := q.styles.BorderStyle.Render(string(border.TopLeft)) +
		hBar +
		titleLeft +
		strings.Repeat(hBar, middlePad) +
		titleRight +
		hBar +
		q.styles.BorderStyle.Render(string(border.TopRight))

	// Side borders
	vBar := q.styles.BorderStyle.Render(string(border.Left))
	vBarRight := q.styles.BorderStyle.Render(string(border.Right))

	// Get table content
	tableContent := ""
	if q.table != nil {
		tableContent = q.table.View()
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
	bottomBorder := q.styles.BorderStyle.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		q.styles.BorderStyle.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}

func (q *Queues) renderMessage(msg string) string {
	// Header: "No queues" placeholder
	header := q.styles.BoxPadding.Render(q.styles.Muted.Render("No queues"))
	headerHeight := 2 // placeholder line + newline

	// Bordered box with centered message
	boxHeight := q.height - headerHeight
	box := messagebox.Render(messagebox.Styles{
		Title:  q.styles.Title,
		Muted:  q.styles.Muted,
		Border: q.styles.BorderStyle,
	}, "Jobs", msg, q.width, boxHeight)

	return header + "\n" + box
}
