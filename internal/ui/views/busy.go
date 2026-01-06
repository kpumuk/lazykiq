package views

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// busyDataMsg carries busy data from the fetch command to the Busy view.
type busyDataMsg struct {
	data sidekiq.BusyData
}

// Busy shows active workers/processes.
type Busy struct {
	client          sidekiq.API
	width           int
	height          int
	styles          Styles
	data            sidekiq.BusyData
	filteredJobs    []sidekiq.Job // jobs filtered by selectedProcess
	rowJobIndex     []int         // table row -> filtered job index (-1 for process rows)
	table           table.Model
	ready           bool
	selectedProcess int // -1 = all, 0-8 = specific process index
	treeMode        bool
	filter          string
	filterStyle     filterdialog.Styles
}

const processGlyph = "⚙"

// NewBusy creates a new Busy view.
func NewBusy(client sidekiq.API) *Busy {
	return &Busy{
		client:          client,
		selectedProcess: -1, // Show all jobs by default
		treeMode:        false,
		table: table.New(
			table.WithColumns(jobColumnsFlat),
			table.WithEmptyMessage("No active jobs"),
		),
	}
}

// Init implements View.
func (b *Busy) Init() tea.Cmd {
	b.reset()
	return b.fetchDataCmd()
}

// Update implements View.
func (b *Busy) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case busyDataMsg:
		b.data = msg.data
		b.ready = true
		b.updateTableRows()
		return b, nil

	case RefreshMsg:
		return b, b.fetchDataCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return b, nil
		}
		if msg.Query == b.filter {
			return b, nil
		}
		b.filter = msg.Query
		b.table.SetCursor(0)
		return b, b.fetchDataCmd()

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "/":
			return b, b.openFilterDialog()
		case "ctrl+u":
			if b.filter != "" {
				b.filter = ""
				b.table.SetCursor(0)
				return b, b.fetchDataCmd()
			}
			return b, nil
		case "s":
			return b, func() tea.Msg {
				return ShowProcessesListMsg{}
			}
		}
		if b.handleProcessSelectKey(key) {
			return b, nil
		}
		switch key {
		case "enter":
			// Show detail for selected job
			if idx := b.table.Cursor(); idx >= 0 && idx < len(b.rowJobIndex) {
				jobIdx := b.rowJobIndex[idx]
				if jobIdx >= 0 && jobIdx < len(b.filteredJobs) {
					return b, func() tea.Msg {
						return ShowJobDetailMsg{Job: b.filteredJobs[jobIdx].JobRecord}
					}
				}
			}
			return b, nil
		case "t":
			b.treeMode = !b.treeMode
			b.updateTableRows()
			return b, nil
		}

		b.table, _ = b.table.Update(msg)
		return b, nil
	}

	return b, nil
}

// View implements View.
func (b *Busy) View() string {
	if !b.ready {
		return b.renderMessage("Loading...")
	}

	if len(b.data.Processes) == 0 && len(b.data.Jobs) == 0 {
		return b.renderMessage("No active processes")
	}

	boxContent := b.renderJobsBox()
	return boxContent
}

// Name implements View.
func (b *Busy) Name() string {
	return "Busy"
}

// ShortHelp implements View.
func (b *Busy) ShortHelp() []key.Binding {
	return nil
}

// HeaderLines implements HeaderLinesProvider.
func (b *Busy) HeaderLines() []string {
	lines := b.processListLines()
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

// HintBindings implements HintProvider.
func (b *Busy) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"s"}, "s", "select process"),
		helpBinding([]string{"ctrl+0"}, "ctrl+0", "all processes"),
		helpBinding([]string{"t"}, "t", "toggle tree"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// HelpSections implements HelpProvider.
func (b *Busy) HelpSections() []HelpSection {
	sections := []HelpSection{{
		Title: "Busy",
		Bindings: []key.Binding{
			helpBinding([]string{"/"}, "/", "filter"),
			helpBinding([]string{"ctrl+u"}, "ctrl+u", "clear filter"),
			helpBinding([]string{"s"}, "s", "select process"),
			helpBinding([]string{"t"}, "t", "toggle tree"),
			helpBinding([]string{"enter"}, "enter", "job detail"),
			helpBinding([]string{"ctrl+1"}, "ctrl+1-9", "select process"),
			helpBinding([]string{"ctrl+0"}, "ctrl+0", "all processes"),
		},
	}}
	return sections
}

// TableHelp implements TableHelpProvider.
func (b *Busy) TableHelp() []key.Binding {
	return tableHelpBindings(b.table.KeyMap)
}

// SetSize implements View.
func (b *Busy) SetSize(width, height int) View {
	b.width = width
	b.height = height
	b.updateTableSize()
	return b
}

// Dispose clears cached data when the view is removed from the stack.
func (b *Busy) Dispose() {
	b.reset()
	b.updateTableSize()
}

// SetStyles implements View.
func (b *Busy) SetStyles(styles Styles) View {
	b.styles = styles
	b.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	b.filterStyle = filterdialog.Styles{
		Title:       styles.Title,
		Border:      styles.FocusBorder,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	}
	return b
}

// SetProcessIdentity updates the selected process by identity.
func (b *Busy) SetProcessIdentity(identity string) {
	if identity == "" {
		if b.selectedProcess != -1 {
			b.selectedProcess = -1
			b.updateTableRows()
		}
		return
	}

	for i, proc := range b.data.Processes {
		if proc.Identity != identity {
			continue
		}
		if b.selectedProcess != i {
			b.selectedProcess = i
			b.updateTableRows()
		}
		return
	}
}

// fetchDataCmd fetches busy data from Redis.
func (b *Busy) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		data, err := b.client.GetBusyData(ctx, b.filter)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return busyDataMsg{data: data}
	}
}

func (b *Busy) reset() {
	b.ready = false
	b.data = sidekiq.BusyData{}
	b.filteredJobs = nil
	b.rowJobIndex = nil
	b.selectedProcess = -1
	b.filter = ""
	b.table.SetRows(nil)
	b.table.SetCursor(0)
}

func (b *Busy) normalizeSelectedProcess() {
	if b.selectedProcess < -1 || b.selectedProcess >= len(b.data.Processes) {
		b.selectedProcess = -1
	}
}

func (b *Busy) selectedIdentity() string {
	if b.selectedProcess >= 0 && b.selectedProcess < len(b.data.Processes) {
		return b.data.Processes[b.selectedProcess].Identity
	}
	return ""
}

func (b *Busy) handleProcessSelectKey(key string) bool {
	if !strings.HasPrefix(key, "ctrl+") {
		return false
	}

	digit := strings.TrimPrefix(key, "ctrl+")
	if digit == "0" {
		if b.selectedProcess != -1 {
			b.selectedProcess = -1
			b.updateTableRows()
		}
		return true
	}
	if len(digit) != 1 {
		return false
	}

	idx, err := strconv.Atoi(digit)
	if err != nil {
		return false
	}
	idx--

	if idx < 0 || idx >= len(b.data.Processes) {
		return true
	}
	if b.selectedProcess != idx {
		b.selectedProcess = idx
		b.updateTableRows()
	}

	return true
}

// Table columns for job list.
var jobColumnsTree = []table.Column{
	{Title: "Process", Width: 14},
	{Title: "JID", Width: 24},
	{Title: "Queue", Width: 12},
	{Title: "Age", Width: 6, Align: table.AlignRight},
	{Title: "Class", Width: 24},
	{Title: "Args", Width: 60},
}

var jobColumnsFlat = []table.Column{
	{Title: "Process", Width: 14},
	{Title: "TID", Width: 6},
	{Title: "JID", Width: 24},
	{Title: "Queue", Width: 12},
	{Title: "Age", Width: 6, Align: table.AlignRight},
	{Title: "Class", Width: 24},
	{Title: "Args", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (b *Busy) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(b.height-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := b.width - 4
	b.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows.
func (b *Busy) updateTableRows() {
	b.normalizeSelectedProcess()
	if b.filter != "" {
		b.table.SetEmptyMessage("No matches")
	} else {
		b.table.SetEmptyMessage("No active jobs")
	}
	if b.treeMode {
		b.updateTableRowsTree()
		return
	}

	b.updateTableRowsFlat()
}

func (b *Busy) updateTableRowsTree() {
	b.table.SetColumns(jobColumnsTree)

	selectedIdentity := b.selectedIdentity()
	jobsByProcess := b.jobsByProcess(selectedIdentity)
	maxBusyLen, maxStartedLen, maxRSSLen := b.processStatWidths(selectedIdentity)
	glyphWidth := lipgloss.Width(processGlyph)

	// Build tree rows: process header + job rows
	b.filteredJobs = make([]sidekiq.Job, 0, len(b.data.Jobs))
	rows := make([]table.Row, 0, len(b.data.Jobs)+len(b.data.Processes))
	rowJobIndex := make([]int, 0, len(b.data.Jobs)+len(b.data.Processes))
	fullRows := make(map[int]string, len(b.data.Processes))
	selectionSpans := make(map[int]table.SelectionSpan, len(b.data.Jobs)+len(b.data.Processes))
	for _, proc := range b.data.Processes {
		if selectedIdentity != "" && proc.Identity != selectedIdentity {
			continue
		}

		processLine := b.renderProcessRow(proc, maxBusyLen, maxStartedLen, maxRSSLen)
		rows = append(rows, table.Row{ID: proc.Identity, Cells: make([]string, len(jobColumnsTree))})
		selectionSpans[len(rows)-1] = table.SelectionSpan{Start: glyphWidth + 1, End: -1}
		fullRows[len(rows)-1] = processLine
		rowJobIndex = append(rowJobIndex, -1)

		jobs := jobsByProcess[proc.Identity]
		for j, job := range jobs {
			branch := "├─ "
			if j == len(jobs)-1 {
				branch = "└─ "
			}
			prefix := branch
			treeCell := b.styles.Muted.Render(prefix) + job.ThreadID

			b.filteredJobs = append(b.filteredJobs, job)
			jobIndex := len(b.filteredJobs) - 1

			rows = append(rows, table.Row{
				ID: job.JID(),
				Cells: []string{
					treeCell,
					job.JID(),
					b.styles.QueueText.Render(job.Queue()),
					format.DurationSince(job.RunAt),
					job.DisplayClass(),
					format.Args(job.DisplayArgs()),
				},
			})
			selectionSpans[len(rows)-1] = table.SelectionSpan{
				Start: lipgloss.Width(prefix),
				End:   -1,
			}
			rowJobIndex = append(rowJobIndex, jobIndex)
		}
	}
	b.rowJobIndex = rowJobIndex
	b.table.SetRowsWithMeta(rows, fullRows, selectionSpans)
	b.updateTableSize()
}

func (b *Busy) updateTableRowsFlat() {
	b.table.SetColumns(jobColumnsFlat)

	selectedIdentity := b.selectedIdentity()

	b.filteredJobs = make([]sidekiq.Job, 0, len(b.data.Jobs))
	rows := make([]table.Row, 0, len(b.data.Jobs))
	rowJobIndex := make([]int, 0, len(b.data.Jobs))
	selectionSpans := make(map[int]table.SelectionSpan, len(b.data.Jobs))
	for _, job := range b.data.Jobs {
		if selectedIdentity != "" && job.ProcessIdentity != selectedIdentity {
			continue
		}

		b.filteredJobs = append(b.filteredJobs, job)
		jobIndex := len(b.filteredJobs) - 1

		rows = append(rows, table.Row{
			ID: job.JID(),
			Cells: []string{
				shortProcessIdentity(job.ProcessIdentity),
				job.ThreadID,
				job.JID(),
				b.styles.QueueText.Render(job.Queue()),
				format.DurationSince(job.RunAt),
				job.DisplayClass(),
				format.Args(job.DisplayArgs()),
			},
		})
		rowJobIndex = append(rowJobIndex, jobIndex)
		selectionSpans[len(rows)-1] = table.SelectionSpan{Start: 0, End: -1}
	}

	b.rowJobIndex = rowJobIndex
	b.table.SetRowsWithMeta(rows, nil, selectionSpans)
	b.updateTableSize()
}

func (b *Busy) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(b.filterStyle),
				filterdialog.WithQuery(b.filter),
			),
		}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
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
	meta := b.styles.MetricLabel.Render("PRC: ") + b.styles.MetricValue.Render(strconv.Itoa(processCount)) +
		sep + b.styles.MetricLabel.Render("THR: ") + b.styles.MetricValue.Render(fmt.Sprintf("%d/%d (%d%%)", busyThreads, totalThreads, percentage)) +
		sep + b.styles.MetricLabel.Render("RSS: ") + b.styles.MetricValue.Render(format.Bytes(totalRSS))

	// Calculate box height
	boxHeight := b.height

	// Build title based on selected process
	title := "Active Jobs"
	if b.selectedProcess >= 0 && b.selectedProcess < len(b.data.Processes) {
		proc := b.data.Processes[b.selectedProcess]
		title = fmt.Sprintf("Active Jobs on %s:%s", proc.Hostname, formatPID(proc.PID))
	}

	// Get table content
	content := b.table.View()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  b.styles.Title,
				Muted:  b.styles.Muted,
				Filter: b.styles.FilterFocused,
				Border: b.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  b.styles.Title,
				Muted:  b.styles.Muted,
				Filter: b.styles.FilterBlurred,
				Border: b.styles.BorderStyle,
			},
		}),
		frame.WithTitle(title),
		frame.WithFilter(b.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(b.width, boxHeight),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

func (b *Busy) renderMessage(msg string) string {
	// Bordered box with centered message
	boxHeight := b.height
	box := messagebox.Render(messagebox.Styles{
		Title:  b.styles.Title,
		Muted:  b.styles.Muted,
		Border: b.styles.FocusBorder,
	}, "Active Jobs", msg, b.width, boxHeight)

	return box
}

func formatPID(pid int) string {
	if pid <= 0 {
		return "-"
	}
	return strconv.Itoa(pid)
}

func shortProcessIdentity(identity string) string {
	parts := strings.Split(identity, ":")
	if len(parts) >= 2 {
		return parts[0] + ":" + parts[1]
	}
	return identity
}

func processIdentity(proc sidekiq.Process) string {
	name := shortProcessIdentity(proc.Identity)
	if proc.Hostname != "" && proc.PID > 0 {
		name = fmt.Sprintf("%s:%s", proc.Hostname, formatPID(proc.PID))
	}
	return name
}

func (b *Busy) jobsByProcess(selectedIdentity string) map[string][]sidekiq.Job {
	jobsByProcess := make(map[string][]sidekiq.Job, len(b.data.Processes))
	for _, job := range b.data.Jobs {
		if selectedIdentity != "" && job.ProcessIdentity != selectedIdentity {
			continue
		}
		jobsByProcess[job.ProcessIdentity] = append(jobsByProcess[job.ProcessIdentity], job)
	}
	return jobsByProcess
}

func (b *Busy) processListLines() []string {
	if len(b.data.Processes) == 0 {
		return nil
	}

	names := make([]string, 0, len(b.data.Processes))
	maxNameLen := 0
	for _, proc := range b.data.Processes {
		name := processIdentity(proc)
		names = append(names, name)
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	maxBusyLen := 0
	maxStartedLen := 0
	for _, proc := range b.data.Processes {
		busy := fmt.Sprintf("%d/%d", proc.Busy, proc.Concurrency)
		started := format.DurationSince(proc.StartedAt)
		if len(busy) > maxBusyLen {
			maxBusyLen = len(busy)
		}
		if len(started) > maxStartedLen {
			maxStartedLen = len(started)
		}
	}

	lines := make([]string, 0, len(b.data.Processes))
	for i, proc := range b.data.Processes {
		name := fmt.Sprintf("%-*s", maxNameLen, names[i])

		nameStyle := b.styles.Text
		if i == b.selectedProcess {
			nameStyle = nameStyle.Bold(true)
		}

		hotkeyText := fmt.Sprintf("ctrl+%d", i+1)
		var hotkey string
		if i == b.selectedProcess {
			hotkey = b.styles.NavKey.Bold(true).Render(hotkeyText)
		} else {
			hotkey = b.styles.NavKey.Render(hotkeyText)
		}

		name = hotkey + nameStyle.Render(name)

		busy := fmt.Sprintf("%d/%d", proc.Busy, proc.Concurrency)
		started := format.DurationSince(proc.StartedAt)
		stats := b.styles.Muted.Render(fmt.Sprintf("  %*s  %*s", maxBusyLen, busy, maxStartedLen, started))

		lines = append(lines, name+stats)
	}

	return lines
}

func (b *Busy) renderProcessRow(proc sidekiq.Process, maxBusyLen, maxStartedLen, maxRSSLen int) string {
	name := b.styles.Muted.Render(processGlyph) + " " + b.styles.Text.Render(processIdentity(proc))
	if proc.Tag != "" {
		name += b.styles.Text.Render(" [" + proc.Tag + "]")
	}
	busy := fmt.Sprintf("%d/%d", proc.Busy, proc.Concurrency)
	started := format.DurationSince(proc.StartedAt)
	rss := format.Bytes(proc.RSS)
	stats := b.styles.Muted.Render(fmt.Sprintf("  %*s  %*s  %*s", maxBusyLen, busy, maxStartedLen, started, maxRSSLen, rss))

	queues := formatProcessCapsules(proc, b.styles.QueueText, b.styles.QueueWeight, b.styles.Muted)
	if queues != "" {
		queues = "  " + queues
	}

	return name + stats + queues
}

func formatProcessQueues(queues []string, weights map[string]int, queueStyle, weightStyle, sepStyle lipgloss.Style) string {
	if len(queues) == 0 {
		return ""
	}

	formatted := make([]string, 0, len(queues))
	for _, queue := range queues {
		queueText := queueStyle.Render(queue)
		weight := weights[queue]
		if weight >= 2 {
			queueText += queueStyle.Render("[") + weightStyle.Render(strconv.Itoa(weight)) + queueStyle.Render("]")
		}
		formatted = append(formatted, queueText)
	}
	return strings.Join(formatted, sepStyle.Render(", "))
}

type processCapsule struct {
	queues  []string
	weights map[string]int
}

func formatProcessCapsules(proc sidekiq.Process, queueStyle, weightStyle, sepStyle lipgloss.Style) string {
	capsules := processCapsules(proc)
	if len(capsules) == 0 {
		return ""
	}

	formatted := make([]string, 0, len(capsules))
	for _, capsule := range capsules {
		queues := formatProcessQueues(capsule.queues, capsule.weights, queueStyle, weightStyle, sepStyle)
		if queues == "" {
			continue
		}
		formatted = append(formatted, queues)
	}
	if len(formatted) == 0 {
		return ""
	}
	return strings.Join(formatted, sepStyle.Render("; "))
}

func processCapsules(proc sidekiq.Process) []processCapsule {
	if len(proc.Capsules) == 0 {
		return nil
	}

	// Build a stable capsule list (default first, then name) so the UI doesn't
	// reorder queues between refreshes and cause visual jitter.
	names := sortedCapsuleNames(proc.Capsules)
	capsules := make([]processCapsule, 0, len(names))
	for _, name := range names {
		capsule := proc.Capsules[name]
		queues := queuesFromWeights(capsule.Weights)
		if len(queues) == 0 {
			continue
		}
		capsules = append(capsules, processCapsule{
			queues:  queues,
			weights: capsule.Weights,
		})
	}
	if len(capsules) == 0 {
		return nil
	}
	return capsules
}

func queuesFromWeights(weights map[string]int) []string {
	if len(weights) == 0 {
		return nil
	}
	queues := slices.Collect(maps.Keys(weights))
	slices.Sort(queues)
	return queues
}

func sortedCapsuleNames(capsules map[string]sidekiq.Capsule) []string {
	names := slices.Collect(maps.Keys(capsules))
	slices.SortFunc(names, func(a, b string) int {
		if a == sidekiq.DefaultCapsuleName {
			return -1
		}
		if b == sidekiq.DefaultCapsuleName {
			return 1
		}
		return strings.Compare(a, b)
	})
	return names
}

func (b *Busy) processStatWidths(selectedIdentity string) (int, int, int) {
	maxBusyLen := 0
	maxStartedLen := 0
	maxRSSLen := 0

	for _, proc := range b.data.Processes {
		if selectedIdentity != "" && proc.Identity != selectedIdentity {
			continue
		}
		busy := fmt.Sprintf("%d/%d", proc.Busy, proc.Concurrency)
		started := format.DurationSince(proc.StartedAt)
		rss := format.Bytes(proc.RSS)

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

	return maxBusyLen, maxStartedLen, maxRSSLen
}

// renderJobDetail renders the job detail view.
