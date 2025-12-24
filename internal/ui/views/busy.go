package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/jobsbox"
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
	width           int
	height          int
	styles          Styles
	data            sidekiq.BusyData
	table           *table.Table
	ready           bool
	selectedProcess int // -1 = all, 0-8 = specific process index
}

// NewBusy creates a new Busy view
func NewBusy() *Busy {
	return &Busy{
		selectedProcess: -1, // Show all jobs by default
	}
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
		// Handle process selection with alt+0-9
		switch msg.String() {
		case "alt+0":
			// Show all jobs
			if b.selectedProcess != -1 {
				b.selectedProcess = -1
				b.updateTableRows()
			}
			return b, nil
		case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8", "alt+9":
			// Extract the digit from the key
			idx := int(msg.String()[4] - '1') // "alt+1" -> 0, "alt+2" -> 1, etc.
			if idx >= 0 && idx < len(b.data.Processes) && b.selectedProcess != idx {
				b.selectedProcess = idx
				b.updateTableRows()
			}
			return b, nil
		}

		// Pass other keys to table for navigation
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

// renderProcessList renders the process list as a table (outside the border)
func (b *Busy) renderProcessList() string {
	if len(b.data.Processes) == 0 {
		return ""
	}

	// First pass: find max widths for alignment
	maxNameLen := 0
	maxBusyLen := 0
	maxStartedLen := 0
	maxRSSLen := 0

	type processRow struct {
		name    string
		busy    string
		started string
		rss     string
		queues  string
	}
	rows := make([]processRow, len(b.data.Processes))

	for i, proc := range b.data.Processes {
		// Name: hostname:pid + tag
		name := fmt.Sprintf("%s:%s", proc.Hostname, proc.PID)
		if proc.Tag != "" {
			name += " [" + proc.Tag + "]"
		}

		// Busy/Threads: busy/concurrency format
		busy := fmt.Sprintf("%d/%d", proc.Busy, proc.Concurrency)

		// Started: relative time
		started := format.Duration(time.Now().Unix() - proc.StartedAt)

		// RSS: memory usage
		rss := format.Bytes(proc.RSS)

		// Queues
		queues := strings.Join(proc.Queues, ", ")

		rows[i] = processRow{name, busy, started, rss, queues}

		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
		if len(busy) > maxBusyLen {
			maxBusyLen = len(busy)
		}
		if len(started) > maxStartedLen {
			maxStartedLen = len(started)
		}
		if len(rss) > maxRSSLen {
			maxRSSLen = len(rss)
		}
	}

	var lines []string
	for i, row := range rows {
		// Hotkey with NavKey style, bold if selected
		hotkeyText := fmt.Sprintf("%d", i+1)
		var hotkey string
		if i == b.selectedProcess {
			hotkey = b.styles.NavKey.Bold(true).Render(hotkeyText)
		} else {
			hotkey = b.styles.NavKey.Render(hotkeyText)
		}

		// Name (left-aligned)
		name := b.styles.Text.Render(fmt.Sprintf("%-*s", maxNameLen, row.name))

		// Stats (right-aligned, muted)
		busy := fmt.Sprintf("%*s", maxBusyLen, row.busy)
		started := fmt.Sprintf("%*s", maxStartedLen, row.started)
		rss := fmt.Sprintf("%*s", maxRSSLen, row.rss)
		stats := b.styles.Muted.Render(fmt.Sprintf("  %s  %s  %s", busy, started, rss))

		// Queues (muted)
		queues := b.styles.Muted.Render("  " + row.queues)

		lines = append(lines, hotkey+name+stats+queues)
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

	// Get the selected process identity for filtering
	var selectedIdentity string
	if b.selectedProcess >= 0 && b.selectedProcess < len(b.data.Processes) {
		selectedIdentity = b.data.Processes[b.selectedProcess].Identity
	}

	rows := make([][]string, 0, len(b.data.Jobs))
	for _, job := range b.data.Jobs {
		// Filter by selected process if one is selected
		if selectedIdentity != "" && job.ProcessIdentity != selectedIdentity {
			continue
		}

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
	// Calculate stats for meta
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

	// Build meta: PRC, THR, RSS info
	sep := b.styles.Muted.Render(" • ")
	meta := b.styles.MetricLabel.Render("PRC: ") + b.styles.MetricValue.Render(fmt.Sprintf("%d", processCount)) +
		sep + b.styles.MetricLabel.Render("THR: ") + b.styles.MetricValue.Render(fmt.Sprintf("%d/%d (%d%%)", busyThreads, totalThreads, percentage)) +
		sep + b.styles.MetricLabel.Render("RSS: ") + b.styles.MetricValue.Render(format.Bytes(totalRSS))

	// Calculate box height (account for process list above)
	processListHeight := len(b.data.Processes) + 1
	boxHeight := b.height - processListHeight

	// Build title based on selected process
	title := "Active Jobs"
	if b.selectedProcess >= 0 && b.selectedProcess < len(b.data.Processes) {
		proc := b.data.Processes[b.selectedProcess]
		title = fmt.Sprintf("Active Jobs on %s:%s", proc.Hostname, proc.PID)
	}

	// Get table content
	content := ""
	if b.table != nil {
		content = b.table.View()
	}

	box := jobsbox.New(
		jobsbox.WithStyles(jobsbox.Styles{
			Title:  b.styles.Title,
			Border: b.styles.BorderStyle,
		}),
		jobsbox.WithTitle(title),
		jobsbox.WithMeta(meta),
		jobsbox.WithContent(content),
		jobsbox.WithSize(b.width, boxHeight),
	)
	return box.View()
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
