package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// Queues shows the list of Sidekiq queues.
type Queues struct {
	client        *sidekiq.Client
	width         int
	height        int
	styles        Styles
	queues        []*QueueInfo
	jobs          []*sidekiq.PositionedEntry
	table         table.Model
	ready         bool
	currentPage   int
	totalPages    int
	selectedQueue int
}

// NewQueues creates a new Queues view.
func NewQueues(client *sidekiq.Client) *Queues {
	return &Queues{
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
func (q *Queues) Init() tea.Cmd {
	q.reset()
	return q.fetchDataCmd()
}

// Update implements View.
func (q *Queues) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case queuesDataMsg:
		q.queues = msg.queues
		q.jobs = msg.jobs
		q.currentPage = msg.currentPage
		q.totalPages = msg.totalPages
		q.selectedQueue = msg.selectedQueue
		q.ready = true
		q.updateTableRows()
		return q, nil

	case RefreshMsg:
		return q, q.fetchDataCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5", "ctrl+6", "ctrl+7", "ctrl+8", "ctrl+9":
			idx := int(msg.String()[5] - '1')
			if idx >= 0 && idx < len(q.queues) && q.selectedQueue != idx {
				q.selectedQueue = idx
				q.currentPage = 1
				return q, q.fetchDataCmd()
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
func (q *Queues) View() string {
	if !q.ready {
		return q.renderMessage("Loading...")
	}

	if len(q.queues) == 0 {
		return q.renderMessage("No queues")
	}

	return lipgloss.JoinVertical(lipgloss.Left, q.renderQueueList(), q.renderJobsBox())
}

// Name implements View.
func (q *Queues) Name() string {
	return "Queues"
}

// ShortHelp implements View.
func (q *Queues) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View.
func (q *Queues) SetSize(width, height int) View {
	q.width = width
	q.height = height
	q.updateTableSize()
	return q
}

// Dispose clears cached data when the view is removed from the stack.
func (q *Queues) Dispose() {
	q.reset()
	q.updateTableSize()
}

// SetStyles implements View.
func (q *Queues) SetStyles(styles Styles) View {
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

// fetchDataCmd fetches queues data from Redis.
func (q *Queues) fetchDataCmd() tea.Cmd {
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

		if selectedQueue >= len(queues) {
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

func (q *Queues) reset() {
	q.ready = false
	q.currentPage = 1
	q.totalPages = 1
	q.selectedQueue = 0
	q.queues = nil
	q.jobs = nil
	q.table.SetRows(nil)
	q.table.SetCursor(0)
}

// renderQueueList renders the compact queue list (outside the border).
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

	lines := make([]string, 0, len(q.queues))
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
func (q *Queues) updateTableSize() {
	// Calculate table height: total height - queue list - box borders
	queueListHeight := len(q.queues)
	tableHeight := max(q.height-queueListHeight-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := q.width - 4
	q.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows.
func (q *Queues) updateTableRows() {
	rows := make([]table.Row, 0, len(q.jobs))
	for _, job := range q.jobs {
		row := table.Row{
			fmt.Sprintf("%d", job.Position),
			job.DisplayClass(),
			format.Args(job.DisplayArgs()),
			formatContext(job.Context()),
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
func (q *Queues) renderJobsBox() string {
	// Build dynamic title with queue name
	queueName := ""
	queueSize := int64(0)
	if q.selectedQueue < len(q.queues) {
		queueName = q.queues[q.selectedQueue].Name
		queueSize = q.queues[q.selectedQueue].Size
	}
	title := fmt.Sprintf("Jobs in %s", queueName)

	// Build meta: SIZE and PAGE info
	sep := q.styles.Muted.Render(" • ")
	sizeInfo := q.styles.MetricLabel.Render("SIZE: ") + q.styles.MetricValue.Render(format.Number(queueSize))
	pageInfo := q.styles.MetricLabel.Render("PAGE: ") + q.styles.MetricValue.Render(fmt.Sprintf("%d/%d", q.currentPage, q.totalPages))
	meta := sizeInfo + sep + pageInfo

	// Calculate box height (account for queue list above)
	queueListHeight := len(q.queues)
	boxHeight := q.height - queueListHeight

	// Get table content
	content := q.table.View()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  q.styles.Title,
				Border: q.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  q.styles.Title,
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

func (q *Queues) renderMessage(msg string) string {
	// Header: "No queues" placeholder
	header := q.styles.BoxPadding.Render(q.styles.Muted.Render("No queues"))
	headerHeight := 2 // placeholder line + newline

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
