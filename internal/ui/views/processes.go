package views

import (
	"context"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// processesListDataMsg carries processes list data internally.
type processesListDataMsg struct {
	processes []sidekiq.Process
}

// ProcessesList shows all Sidekiq processes in a table.
type ProcessesList struct {
	client                  sidekiq.API
	width                   int
	height                  int
	styles                  Styles
	processes               []sidekiq.Process
	table                   table.Model
	ready                   bool
	filter                  string
	dangerousActionsEnabled bool
	frameStyles             frame.Styles
	filterStyle             filterdialog.Styles
}

// NewProcessesList creates a new ProcessesList view.
func NewProcessesList(client sidekiq.API) *ProcessesList {
	return &ProcessesList{
		client: client,
		table: table.New(
			table.WithColumns(processesListColumns),
			table.WithEmptyMessage("No processes"),
		),
	}
}

// Init implements View.
func (p *ProcessesList) Init() tea.Cmd {
	p.reset()
	return p.fetchDataCmd()
}

// Update implements View.
func (p *ProcessesList) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case processesListDataMsg:
		p.processes = msg.processes
		p.ready = true
		p.updateTableRows()
		return p, nil

	case RefreshMsg:
		return p, p.fetchDataCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return p, nil
		}
		if msg.Query == p.filter {
			return p, nil
		}
		p.filter = msg.Query
		p.table.SetCursor(0)
		return p, p.fetchDataCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return p, func() tea.Msg {
				return dialogs.OpenDialogMsg{
					Model: filterdialog.New(
						filterdialog.WithStyles(p.filterStyle),
						filterdialog.WithQuery(p.filter),
					),
				}
			}
		case "c":
			if identity, ok := p.selectedProcessIdentity(); ok {
				return p, copyTextCmd(identity)
			}
			return p, nil
		case "enter":
			if idx := p.table.Cursor(); idx >= 0 && idx < len(p.processes) {
				identity := p.processes[idx].Identity
				return p, func() tea.Msg {
					return ShowProcessSelectMsg{Identity: identity}
				}
			}
			return p, nil
		}

		if p.dangerousActionsEnabled {
			switch msg.String() {
			case "p":
				if identity, ok := p.selectedProcessIdentity(); ok {
					return p, p.pauseProcessCmd(identity)
				}
				return p, nil
			case "s":
				if identity, ok := p.selectedProcessIdentity(); ok {
					return p, p.stopProcessCmd(identity)
				}
				return p, nil
			}
		}

		p.table, _ = p.table.Update(msg)
		return p, nil
	}

	return p, nil
}

// View implements View.
func (p *ProcessesList) View() string {
	if !p.ready {
		return p.renderMessage("Loading...")
	}

	if len(p.processes) == 0 {
		if p.filter != "" {
			return p.renderMessage("No processes matching filter")
		}
		return p.renderMessage("No processes")
	}

	return p.renderProcessesBox()
}

// Name implements View.
func (p *ProcessesList) Name() string {
	return "Select process"
}

// ShortHelp implements View.
func (p *ProcessesList) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (p *ProcessesList) ContextItems() []ContextItem {
	if len(p.processes) == 0 {
		return nil
	}

	processCount := len(p.processes)
	totalThreads := 0
	busyThreads := 0
	var totalRSS int64
	var oldestStart time.Time

	for _, proc := range p.processes {
		totalThreads += proc.Concurrency
		busyThreads += proc.Busy
		totalRSS += proc.RSS
		if !proc.StartedAt.IsZero() && (oldestStart.IsZero() || proc.StartedAt.Before(oldestStart)) {
			oldestStart = proc.StartedAt
		}
	}

	percentage := 0
	if totalThreads > 0 {
		percentage = (busyThreads * 100) / totalThreads
	}

	oldestAge := "-"
	if !oldestStart.IsZero() {
		oldestAge = format.DurationSince(oldestStart)
	}

	return []ContextItem{
		{Label: "Processes", Value: strconv.Itoa(processCount)},
		{Label: "Capacity", Value: strconv.Itoa(totalThreads)},
		{Label: "Busy", Value: strconv.Itoa(busyThreads) + " (" + strconv.Itoa(percentage) + "%)"},
		{Label: "RSS", Value: format.Bytes(totalRSS)},
		{Label: "Oldest", Value: oldestAge},
	}
}

// HintBindings implements HintProvider.
func (p *ProcessesList) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"enter"}, "enter", "select process"),
	}
}

// MutationBindings implements MutationHintProvider.
func (p *ProcessesList) MutationBindings() []key.Binding {
	if !p.dangerousActionsEnabled {
		return nil
	}
	return []key.Binding{
		helpBinding([]string{"p"}, "p", "pause process"),
		helpBinding([]string{"s"}, "s", "stop process"),
	}
}

// HelpSections implements HelpProvider.
func (p *ProcessesList) HelpSections() []HelpSection {
	sections := []HelpSection{{
		Title: "Process Actions",
		Bindings: []key.Binding{
			helpBinding([]string{"/"}, "/", "filter processes"),
			helpBinding([]string{"c"}, "c", "copy identity"),
			helpBinding([]string{"enter"}, "enter", "select process"),
		},
	}}
	if p.dangerousActionsEnabled {
		sections = append(sections, HelpSection{
			Title: "Dangerous Actions",
			Bindings: []key.Binding{
				helpBinding([]string{"p"}, "p", "pause process"),
				helpBinding([]string{"s"}, "s", "stop process"),
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (p *ProcessesList) TableHelp() []key.Binding {
	return tableHelpBindings(p.table.KeyMap)
}

// SetSize implements View.
func (p *ProcessesList) SetSize(width, height int) View {
	p.width = width
	p.height = height
	p.updateTableSize()
	return p
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (p *ProcessesList) SetDangerousActionsEnabled(enabled bool) {
	p.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (p *ProcessesList) Dispose() {
	p.reset()
	p.updateTableSize()
}

// SetStyles implements View.
func (p *ProcessesList) SetStyles(styles Styles) View {
	p.styles = styles
	p.table.SetStyles(tableStylesFromTheme(styles))
	p.frameStyles = frameStylesFromTheme(styles)
	p.filterStyle = filterDialogStylesWithPrompt(styles)
	return p
}

// fetchDataCmd fetches processes data from Redis.
func (p *ProcessesList) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "processes.fetchDataCmd")

		data, err := p.client.GetBusyData(ctx, "")
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		processes := make([]sidekiq.Process, 0, len(data.Processes))
		for _, process := range data.Processes {
			if !p.matchesFilter(process) {
				continue
			}
			processes = append(processes, process)
		}

		return processesListDataMsg{processes: processes}
	}
}

func (p *ProcessesList) matchesFilter(proc sidekiq.Process) bool {
	if p.filter == "" {
		return true
	}

	needle := strings.ToLower(p.filter)
	name := strings.ToLower(processIdentity(proc))
	if strings.Contains(name, needle) {
		return true
	}
	if strings.Contains(strings.ToLower(proc.Identity), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(proc.Tag), needle) {
		return true
	}
	for _, capsule := range processCapsules(proc) {
		for _, queue := range capsule.queues {
			if strings.Contains(strings.ToLower(queue), needle) {
				return true
			}
		}
	}

	return false
}

func (p *ProcessesList) reset() {
	p.ready = false
	p.processes = nil
	p.table.SetRows(nil)
	p.table.SetCursor(0)
}

func (p *ProcessesList) selectedProcessIdentity() (string, bool) {
	idx := p.table.Cursor()
	if idx < 0 || idx >= len(p.processes) {
		return "", false
	}
	return p.processes[idx].Identity, true
}

func (p *ProcessesList) pauseProcessCmd(identity string) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "processes.pauseProcessCmd")
		if err := p.client.NewProcess(identity).Pause(ctx); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (p *ProcessesList) stopProcessCmd(identity string) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "processes.stopProcessCmd")
		if err := p.client.NewProcess(identity).Stop(ctx); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// Table columns for processes list.
var processesListColumns = []table.Column{
	{Title: "Name", Width: 28},
	{Title: "Started", Width: 8},
	{Title: "RSS", Width: 10, Align: table.AlignRight},
	{Title: "Capacity", Width: 8, Align: table.AlignRight},
	{Title: "Busy", Width: 5, Align: table.AlignRight},
	{Title: "Status", Width: 9},
	{Title: "Queues", Width: 30},
	{Title: "Version", Width: 10},
}

// updateTableSize updates the table dimensions based on current view size.
func (p *ProcessesList) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(p.height-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := p.width - 4
	p.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts process data to table rows.
func (p *ProcessesList) updateTableRows() {
	rows := make([]table.Row, 0, len(p.processes))
	for _, process := range p.processes {
		name := processIdentity(process)
		if process.Tag != "" {
			name += " [" + process.Tag + "]"
		}

		queues := formatProcessCapsules(process, p.styles.QueueText, p.styles.QueueWeight, p.styles.Muted)
		version := process.Version
		if version == "" {
			version = "-"
		}

		row := table.Row{
			ID: process.Identity,
			Cells: []string{
				name,
				format.DurationSince(process.StartedAt),
				format.Bytes(process.RSS),
				strconv.Itoa(process.Concurrency),
				strconv.Itoa(process.Busy),
				process.Status,
				queues,
				version,
			},
		}
		rows = append(rows, row)
	}
	p.table.SetRows(rows)
	p.updateTableSize()
}

// renderProcessesBox renders the bordered box containing the processes table.
func (p *ProcessesList) renderProcessesBox() string {
	boxHeight := p.height
	content := p.table.View()

	box := frame.New(
		frame.WithStyles(p.frameStyles),
		frame.WithTitle("Select process"),
		frame.WithFilter(p.filter),
		frame.WithTitlePadding(0),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(p.width, boxHeight),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

func (p *ProcessesList) renderMessage(msg string) string {
	return renderStatusMessage("Select process", msg, p.styles, p.width, p.height)
}
