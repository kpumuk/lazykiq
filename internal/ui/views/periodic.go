package views

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
	"github.com/kpumuk/lazykiq/internal/ui/requestctx"
)

type periodicDataMsg struct {
	jobs    []sidekiq.PeriodicJob
	job     *sidekiq.PeriodicJob
	history []sidekiq.PeriodicExecution
	now     time.Time
}

// Periodic displays recurring definitions, or a stacked definition and history.
type Periodic struct {
	requestctx.Scope
	client                  sidekiq.API
	capabilities            sidekiq.Capabilities
	detail                  bool
	lid                     string
	width                   int
	height                  int
	styles                  Styles
	ready                   bool
	jobs                    []sidekiq.PeriodicJob
	rows                    []sidekiq.PeriodicJob
	job                     *sidekiq.PeriodicJob
	history                 []sidekiq.PeriodicExecution
	table                   table.Model
	filter                  string
	focusHistory            bool
	infoOffset              int
	now                     time.Time
	notice                  string
	dangerousActionsEnabled bool
	request                 requestctx.Controller
	actions                 periodicActions
}

// NewPeriodic creates the Enterprise recurring-job list.
func NewPeriodic(client sidekiq.API) *Periodic {
	p := &Periodic{client: client, table: table.New(table.WithColumns(periodicColumns), table.WithEmptyMessage("No periodic jobs"))}
	p.table.KeyMap.PageUp = helpBinding([]string{"[", "pgup", "alt+left"}, "[/pgup", "page up")
	p.table.KeyMap.PageDown = helpBinding([]string{"]", "pgdown", "alt+right"}, "]/pgdn", "page down")
	if client != nil {
		p.capabilities = client.GetCapabilities()
	}
	return p
}

// NewPeriodicDetail creates the stacked information and enqueue-history view.
func NewPeriodicDetail(client sidekiq.API) *Periodic {
	p := NewPeriodic(client)
	p.detail = true
	p.table.SetColumns(periodicHistoryColumns)
	p.table.SetEmptyMessage("No enqueue history")
	return p
}

// SetPeriodicJob selects the definition for the stacked view.
func (p *Periodic) SetPeriodicJob(lid string) {
	p.Dispose()
	p.lid = lid
}

// Init implements View.
func (p *Periodic) Init() tea.Cmd {
	if p.actions.busy {
		return nil
	}
	p.ready = false
	return p.fetch()
}

// Update implements View.
func (p *Periodic) Update(msg tea.Msg) (View, tea.Cmd) {
	if result, ok := msg.(ReadResultMsg); ok {
		if result.Owner != p || !result.Current() || result.Err != nil {
			return p, nil
		}
		msg = result.Data
	}
	switch msg := msg.(type) {
	case periodicDataMsg:
		if p.actions.busy {
			return p, nil
		}
		p.jobs, p.job, p.history, p.now = msg.jobs, msg.job, msg.history, msg.now
		p.ready = true
		p.buildRows()
		p.clampInfoOffset()
	case RefreshMsg:
		if !p.actions.busy {
			return p, p.fetch()
		}
	case PeriodicActionResultMsg:
		if p.actions.complete(msg) {
			p.notice = ""
			if msg.Err == nil {
				p.notice = msg.notice
			}
			return p, p.fetch()
		}
		if !p.actions.busy {
			return p, p.fetch()
		}
	case filterdialog.ActionMsg:
		if !p.detail && msg.Action != filterdialog.ActionNone {
			p.filter = msg.Query
			p.buildRows()
			p.table.GotoTop()
		}
	case confirmdialog.ActionMsg:
		if p.canMutate() {
			cmd := p.actions.confirm(p.Context(), msg, p.client)
			if cmd != nil {
				p.request.Cancel()
			}
			return p, cmd
		}
	case tea.KeyPressMsg:
		if p.actions.busy {
			return p, nil
		}
		return p, p.handleKey(msg)
	}
	return p, nil
}

// View implements View.
func (p *Periodic) View() string {
	if p.width <= 0 || p.height <= 0 {
		return ""
	}
	if !p.ready {
		return renderStatusMessage(p.Name(), "Loading...", p.styles, p.width, p.height)
	}
	if !p.detail {
		return p.renderFrame("Periodic", p.table.View(), p.width, p.height, true, p.filter)
	}
	if p.job == nil {
		return renderStatusMessage("Periodic Job", "Periodic job is no longer registered", p.styles, p.width, p.height)
	}
	leftWidth, leftHeight, rightWidth, rightHeight := p.panelSizes()
	if p.singlePane() && p.focusHistory {
		return p.renderFrame("Enqueue History", p.table.View(), rightWidth, rightHeight, true, "")
	}
	lines := p.infoLines(max(leftWidth-4, 1))
	end := min(p.infoOffset+max(leftHeight-2, 0), len(lines))
	info := strings.Join(lines[p.infoOffset:end], "\n")
	left := p.renderFrame("Periodic Job", info, leftWidth, leftHeight, !p.focusHistory, "")
	if p.singlePane() {
		return left
	}
	right := p.renderFrame("Enqueue History", p.table.View(), rightWidth, rightHeight, p.focusHistory, "")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// Name implements View.
func (p *Periodic) Name() string {
	if p.detail {
		if p.job != nil {
			return p.job.Class
		}
		return "Periodic Job"
	}
	return "Periodic"
}

// ShortHelp implements View.
func (p *Periodic) ShortHelp() []key.Binding { return nil }

// ContextItems implements ContextProvider.
func (p *Periodic) ContextItems() []ContextItem {
	var items []ContextItem
	if p.detail {
		if p.job == nil {
			return nil
		}
		class := p.job.Class
		if p.job.Paused {
			class += "  " + p.styles.QueuePaused.Render("Paused")
		}
		items = []ContextItem{
			{Label: "Class", Value: class},
			{Label: "Queue", Value: p.styles.QueueText.Render(p.job.Queue())},
			{Label: "Schedule", Value: p.job.Schedule + " [" + p.job.EffectiveTimezone() + "]"},
			{Label: "Next Run", Value: periodicNextRun(*p.job, p.now, true)},
		}
	} else {
		paused := 0
		for _, job := range p.jobs {
			if job.Paused {
				paused++
			}
		}
		items = []ContextItem{
			{Label: "Schedules", Value: strconv.Itoa(len(p.jobs))},
			{Label: "Paused", Value: strconv.Itoa(paused)},
			{Label: "Matching", Value: strconv.Itoa(len(p.rows))},
		}
	}
	if p.notice != "" {
		items = append(items, ContextItem{Label: "Last action", Value: p.notice})
	}
	return items
}

// HintBindings implements HintProvider.
func (p *Periodic) HintBindings() []key.Binding {
	if p.detail {
		return []key.Binding{helpBinding([]string{"tab", "shift+tab"}, "tab", "switch panel"), helpBinding([]string{"c"}, "c", "copy lid/jid"), helpBinding([]string{"r"}, "r", "refresh")}
	}
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"), helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"enter"}, "enter", "periodic job detail"), helpBinding([]string{"c"}, "c", "copy lid"), helpBinding([]string{"r"}, "r", "refresh"),
	}
}

// MutationBindings implements MutationHintProvider.
func (p *Periodic) MutationBindings() []key.Binding {
	job, ok := p.selectedJob()
	if !p.canMutate() || !ok {
		return nil
	}
	verb := "pause schedule"
	if job.Paused {
		verb = "resume schedule"
	}
	return []key.Binding{helpBinding([]string{"e"}, "e", "enqueue now"), helpBinding([]string{"p"}, "p", verb)}
}

// HelpSections implements HelpProvider.
func (p *Periodic) HelpSections() []HelpSection {
	sections := []HelpSection{{Title: p.Name(), Bindings: p.HintBindings(), Lines: []string{"History records enqueues, not execution outcomes."}}}
	if bindings := p.MutationBindings(); len(bindings) != 0 {
		sections = append(sections, HelpSection{Title: "Dangerous Actions", Bindings: bindings})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (p *Periodic) TableHelp() []key.Binding { return tableHelpBindings(p.table.KeyMap) }

// SetSize implements View.
func (p *Periodic) SetSize(width, height int) View {
	p.width, p.height = max(width, 0), max(height, 0)
	tableWidth, tableHeight := p.width, p.height
	if p.detail {
		_, _, tableWidth, tableHeight = p.panelSizes()
	}
	p.table.SetSize(max(tableWidth-4, 0), max(tableHeight-2, 0))
	p.clampInfoOffset()
	return p
}

// SetStyles implements View.
func (p *Periodic) SetStyles(styles Styles) View {
	p.styles = styles
	p.table.SetStyles(tableStylesFromTheme(styles))
	if p.ready {
		p.buildRows()
	}
	return p
}

// SetDangerousActionsEnabled toggles confirmed mutations.
func (p *Periodic) SetDangerousActionsEnabled(enabled bool) {
	p.dangerousActionsEnabled = enabled
	if !enabled {
		p.actions.pending = nil
	}
}

// CancelRequests stops replaceable reads when hidden.
func (p *Periodic) CancelRequests() {
	p.request.Cancel()
	p.actions.reset()
}

// Dispose releases data, in-flight reads, and pending confirmations.
func (p *Periodic) Dispose() {
	p.CancelRequests()
	p.jobs, p.rows, p.history, p.job = nil, nil, nil, nil
	p.ready, p.focusHistory, p.infoOffset, p.notice = false, false, 0, ""
	p.table.SetRows(nil)
}

func (p *Periodic) canMutate() bool {
	return p.dangerousActionsEnabled && p.capabilities.SupportsPeriodicJobs()
}

func (p *Periodic) fetch() tea.Cmd {
	client, detail, lid := p.client, p.detail, p.lid
	ctx := p.request.Start(devtools.WithTracker(p.Context(), "periodic.fetch"))
	return readCmd(ctx, p, func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		data := periodicDataMsg{now: time.Now()}
		var err error
		if detail {
			data.job, data.history, err = client.GetPeriodicJob(ctx, lid)
		} else {
			data.jobs, err = client.GetPeriodicJobs(ctx)
		}
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return data
	})
}

func (p *Periodic) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "r":
		return p.fetch()
	case "tab", "shift+tab":
		if p.detail {
			p.focusHistory = !p.focusHistory
		}
		return nil
	case "c":
		if p.detail && p.focusHistory {
			if row := p.table.SelectedRow(); row.ID != "" {
				return copyTextCmd(row.ID)
			}
		} else if job, ok := p.selectedJob(); ok {
			return copyTextCmd(job.LID)
		}
		return nil
	case "enter":
		if !p.detail {
			if job, ok := p.selectedJob(); ok {
				return func() tea.Msg { return ShowPeriodicDetailMsg{LID: job.LID} }
			}
		}
		return nil
	case "/":
		if !p.detail {
			return func() tea.Msg {
				return dialogs.OpenDialogMsg{Model: filterdialog.New(filterdialog.WithStyles(filterDialogStylesFromTheme(p.styles)), filterdialog.WithQuery(p.filter))}
			}
		}
	case "ctrl+u":
		if !p.detail {
			p.filter = ""
			p.buildRows()
			p.table.GotoTop()
		}
	case "e", "p":
		if p.canMutate() {
			if job, ok := p.selectedJob(); ok {
				return p.actions.dialog(job, msg.String() == "e", p.styles)
			}
		}
		return nil
	}
	if p.detail && !p.focusHistory {
		_, panelHeight, _, _ := p.panelSizes()
		switch msg.String() {
		case "up", "k":
			p.infoOffset--
		case "down", "j":
			p.infoOffset++
		case "[", "pgup", "alt+left":
			p.infoOffset -= max(panelHeight-3, 1)
		case "]", "pgdown", "alt+right":
			p.infoOffset += max(panelHeight-3, 1)
		case "g", "home":
			p.infoOffset = 0
		case "G", "end":
			p.infoOffset = p.maxInfoOffset()
		}
		p.clampInfoOffset()
		return nil
	}
	p.table, _ = p.table.Update(msg)
	return nil
}

func (p *Periodic) selectedJob() (sidekiq.PeriodicJob, bool) {
	if p.detail {
		if p.job != nil {
			return *p.job, true
		}
		return sidekiq.PeriodicJob{}, false
	}
	index := p.table.Cursor()
	if index < 0 || index >= len(p.rows) {
		return sidekiq.PeriodicJob{}, false
	}
	return p.rows[index], true
}

func (p *Periodic) buildRows() {
	var rows []table.Row
	if p.detail {
		rows = make([]table.Row, len(p.history))
		for i, event := range p.history {
			rows[i] = table.Row{ID: event.JID, Cells: []string{event.EnqueuedAt.UTC().Format("2006-01-02 15:04:05 UTC"), event.JID}}
		}
	} else {
		p.rows = nil
		for _, job := range p.jobs {
			if p.filter != "" && !strings.Contains(job.Class+" "+job.Schedule+" "+job.Options+" "+job.LID+" "+job.Timezone, p.filter) {
				continue
			}
			p.rows = append(p.rows, job)
			state := periodicState(job)
			if job.Paused {
				state = p.styles.QueuePaused.Render(state)
			}
			rows = append(rows, table.Row{ID: job.LID, Cells: []string{job.Class, job.Schedule, state, p.styles.QueueText.Render(job.Queue()), periodicNextRun(job, p.now, false), job.Options}})
		}
		empty := "No periodic jobs"
		if p.filter != "" {
			empty = "No matches"
		}
		p.table.SetEmptyMessage(empty)
	}
	p.table.SetRows(rows)
}

func (p *Periodic) singlePane() bool { return p.width < 70 }

func (p *Periodic) panelSizes() (int, int, int, int) {
	if p.singlePane() {
		return p.width, p.height, p.width, p.height
	}
	leftWidth := max(p.width*40/100, 28)
	return leftWidth, p.height, p.width - leftWidth, p.height
}

func (p *Periodic) maxInfoOffset() int {
	width, height, _, _ := p.panelSizes()
	return max(len(p.infoLines(max(width-4, 1)))-max(height-2, 0), 0)
}

func (p *Periodic) clampInfoOffset() {
	p.infoOffset = mathutil.Clamp(p.infoOffset, 0, p.maxInfoOffset())
}

func (p *Periodic) infoLines(width int) []string {
	if p.job == nil {
		return nil
	}
	job := *p.job
	var pretty bytes.Buffer
	options := job.Options
	if json.Indent(&pretty, []byte(options), "", "  ") == nil {
		options = pretty.String()
	}
	properties := []PropertyRow{
		{Label: "Class", Value: job.Class},
		{Label: "Schedule", Value: job.Schedule},
		{Label: "State", Value: periodicState(job)},
		{Label: "Queue", Value: job.Queue()},
		{Label: "Timezone", Value: job.EffectiveTimezone()},
		{Label: "Next Run", Value: periodicNextRun(job, p.now, true)},
		{Label: "LID", Value: job.LID},
		{Label: "Options", Value: options},
	}
	var lines []string
	for _, prop := range properties {
		lines = append(lines, p.styles.Muted.Render(prop.Label+":"))
		for _, line := range wrapText(prop.Value, max(width-2, 1)) {
			lines = append(lines, "  "+p.styles.Text.Render(line))
		}
	}
	return lines
}

func (p *Periodic) renderFrame(title, content string, width, height int, focused bool, filter string) string {
	if width < 4 || height < 2 {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", max(width, 0))+"\n", max(height, 0)), "\n")
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = display.HorizontalScroll(line, 0, width-4)
	}
	return frame.New(frame.WithStyles(frameStylesFromTheme(p.styles)), frame.WithTitle(title), frame.WithFilter(filter),
		frame.WithTitlePadding(0), frame.WithPadding(1), frame.WithContent(strings.Join(lines, "\n")),
		frame.WithSize(width, height), frame.WithFocused(focused)).View()
}

func periodicState(job sidekiq.PeriodicJob) string {
	if job.Paused {
		return "Paused"
	}
	return "Active"
}

func periodicNextRun(job sidekiq.PeriodicJob, now time.Time, countdown bool) string {
	next, err := job.NextRun(now)
	if err != nil {
		return "Unknown: " + err.Error()
	}
	value := next.Format("2006-01-02 15:04:05 MST")
	if countdown {
		value += " (in " + display.Duration(int64(next.Sub(now).Seconds())) + ")"
	}
	return value
}

var periodicColumns = []table.Column{
	{Title: "Job", Width: 28},
	{Title: "Schedule", Width: 15},
	{Title: "State", Width: 8},
	{Title: "Queue", Width: 12},
	{Title: "Next Run", Width: 26},
	{Title: "Options", Width: 40},
}

var periodicHistoryColumns = []table.Column{
	{Title: "Enqueued", Width: 23}, {Title: "JID", Width: 24},
}
