package views

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
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

type queueDetailsPayload struct {
	queues        []*QueueInfo
	jobs          []*sidekiq.PositionedEntry
	selectedQueue int
}

const (
	queuesWindowPages      = 3
	queuesFallbackPageSize = 25
)

// QueueDetails shows the jobs in a specific Sidekiq queue.
type QueueDetails struct {
	client           sidekiq.API
	width            int
	height           int
	styles           Styles
	queues           []*QueueInfo
	jobs             []*sidekiq.PositionedEntry
	lazy             lazytable.Model
	ready            bool
	selectedQueue    int
	selectedQueueKey string // Queue name to select after loading
	displayOrder     []int  // Maps ctrl+1-5 to queue indices
}

// NewQueueDetails creates a new QueueDetails view.
func NewQueueDetails(client sidekiq.API) *QueueDetails {
	q := &QueueDetails{
		client:        client,
		selectedQueue: 0,
		lazy: lazytable.New(
			lazytable.WithTableOptions(
				table.WithColumns(queueJobColumns),
				table.WithEmptyMessage("No jobs in queue"),
			),
			lazytable.WithWindowPages(queuesWindowPages),
			lazytable.WithFallbackPageSize(queuesFallbackPageSize),
		),
	}
	q.lazy.SetFetcher(q.fetchWindow)
	q.lazy.SetErrorHandler(func(err error) tea.Msg {
		return ConnectionErrorMsg{Err: err}
	})
	return q
}

// Init implements View.
func (q *QueueDetails) Init() tea.Cmd {
	q.reset()
	return q.lazy.RequestWindow(0, lazytable.CursorStart)
}

// Update implements View.
func (q *QueueDetails) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if msg.RequestID != q.lazy.RequestID() {
			return q, nil
		}
		if payload, ok := msg.Result.Payload.(queueDetailsPayload); ok {
			q.queues = payload.queues
			q.jobs = payload.jobs
			q.selectedQueue = payload.selectedQueue
		}
		// Clear the queue key after successfully loading
		q.selectedQueueKey = ""
		q.ready = true
		var cmd tea.Cmd
		q.lazy, cmd = q.lazy.Update(msg)
		return q, cmd

	case RefreshMsg:
		if q.lazy.Loading() {
			return q, nil
		}
		if q.selectedQueueKey != "" {
			return q, q.lazy.RequestWindow(0, lazytable.CursorStart)
		}
		return q, q.lazy.RequestWindow(q.lazy.WindowStart(), lazytable.CursorKeep)

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			// Switch to queues list view
			return q, func() tea.Msg {
				return ShowQueuesListMsg{}
			}
		case "c":
			if idx := q.lazy.Table().Cursor(); idx >= 0 && idx < len(q.jobs) {
				if q.jobs[idx] != nil {
					return q, copyTextCmd(q.jobs[idx].JID())
				}
			}
			return q, nil
		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5":
			displayIdx := int(msg.String()[5] - '1')
			if displayIdx >= 0 && displayIdx < len(q.displayOrder) {
				queueIdx := q.displayOrder[displayIdx]
				if queueIdx >= 0 && queueIdx < len(q.queues) && q.selectedQueue != queueIdx {
					q.selectedQueue = queueIdx
					q.selectedQueueKey = "" // Clear queue key when manually switching
					q.lazy.Table().SetCursor(0)
					return q, q.lazy.RequestWindow(0, lazytable.CursorStart)
				}
			}
			return q, nil
		case "alt+left", "[":
			q.lazy.MovePage(-1)
			return q, q.lazy.MaybePrefetch()
		case "alt+right", "]":
			q.lazy.MovePage(1)
			return q, q.lazy.MaybePrefetch()
		case "enter":
			// Show detail for selected job
			if idx := q.lazy.Table().Cursor(); idx >= 0 && idx < len(q.jobs) {
				return q, func() tea.Msg {
					return ShowJobDetailMsg{Job: q.jobs[idx].JobRecord}
				}
			}
			return q, nil
		}

		var cmd tea.Cmd
		q.lazy, cmd = q.lazy.Update(msg)
		return q, cmd
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
	if start, end, total := q.lazy.Range(); total > 0 && len(q.jobs) > 0 {
		items = append(items, ContextItem{
			Label: "Rows",
			Value: fmt.Sprintf(
				"%s-%s/%s",
				format.Number(int64(start)),
				format.Number(int64(end)),
				format.Number(total),
			),
		})
	}
	return items
}

// HintBindings implements HintProvider.
func (q *QueueDetails) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"s"}, "s", "switch queue"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
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
			helpBinding([]string{"["}, "[", "page up"),
			helpBinding([]string{"]"}, "]", "page down"),
			helpBinding([]string{"c"}, "c", "copy jid"),
			helpBinding([]string{"enter"}, "enter", "job detail"),
		},
	}}
	return sections
}

// TableHelp implements TableHelpProvider.
func (q *QueueDetails) TableHelp() []key.Binding {
	return tableHelpBindings(q.lazy.Table().KeyMap)
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
	q.lazy.SetSpinnerStyle(styles.Muted)
	q.lazy.SetTableStyles(table.Styles{
		Text:           styles.Text,
		Muted:          styles.Muted,
		Header:         styles.TableHeader,
		Selected:       styles.TableSelected,
		Separator:      styles.TableSeparator,
		ScrollbarTrack: styles.ScrollbarTrack,
		ScrollbarThumb: styles.ScrollbarThumb,
	})
	return q
}

// SetQueue allows setting the selected queue by name.
func (q *QueueDetails) SetQueue(queueName string) {
	q.selectedQueueKey = queueName
	q.lazy.Table().SetCursor(0)
	// Try to find and select immediately if queues are already loaded
	for i, queue := range q.queues {
		if queue.Name == queueName {
			q.selectedQueue = i
			return
		}
	}
}

func (q *QueueDetails) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	ctx = devtools.WithTracker(ctx, "queue_details.fetchWindow")

	queues, err := q.client.GetQueues(ctx)
	if err != nil {
		return lazytable.FetchResult{}, err
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

	selectedQueue := q.resolveSelectedQueue(queues, q.selectedQueue)
	jobs, totalSize, windowStart := q.fetchQueueJobs(ctx, queues, selectedQueue, windowStart, windowSize)

	return lazytable.FetchResult{
		Rows:        q.buildRows(jobs),
		Total:       totalSize,
		WindowStart: windowStart,
		Payload: queueDetailsPayload{
			queues:        queueInfos,
			jobs:          jobs,
			selectedQueue: selectedQueue,
		},
	}, nil
}

func (q *QueueDetails) resolveSelectedQueue(queues []*sidekiq.Queue, selected int) int {
	if len(queues) == 0 {
		return 0
	}

	if q.selectedQueueKey != "" {
		if idx := slices.IndexFunc(queues, func(queue *sidekiq.Queue) bool {
			return queue.Name() == q.selectedQueueKey
		}); idx >= 0 {
			return idx
		}
		return 0
	}

	if selected < 0 || selected >= len(queues) {
		return 0
	}
	return selected
}

func (q *QueueDetails) fetchQueueJobs(
	ctx context.Context,
	queues []*sidekiq.Queue,
	selectedQueue int,
	windowStart int,
	windowSize int,
) ([]*sidekiq.PositionedEntry, int64, int) {
	if len(queues) == 0 || selectedQueue < 0 || selectedQueue >= len(queues) {
		return nil, 0, 0
	}

	if windowSize <= 0 {
		windowSize = max(queuesFallbackPageSize, 1) * queuesWindowPages
	}

	queue := queues[selectedQueue]
	jobs, totalSize, _ := queue.GetJobs(ctx, windowStart, windowSize)

	if totalSize > 0 {
		maxStart := max(int(totalSize)-windowSize, 0)
		if windowStart > maxStart {
			windowStart = maxStart
			jobs, totalSize, _ = queue.GetJobs(ctx, windowStart, windowSize)
		}
	} else {
		windowStart = 0
	}

	return jobs, totalSize, windowStart
}

func (q *QueueDetails) reset() {
	q.ready = false
	q.lazy.Reset()
	// Only reset selectedQueue if no queue key is set
	if q.selectedQueueKey == "" {
		q.selectedQueue = 0
	}
	q.queues = nil
	q.jobs = nil
	q.displayOrder = nil
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
	q.lazy.SetSize(tableWidth, tableHeight)
}

func (q *QueueDetails) buildRows(jobs []*sidekiq.PositionedEntry) []table.Row {
	rows := make([]table.Row, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, table.Row{
			ID: job.JID(),
			Cells: []string{
				strconv.Itoa(job.Position),
				job.DisplayClass(),
				format.Args(job.DisplayArgs()),
				formatContext(job.Context()),
			},
		})
	}
	return rows
}

func (q *QueueDetails) rowsMeta() string {
	start, end, total := q.lazy.Range()
	totalLabel := format.Number(total)
	if total == 0 || len(q.jobs) == 0 {
		return q.styles.MetricLabel.Render("rows: ") + q.styles.MetricValue.Render("0/0")
	}

	rangeLabel := fmt.Sprintf(
		"%s-%s/%s",
		format.Number(int64(start)),
		format.Number(int64(end)),
		totalLabel,
	)
	return q.styles.MetricLabel.Render("rows: ") + q.styles.MetricValue.Render(rangeLabel)
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
	meta := sizeInfo + sep + q.rowsMeta()

	// Calculate box height
	boxHeight := q.height

	// Get table content
	content := q.lazy.View()

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
