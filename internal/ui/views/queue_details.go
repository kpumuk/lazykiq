package views

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

// QueueInfo holds pre-fetched queue information for display.
type QueueInfo struct {
	Name    string
	Size    int64
	Latency float64
	Paused  bool
}

type queueDetailsPayload struct {
	queues           []*QueueInfo
	jobs             []*sidekiq.PositionedEntry
	selectedQueue    int
	capabilities     sidekiq.Capabilities
	metadataRevision uint64
}

const (
	queuesWindowPages      = 3
	queuesFallbackPageSize = 25
)

// QueueDetails shows the jobs in a specific Sidekiq queue.
type QueueDetails struct {
	client sidekiq.API
	detailListView
	queues                  []*QueueInfo
	jobs                    []*sidekiq.PositionedEntry
	selectedQueue           int
	selectedQueueKey        string // Queue name to select after loading
	displayOrder            []int  // Maps ctrl+1-5 to queue indices
	capabilities            sidekiq.Capabilities
	dangerousActionsEnabled bool
	actions                 queueActions
	metadataRevision        uint64
	metadataLoadedRevision  uint64
}

// NewQueueDetails creates a new QueueDetails view.
func NewQueueDetails(client sidekiq.API) *QueueDetails {
	q := &QueueDetails{
		client: client,
		detailListView: newDetailListView(
			"Jobs",
			queueJobColumns,
			"No jobs in queue",
			queuesWindowPages,
			queuesFallbackPageSize,
		),
		selectedQueue: 0,
	}
	q.lazy.SetFetcherFactory(func() lazytable.Fetcher {
		snapshot := *q
		return snapshot.fetchWindow
	})
	return q
}

// Init implements View.
func (q *QueueDetails) Init() tea.Cmd {
	return q.init(q.reset)
}

// Update implements View.
func (q *QueueDetails) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		q.lazy, cmd = q.lazy.Update(msg)
		return q, cmd

	case lazytable.DataMsg:
		if q.actions.busy {
			return q, nil
		}
		if handled, cmd := q.handleData(msg, func(result lazytable.FetchResult) {
			if payload, ok := result.Payload.(queueDetailsPayload); ok {
				q.queues = payload.queues
				q.jobs = payload.jobs
				q.selectedQueue = payload.selectedQueue
				q.capabilities = payload.capabilities
				q.metadataLoadedRevision = payload.metadataRevision
			}
			q.selectedQueueKey = ""
			q.updateEmptyMessage()
		}); handled {
			return q, cmd
		}
		return q, nil

	case RefreshMsg:
		if q.actions.busy {
			return q, nil
		}
		q.metadataRevision++
		return q, q.refreshWindow()
	case QueueActionResultMsg:
		if !q.actions.complete(msg) {
			return q, nil
		}
		if msg.Err != nil {
			return q, nil
		}
		q.metadataRevision++
		return q, q.refreshWindow()
	case confirmdialog.ActionMsg:
		if !q.dangerousActionsEnabled {
			return q, nil
		}
		cmd := q.actions.confirm(q.Context(), msg, q.client)
		if cmd != nil {
			q.lazy.CancelRequest()
		}
		return q, cmd

	case filterdialog.ActionMsg:
		return q, q.handleFilterAction(msg, q.updateEmptyMessage)

	case tea.KeyPressMsg:
		if q.actions.busy {
			return q, nil
		}
		if handled, cmd := q.handleKeyPress(msg, q.updateEmptyMessage); handled {
			return q, cmd
		}

		switch msg.String() {
		case "p":
			if q.dangerousActionsEnabled && q.capabilities.SupportsQueuePause() {
				if queue, ok := q.currentQueue(); ok {
					return q, q.actions.pauseDialog(queue.Name, queue.Paused, q.styles, q.IntentScope)
				}
			}
			return q, nil
		case "s":
			// Switch to queues list view
			return q, q.EmitIntent(ShowQueuesListMsg{})
		case "c":
			if job, ok := q.selectedJob(); ok {
				return q, copyTextCmd(job.JID())
			}
			return q, nil
		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5":
			displayIdx := int(msg.String()[5] - '1')
			if displayIdx >= 0 && displayIdx < len(q.displayOrder) {
				queueIdx := q.displayOrder[displayIdx]
				if queueIdx >= 0 && queueIdx < len(q.queues) && q.selectedQueue != queueIdx {
					return q, q.selectQueue(queueIdx)
				}
			}
			return q, nil
		case "enter":
			// Show detail for selected job
			if job, ok := q.selectedJob(); ok {
				return q, q.EmitIntent(ShowJobDetailMsg{Job: job.JobRecord})
			}
			return q, nil
		}

		return q, q.updateKeyPress(msg)
	}

	return q, nil
}

// View implements View.
func (q *QueueDetails) View() string {
	if !q.ready {
		return q.renderLoadingMessage()
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
	if q.filter != "" {
		items = append(items, ContextItem{Label: "Filter", Value: q.filter})
	}
	if start, end, total := q.lazy.Range(); total > 0 && len(q.jobs) > 0 {
		items = append(items, ContextItem{
			Label: "Rows",
			Value: fmt.Sprintf(
				"%s-%s/%s",
				display.Number(int64(start)),
				display.Number(int64(end)),
				display.Number(total),
			),
		})
	}
	return items
}

// HintBindings implements HintProvider.
func (q *QueueDetails) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"s"}, "s", "switch queue"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// HelpSections implements HelpProvider.
func (q *QueueDetails) HelpSections() []HelpSection {
	sections := []HelpSection{{
		Title: "Queue Jobs",
		Bindings: []key.Binding{
			helpBinding([]string{"/"}, "/", "filter"),
			helpBinding([]string{"ctrl+u"}, "ctrl+u", "clear filter"),
			helpBinding([]string{"s"}, "s", "switch queue"),
			helpBinding([]string{"ctrl+1"}, "ctrl+1-5", "select queue"),
			helpBinding([]string{"["}, "[", "page up"),
			helpBinding([]string{"]"}, "]", "page down"),
			helpBinding([]string{"g"}, "g", "jump to start"),
			helpBinding([]string{"G"}, "shift+g", "jump to end"),
			helpBinding([]string{"c"}, "c", "copy jid"),
			helpBinding([]string{"enter"}, "enter", "job detail"),
		},
	}}
	if bindings := q.MutationBindings(); len(bindings) > 0 {
		sections = append(sections, HelpSection{Title: "Dangerous Actions", Bindings: bindings})
	}
	return sections
}

// MutationBindings implements MutationHintProvider.
func (q *QueueDetails) MutationBindings() []key.Binding {
	if q.dangerousActionsEnabled && q.capabilities.SupportsQueuePause() {
		if queue, ok := q.currentQueue(); ok {
			return []key.Binding{helpBinding([]string{"p"}, "p", queuePauseHelp(queue.Paused))}
		}
	}
	return nil
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (q *QueueDetails) SetDangerousActionsEnabled(enabled bool) {
	q.dangerousActionsEnabled = enabled
}

// TableHelp implements TableHelpProvider.
func (q *QueueDetails) TableHelp() []key.Binding {
	return q.tableHelp()
}

// SetSize implements View.
func (q *QueueDetails) SetSize(width, height int) View {
	q.setSize(width, height)
	return q
}

// Dispose clears cached data when the view is removed from the stack.
func (q *QueueDetails) Dispose() {
	q.dispose(q.reset)
}

// CancelRequests stops in-flight fetches when the view is hidden.
func (q *QueueDetails) CancelRequests() {
	q.cancelRequests()
	q.actions.reset()
	q.metadataRevision++
}

// SetStyles implements View.
func (q *QueueDetails) SetStyles(styles Styles) View {
	q.setStyles(styles)
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
	queueInfos, capabilities, err := q.fetchMetadata(ctx)
	if err != nil {
		return lazytable.FetchResult{}, err
	}

	selectedQueue := q.resolveSelectedQueue(queueInfos)
	var queue *sidekiq.Queue
	if len(queueInfos) > 0 {
		queue = q.client.NewQueue(queueInfos[selectedQueue].Name)
	}
	jobs, totalSize, windowStart, err := q.fetchQueueJobs(ctx, queue, windowStart, windowSize)
	if err != nil {
		return lazytable.FetchResult{}, err
	}

	return lazytable.FetchResult{
		Rows:        q.buildRows(jobs),
		Total:       totalSize,
		WindowStart: windowStart,
		Payload: queueDetailsPayload{
			queues:           queueInfos,
			jobs:             jobs,
			selectedQueue:    selectedQueue,
			capabilities:     capabilities,
			metadataRevision: q.metadataRevision,
		},
	}, nil
}

func (q *QueueDetails) fetchMetadata(ctx context.Context) ([]*QueueInfo, sidekiq.Capabilities, error) {
	// Paging and filtering reuse the last accepted immutable metadata snapshot.
	// RefreshMsg (the app's five-second cadence), mutations and hiding invalidate it.
	if q.queues != nil && q.metadataLoadedRevision == q.metadataRevision {
		return q.queues, q.capabilities, nil
	}

	capabilities, paused, err := fetchQueueCapabilities(ctx, q.client)
	if err != nil {
		return nil, capabilities, err
	}
	queues, err := q.client.GetQueues(ctx)
	if err != nil {
		return nil, capabilities, err
	}
	metadata, err := sidekiq.FetchQueueMetadata(ctx, queues)
	if err != nil {
		return nil, capabilities, err
	}
	infos := make([]*QueueInfo, len(metadata))
	for i, queue := range metadata {
		infos[i] = &QueueInfo{
			Name:    queue.Name,
			Size:    queue.Size,
			Latency: queue.Latency,
			Paused:  paused[queue.Name],
		}
	}
	return infos, capabilities, nil
}

func (q *QueueDetails) refreshWindow() tea.Cmd {
	if q.lazy.Loading() {
		return nil
	}
	if q.selectedQueueKey != "" {
		return q.reloadFromStart()
	}
	return q.detailListView.refreshWindow()
}

func (q *QueueDetails) resolveSelectedQueue(queues []*QueueInfo) int {
	if len(queues) == 0 {
		return 0
	}

	selected, key := q.selectedQueue, q.selectedQueueKey
	if key == "" && selected >= 0 && selected < len(q.queues) {
		key = q.queues[selected].Name
	}
	if key != "" {
		if idx := slices.IndexFunc(queues, func(queue *QueueInfo) bool {
			return queue.Name == key
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
	queue *sidekiq.Queue,
	windowStart int,
	windowSize int,
) ([]*sidekiq.PositionedEntry, int64, int, error) {
	if queue == nil {
		return nil, 0, 0, nil
	}

	if windowSize <= 0 {
		windowSize = max(queuesFallbackPageSize, 1) * queuesWindowPages
	}

	if q.filter != "" {
		return q.fetchFilteredQueueJobs(ctx, queue, windowStart, windowSize)
	}

	return q.fetchUnfilteredQueueJobs(ctx, queue, windowStart, windowSize)
}

func (q *QueueDetails) fetchFilteredQueueJobs(
	ctx context.Context,
	queue *sidekiq.Queue,
	windowStart int,
	windowSize int,
) ([]*sidekiq.PositionedEntry, int64, int, error) {
	window, err := queue.ScanJobsWindow(ctx, q.filter, windowStart, windowSize)
	if err != nil {
		return nil, 0, 0, err
	}

	totalSize := window.Total
	if totalSize <= 0 {
		return nil, 0, 0, nil
	}

	maxStart := max(int(totalSize)-windowSize, 0)
	if windowStart > maxStart {
		windowStart = maxStart
		window, err = queue.ScanJobsWindow(ctx, q.filter, windowStart, windowSize)
		if err != nil {
			return nil, 0, 0, err
		}
	}

	return window.Entries, totalSize, windowStart, nil
}

func (q *QueueDetails) fetchUnfilteredQueueJobs(
	ctx context.Context,
	queue *sidekiq.Queue,
	windowStart int,
	windowSize int,
) ([]*sidekiq.PositionedEntry, int64, int, error) {
	jobs, totalSize, err := queue.GetJobs(ctx, windowStart, windowSize)
	if err != nil {
		return nil, 0, 0, err
	}

	if totalSize > 0 {
		maxStart := max(int(totalSize)-windowSize, 0)
		if windowStart > maxStart {
			windowStart = maxStart
			jobs, totalSize, err = queue.GetJobs(ctx, windowStart, windowSize)
			if err != nil {
				return nil, 0, 0, err
			}
		}
	} else {
		windowStart = 0
	}

	return jobs, totalSize, windowStart, nil
}

func (q *QueueDetails) reset() {
	q.actions.reset()
	q.resetShell()
	// Only reset selectedQueue if no queue key is set
	if q.selectedQueueKey == "" {
		q.selectedQueue = 0
	}
	q.queues = nil
	q.jobs = nil
	q.displayOrder = nil
	q.updateEmptyMessage()
}

func (q *QueueDetails) selectQueue(queueIdx int) tea.Cmd {
	q.selectedQueue = queueIdx
	q.selectedQueueKey = ""
	return q.reloadFromStart()
}

func (q *QueueDetails) selectedJob() (*sidekiq.PositionedEntry, bool) {
	idx := q.lazy.Table().Cursor()
	if idx < 0 || idx >= len(q.jobs) {
		return nil, false
	}
	if q.jobs[idx] == nil {
		return nil, false
	}
	return q.jobs[idx], true
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
		if queue.Paused {
			stats += "  " + q.styles.QueuePaused.Render("Paused")
		}

		lines = append(lines, hotkey+" "+name+stats)
	}

	return lines
}

// formatLatency formats latency in seconds as a readable string.
func formatLatency(seconds float64) string {
	if seconds < 1 {
		return fmt.Sprintf("%.0fms", seconds*1000)
	}
	return display.Duration(int64(seconds))
}

// Table columns for queue job list.
var queueJobColumns = []table.Column{
	{Title: "#", Width: 6},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 60},
	{Title: "Context", Width: 40},
}

func (q *QueueDetails) buildRows(jobs []*sidekiq.PositionedEntry) []table.Row {
	rows := make([]table.Row, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, table.Row{
			ID: job.JID(),
			Cells: []string{
				strconv.Itoa(job.Position),
				job.DisplayClass(),
				display.ArgsPreview(job.DisplayArgs()),
				formatContext(job.Context()),
			},
		})
	}
	return rows
}

func (q *QueueDetails) updateEmptyMessage() {
	msg := "No jobs in queue"
	if len(q.queues) == 0 {
		msg = "No queues"
	} else if q.filter != "" {
		msg = "No matches"
	}
	q.lazy.SetEmptyMessage(msg)
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
	title := "Jobs"
	if q.selectedQueue >= 0 && q.selectedQueue < len(q.queues) {
		title = "Jobs in " + q.queues[q.selectedQueue].Name
		if q.queues[q.selectedQueue].Paused {
			title += " · Paused"
		}
	}
	return q.renderBox(title, len(q.jobs))
}

func (q *QueueDetails) currentQueue() (*QueueInfo, bool) {
	if q.selectedQueue >= 0 && q.selectedQueue < len(q.queues) {
		return q.queues[q.selectedQueue], true
	}
	return nil, false
}

// renderJobDetail renders the job detail view.
