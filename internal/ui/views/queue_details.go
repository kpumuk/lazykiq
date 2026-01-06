package views

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// QueueInfo holds pre-fetched queue information for display.
type QueueInfo struct {
	Name    string
	Size    int64
	Latency float64
}

// queuesDataMsg carries queues data internally.
type queuesDataMsg struct {
	queues        []*QueueInfo
	jobs          []*sidekiq.PositionedEntry
	currentPage   int
	totalPages    int
	selectedQueue int
}

const queuesPageSize = 25

// QueueDetails shows the jobs in a specific Sidekiq queue.
type QueueDetails struct {
	client           sidekiq.API
	width            int
	height           int
	styles           Styles
	queues           []*QueueInfo
	jobs             []*sidekiq.PositionedEntry
	table            table.Model
	ready            bool
	currentPage      int
	totalPages       int
	selectedQueue    int
	selectedQueueKey string // Queue name to select after loading
	displayOrder     []int  // Maps ctrl+1-5 to queue indices
}

// NewQueueDetails creates a new QueueDetails view.
func NewQueueDetails(client sidekiq.API) *QueueDetails {
	return &QueueDetails{
		client:        client,
		currentPage:   1,
		totalPages:    1,
		selectedQueue: 0,
		table: table.New(
			table.WithColumns(queueJobColumns),
			table.WithEmptyMessage("No jobs in queue"),
		),
	}
}

// Init implements View.
func (q *QueueDetails) Init() tea.Cmd {
	q.reset()
	return q.fetchDataCmd()
}

// Update implements View.
func (q *QueueDetails) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case queuesDataMsg:
		q.queues = msg.queues
		q.jobs = msg.jobs
		q.currentPage = msg.currentPage
		q.totalPages = msg.totalPages
		q.selectedQueue = msg.selectedQueue
		// Clear the queue key after successfully loading
		q.selectedQueueKey = ""
		q.ready = true
		q.updateTableRows()
		return q, nil

	case RefreshMsg:
		return q, q.fetchDataCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			// Switch to queues list view
			return q, func() tea.Msg {
				return ShowQueuesListMsg{}
			}
		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5":
			displayIdx := int(msg.String()[5] - '1')
			if displayIdx >= 0 && displayIdx < len(q.displayOrder) {
				queueIdx := q.displayOrder[displayIdx]
				if queueIdx >= 0 && queueIdx < len(q.queues) && q.selectedQueue != queueIdx {
					q.selectedQueue = queueIdx
					q.selectedQueueKey = "" // Clear queue key when manually switching
					q.currentPage = 1
					return q, q.fetchDataCmd()
				}
			}
			return q, nil
		case "alt+left", "[":
			if q.currentPage > 1 {
				q.currentPage--
				return q, q.fetchDataCmd()
			}
			return q, nil
		case "alt+right", "]":
			if q.currentPage < q.totalPages {
				q.currentPage++
				return q, q.fetchDataCmd()
			}
			return q, nil
		case "enter":
			// Show detail for selected job
			if idx := q.table.Cursor(); idx >= 0 && idx < len(q.jobs) {
				return q, func() tea.Msg {
					return ShowJobDetailMsg{Job: q.jobs[idx].JobRecord}
				}
			}
			return q, nil
		}

		q.table, _ = q.table.Update(msg)
		return q, nil
	}

	return q, nil
}

// View implements View.
func (q *QueueDetails) View() string {
	if !q.ready {
		return q.renderMessage("Loading...")
	}

	if len(q.queues) == 0 {
		return q.renderMessage("No queues")
	}

	return q.renderJobsBox()
}

// Name implements View.
func (q *QueueDetails) Name() string {
	return "Queues"
}

// ShortHelp implements View.
func (q *QueueDetails) ShortHelp() []key.Binding {
	return nil
}

// HeaderLines implements HeaderLinesProvider.
func (q *QueueDetails) HeaderLines() []string {
	lines := q.queueListLines()
	if len(lines) > 5 {
		lines = lines[:5]
	}
	if len(lines) < 5 {
		padding := make([]string, 5-len(lines))
		lines = append(lines, padding...)
	}
	if len(lines) == 0 {
		return make([]string, 5)
	}
	return lines
}

// ContextItems implements ContextProvider.
func (q *QueueDetails) ContextItems() []ContextItem {
	queueName := ""
	if q.selectedQueue >= 0 && q.selectedQueue < len(q.queues) {
		queueName = q.queues[q.selectedQueue].Name
	}
	items := []ContextItem{}
	if queueName != "" {
		items = append(items, ContextItem{Label: "Queue", Value: q.styles.QueueText.Render(queueName)})
	}
	if q.totalPages > 0 {
		items = append(items, ContextItem{Label: "Page", Value: fmt.Sprintf("%d/%d", q.currentPage, q.totalPages)})
	}
	return items
}

// HintBindings implements HintProvider.
func (q *QueueDetails) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"s"}, "s", "switch queue"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "change page"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// HelpSections implements HelpProvider.
func (q *QueueDetails) HelpSections() []HelpSection {
	sections := []HelpSection{{
		Title: "Queue Actions",
		Bindings: []key.Binding{
			helpBinding([]string{"s"}, "s", "switch queue"),
			helpBinding([]string{"ctrl+1"}, "ctrl+1-5", "select queue"),
			helpBinding([]string{"["}, "[", "previous page"),
			helpBinding([]string{"]"}, "]", "next page"),
			helpBinding([]string{"enter"}, "enter", "job detail"),
		},
	}}
	return sections
}

// TableHelp implements TableHelpProvider.
func (q *QueueDetails) TableHelp() []key.Binding {
	return tableHelpBindings(q.table.KeyMap)
}

// SetSize implements View.
func (q *QueueDetails) SetSize(width, height int) View {
	q.width = width
	q.height = height
	q.updateTableSize()
	return q
}

// Dispose clears cached data when the view is removed from the stack.
func (q *QueueDetails) Dispose() {
	q.reset()
	q.updateTableSize()
}

// SetStyles implements View.
func (q *QueueDetails) SetStyles(styles Styles) View {
	q.styles = styles
	q.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	return q
}

// SetQueue allows setting the selected queue by name.
func (q *QueueDetails) SetQueue(queueName string) {
	q.selectedQueueKey = queueName
	q.currentPage = 1
	// Try to find and select immediately if queues are already loaded
	for i, queue := range q.queues {
		if queue.Name == queueName {
			q.selectedQueue = i
			return
		}
	}
}

// fetchDataCmd fetches queues data from Redis.
func (q *QueueDetails) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		queues, err := q.client.GetQueues(ctx)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		queueInfos := make([]*QueueInfo, len(queues))
		for i, queue := range queues {
			size, _ := queue.Size(ctx)
			latency, _ := queue.Latency(ctx)
			queueInfos[i] = &QueueInfo{
				Name:    queue.Name(),
				Size:    size,
				Latency: latency,
			}
		}

		var jobs []*sidekiq.PositionedEntry
		var totalSize int64
		currentPage := q.currentPage
		totalPages := 1
		selectedQueue := q.selectedQueue

		// If a queue key is set, find it by name
		if q.selectedQueueKey != "" && len(queues) > 0 {
			found := false
			for i, queue := range queues {
				if queue.Name() == q.selectedQueueKey {
					selectedQueue = i
					found = true
					break
				}
			}
			// If queue was not found, default to first queue
			if !found {
				selectedQueue = 0
			}
		} else if selectedQueue >= len(queues) {
			selectedQueue = 0
		}

		if len(queues) > 0 && selectedQueue < len(queues) {
			start := (currentPage - 1) * queuesPageSize
			jobs, totalSize, _ = queues[selectedQueue].GetJobs(ctx, start, queuesPageSize)

			if totalSize > 0 {
				totalPages = int((totalSize + queuesPageSize - 1) / queuesPageSize)
			}

			if currentPage > totalPages {
				currentPage = totalPages
			}
			if currentPage < 1 {
				currentPage = 1
			}
		}

		return queuesDataMsg{
			queues:        queueInfos,
			jobs:          jobs,
			currentPage:   currentPage,
			totalPages:    totalPages,
			selectedQueue: selectedQueue,
		}
	}
}

func (q *QueueDetails) reset() {
	q.ready = false
	q.currentPage = 1
	q.totalPages = 1
	// Only reset selectedQueue if no queue key is set
	if q.selectedQueueKey == "" {
		q.selectedQueue = 0
	}
	q.queues = nil
	q.jobs = nil
	q.displayOrder = nil
	q.table.SetRows(nil)
	q.table.SetCursor(0)
}

// renderQueueList renders the compact queue list (outside the border).
func (q *QueueDetails) queueListLines() []string {
	if len(q.queues) == 0 {
		return nil
	}

	// Create index mapping for sorting
	type indexedQueue struct {
		queue *QueueInfo
		index int
	}
	indexed := make([]indexedQueue, len(q.queues))
	for i, queue := range q.queues {
		indexed[i] = indexedQueue{queue: queue, index: i}
	}

	// Sort by size (desc) and name (asc)
	sort.Slice(indexed, func(i, j int) bool {
		if indexed[i].queue.Size != indexed[j].queue.Size {
			return indexed[i].queue.Size > indexed[j].queue.Size
		}
		return indexed[i].queue.Name < indexed[j].queue.Name
	})

	// Take top 5 and build display order mapping
	displayCount := min(5, len(indexed))
	q.displayOrder = make([]int, displayCount)
	displayQueues := make([]*QueueInfo, displayCount)
	for i := range displayCount {
		q.displayOrder[i] = indexed[i].index
		displayQueues[i] = indexed[i].queue
	}

	// First pass: find max widths for alignment
	maxNameLen := 0
	maxSizeLen := 0
	maxLatencyLen := 0
	for _, queue := range displayQueues {
		if len(queue.Name) > maxNameLen {
			maxNameLen = len(queue.Name)
		}
		sizeStr := strconv.FormatInt(queue.Size, 10)
		if len(sizeStr) > maxSizeLen {
			maxSizeLen = len(sizeStr)
		}
		latencyStr := formatLatency(queue.Latency)
		if len(latencyStr) > maxLatencyLen {
			maxLatencyLen = len(latencyStr)
		}
	}

	lines := make([]string, 0, len(displayQueues))
	nameStyle := q.styles.QueueText.Bold(true).Width(maxNameLen)
	for i, queue := range displayQueues {
		queueIdx := q.displayOrder[i]

		// Hotkey with grey background (like navbar), bold if selected
		hotkeyText := fmt.Sprintf("ctrl+%d", i+1)
		var hotkey string
		if queueIdx == q.selectedQueue {
			hotkey = q.styles.NavKey.Bold(true).Render(hotkeyText)
		} else {
			hotkey = q.styles.NavKey.Render(hotkeyText)
		}

		// Queue name (left-aligned)
		name := nameStyle.Render(queue.Name)

		// Size and latency (right-aligned)
		sizeStr := fmt.Sprintf("%*d", maxSizeLen, queue.Size)
		latencyStr := fmt.Sprintf("%*s", maxLatencyLen, formatLatency(queue.Latency))
		stats := q.styles.Muted.Render(fmt.Sprintf("  %s  %s", sizeStr, latencyStr))

		lines = append(lines, hotkey+name+stats)
	}

	return lines
}

// formatLatency formats latency in seconds as a readable string.
func formatLatency(seconds float64) string {
	if seconds < 1 {
		return fmt.Sprintf("%.0fms", seconds*1000)
	}
	return format.Duration(int64(seconds))
}

// Table columns for queue job list.
var queueJobColumns = []table.Column{
	{Title: "#", Width: 6},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 60},
	{Title: "Context", Width: 40},
}

// updateTableSize updates the table dimensions based on current view size.
func (q *QueueDetails) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(q.height-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := q.width - 4
	q.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows.
func (q *QueueDetails) updateTableRows() {
	rows := make([]table.Row, 0, len(q.jobs))
	for _, job := range q.jobs {
		row := table.Row{
			ID: job.JID(),
			Cells: []string{
				strconv.Itoa(job.Position),
				job.DisplayClass(),
				format.Args(job.DisplayArgs()),
				formatContext(job.Context()),
			},
		}
		rows = append(rows, row)
	}
	q.table.SetRows(rows)
	q.updateTableSize()
}

// formatContext formats the context map as a string.
func formatContext(ctx map[string]any) string {
	if len(ctx) == 0 {
		return ""
	}
	b, err := json.Marshal(ctx)
	if err != nil {
		return ""
	}
	return string(b)
}

// renderJobsBox renders the bordered box containing the jobs table.
func (q *QueueDetails) renderJobsBox() string {
	// Build dynamic title with queue name
	queueName := ""
	queueSize := int64(0)
	if q.selectedQueue < len(q.queues) {
		queueName = q.queues[q.selectedQueue].Name
		queueSize = q.queues[q.selectedQueue].Size
	}
	title := "Jobs in " + queueName

	// Build meta: SIZE and PAGE info
	sep := q.styles.Muted.Render(" • ")
	sizeInfo := q.styles.MetricLabel.Render("SIZE: ") + q.styles.MetricValue.Render(format.ShortNumber(queueSize))
	pageInfo := q.styles.MetricLabel.Render("PAGE: ") + q.styles.MetricValue.Render(fmt.Sprintf("%d/%d", q.currentPage, q.totalPages))
	meta := sizeInfo + sep + pageInfo

	// Calculate box height
	boxHeight := q.height

	// Get table content
	content := q.table.View()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  q.styles.Title,
				Muted:  q.styles.Muted,
				Filter: q.styles.FilterFocused,
				Border: q.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  q.styles.Title,
				Muted:  q.styles.Muted,
				Filter: q.styles.FilterBlurred,
				Border: q.styles.BorderStyle,
			},
		}),
		frame.WithTitle(title),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(q.width, boxHeight),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

func (q *QueueDetails) renderMessage(msg string) string {
	// Header: "No queues" placeholder
	header := q.styles.BoxPadding.Render(q.styles.Muted.Render("No queues"))
	headerHeight := lipgloss.Height(header)

	// Bordered box with centered message
	boxHeight := q.height - headerHeight
	box := messagebox.Render(messagebox.Styles{
		Title:  q.styles.Title,
		Muted:  q.styles.Muted,
		Border: q.styles.FocusBorder,
	}, "Jobs", msg, q.width, boxHeight)

	return header + "\n" + box
}

// renderJobDetail renders the job detail view.
