package views

import (
	"context"
	"fmt"
	"time"

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

// sortedJobsSpec describes one sorted-set screen: what it reads, how it
// renders rows, and which dangerous actions it offers.
type sortedJobsSpec struct {
	name             string // view name and help section title
	title            string // frame title
	tracker          string // devtools origin prefix
	kind             sidekiq.SortedSetKind
	columns          []table.Column
	emptyMessage     string
	windowPages      int
	fallbackPageSize int
	cells            func(styles Styles, now time.Time, job *sidekiq.SortedEntry) []string
	contextItems     func(now time.Time, first, last *sidekiq.SortedEntry) []ContextItem
	actions          []sortedJobAction // in help order
}

// sortedJobAction is a confirmed mutation bound to a key. It runs either on
// the selected entry (runEntry) or on the whole set (runAll).
type sortedJobAction struct {
	key      string
	helpKey  string
	help     string
	op       string // devtools origin suffix
	title    string
	message  string // per-entry actions format the job name into %s
	runEntry func(sidekiq.API, context.Context, sidekiq.SortedSetKind, *sidekiq.SortedEntry) error
	runAll   func(sidekiq.API, context.Context, sidekiq.SortedSetKind) error
}

var (
	sortedDeleteJobAction = sortedJobAction{
		key: "D", helpKey: "shift+d", help: "delete job", op: "deleteJobCmd",
		title:    "Delete job",
		message:  "Are you sure you want to delete the %s job?\n\nThis action is not recoverable.",
		runEntry: sidekiq.API.DeleteSortedEntry,
	}
	sortedRetryJobAction = sortedJobAction{
		key: "R", helpKey: "shift+r", help: "retry now", op: "retryNowJobCmd",
		title:    "Retry job",
		message:  "Retry the %s job now?\n\nThis will enqueue it immediately.",
		runEntry: sidekiq.API.EnqueueSortedEntry,
	}
)

// SortedJobs lists the jobs of one Sidekiq sorted set (retries, scheduled, or dead).
type SortedJobs struct {
	detailListView
	client                  sidekiq.API
	spec                    *sortedJobsSpec
	jobs                    []*sidekiq.SortedEntry
	firstEntry              *sidekiq.SortedEntry
	lastEntry               *sidekiq.SortedEntry
	dangerousActionsEnabled bool
	pendingConfirm          pendingConfirm[*sortedJobAction]
}

func newSortedJobs(client sidekiq.API, spec *sortedJobsSpec) *SortedJobs {
	v := &SortedJobs{
		client: client,
		spec:   spec,
		detailListView: newDetailListView(
			spec.title,
			spec.columns,
			spec.emptyMessage,
			spec.windowPages,
			spec.fallbackPageSize,
		),
	}
	v.lazy.SetFetcherFactory(func() lazytable.Fetcher {
		snapshot := *v
		return snapshot.fetchWindow
	})
	return v
}

// Init implements View.
func (v *SortedJobs) Init() tea.Cmd {
	return v.init(v.reset)
}

// Update implements View.
func (v *SortedJobs) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		v.lazy, cmd = v.lazy.Update(msg)
		return v, cmd

	case lazytable.DataMsg:
		_, cmd := v.handleData(msg, func(result lazytable.FetchResult) {
			if payload, ok := result.Payload.(sortedEntriesPayload); ok {
				v.jobs = payload.jobs
				v.firstEntry = payload.firstEntry
				v.lastEntry = payload.lastEntry
			}
		})
		return v, cmd

	case RefreshMsg:
		return v, v.refreshWindow()

	case filterdialog.ActionMsg:
		return v, v.handleFilterAction(msg, v.updateEmptyMessage)

	case confirmdialog.ActionMsg:
		action, entry, ok := v.pendingConfirm.Confirm(msg, v.dangerousActionsEnabled, nil)
		if !ok {
			return v, nil
		}
		return v, v.actionCmd(action, entry)

	case tea.KeyPressMsg:
		return v, v.handleKey(msg)
	}

	return v, nil
}

// View implements View.
func (v *SortedJobs) View() string {
	if !v.ready {
		return v.renderLoadingMessage()
	}
	return v.renderBox(v.spec.title, len(v.jobs))
}

// Name implements View.
func (v *SortedJobs) Name() string {
	return v.spec.name
}

// ShortHelp implements View.
func (v *SortedJobs) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (v *SortedJobs) ContextItems() []ContextItem {
	items := v.spec.contextItems(time.Now(), v.firstEntry, v.lastEntry)
	return append(items, ContextItem{Label: "Total items", Value: display.Number(v.lazy.Total())})
}

// HintBindings implements HintProvider.
func (v *SortedJobs) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// MutationBindings implements MutationHintProvider.
func (v *SortedJobs) MutationBindings() []key.Binding {
	if !v.dangerousActionsEnabled {
		return nil
	}
	return v.actionBindings()
}

// HelpSections implements HelpProvider.
func (v *SortedJobs) HelpSections() []HelpSection {
	sections := []HelpSection{
		{
			Title: v.spec.name,
			Bindings: []key.Binding{
				helpBinding([]string{"/"}, "/", "filter"),
				helpBinding([]string{"ctrl+u"}, "ctrl+u", "clear filter"),
				helpBinding([]string{"["}, "[", "page up"),
				helpBinding([]string{"]"}, "]", "page down"),
				helpBinding([]string{"g"}, "g", "jump to start"),
				helpBinding([]string{"G"}, "shift+g", "jump to end"),
				helpBinding([]string{"c"}, "c", "copy jid"),
				helpBinding([]string{"enter"}, "enter", "job detail"),
			},
		},
	}
	if v.dangerousActionsEnabled {
		sections = append(sections, HelpSection{
			Title:    "Dangerous Actions",
			Bindings: v.actionBindings(),
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (v *SortedJobs) TableHelp() []key.Binding {
	return v.tableHelp()
}

// SetSize implements View.
func (v *SortedJobs) SetSize(width, height int) View {
	v.setSize(width, height)
	return v
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (v *SortedJobs) SetDangerousActionsEnabled(enabled bool) {
	v.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (v *SortedJobs) Dispose() {
	v.dispose(v.reset)
}

// CancelRequests stops in-flight fetches when the view is hidden.
func (v *SortedJobs) CancelRequests() {
	v.pendingConfirm.Clear(nil)
	v.cancelRequests()
}

// SetStyles implements View.
func (v *SortedJobs) SetStyles(styles Styles) View {
	v.setStyles(styles)
	return v
}

func (v *SortedJobs) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	if handled, cmd := v.handleKeyPress(msg, v.updateEmptyMessage); handled {
		return cmd
	}

	switch msg.String() {
	case "c":
		if entry, ok := v.selectedSortedEntry(); ok {
			return copyTextCmd(entry.JID())
		}
		return nil
	case "enter":
		if entry, ok := v.selectedSortedEntry(); ok {
			return v.EmitIntent(ShowJobDetailMsg{Job: entry.JobRecord})
		}
		return nil
	}

	if v.dangerousActionsEnabled && v.pendingConfirm.target == "" {
		for i := range v.spec.actions {
			if action := &v.spec.actions[i]; action.key == msg.String() {
				return v.openConfirm(action)
			}
		}
	}

	return v.updateKeyPress(msg)
}

func (v *SortedJobs) actionBindings() []key.Binding {
	bindings := make([]key.Binding, 0, len(v.spec.actions))
	for _, action := range v.spec.actions {
		bindings = append(bindings, helpBinding([]string{action.key}, action.helpKey, action.help))
	}
	return bindings
}

func (v *SortedJobs) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	return fetchSortedEntriesWindow(ctx, sortedEntriesFetchConfig{
		tracker:          v.spec.tracker + ".fetchWindow",
		client:           v.client,
		kind:             v.spec.kind,
		filter:           v.filter,
		windowStart:      windowStart,
		windowSize:       windowSize,
		fallbackPageSize: v.spec.fallbackPageSize,
		windowPages:      v.spec.windowPages,
		buildRows:        v.buildRows,
	})
}

func (v *SortedJobs) reset() {
	v.pendingConfirm.Clear(nil)
	v.jobs = nil
	v.firstEntry = nil
	v.lastEntry = nil
	v.resetShell()
	v.updateEmptyMessage()
}

func (v *SortedJobs) updateEmptyMessage() {
	msg := v.spec.emptyMessage
	if v.filter != "" {
		msg = "No matches"
	}
	v.lazy.SetEmptyMessage(msg)
}

func (v *SortedJobs) buildRows(jobs []*sidekiq.SortedEntry) []table.Row {
	rows := make([]table.Row, 0, len(jobs))
	now := time.Now()
	for _, job := range jobs {
		rows = append(rows, table.Row{ID: job.JID(), Cells: v.spec.cells(v.styles, now, job)})
	}
	return rows
}

func (v *SortedJobs) selectedSortedEntry() (*sidekiq.SortedEntry, bool) {
	idx := v.lazy.Table().Cursor()
	if idx < 0 || idx >= len(v.jobs) {
		return nil, false
	}
	return v.jobs[idx], true
}

// openConfirm prepares action and asks to confirm it. Per-entry actions bind
// to the selected entry and do nothing without one.
func (v *SortedJobs) openConfirm(action *sortedJobAction) tea.Cmd {
	message := action.message
	if action.runAll != nil {
		v.pendingConfirm.Set(action, nil, v.spec.tracker+"."+action.op)
	} else {
		entry, ok := v.selectedSortedEntry()
		if !ok {
			return nil
		}
		v.pendingConfirm.SetForEntry(action, entry)
		message = fmt.Sprintf(message, v.styles.Text.Bold(true).Render(sortedJobName(entry)))
	}
	return v.pendingConfirm.OpenDialog(v.Opener, newConfirmDialog(
		v.styles,
		action.title,
		message,
		v.pendingConfirm.target,
		v.styles.DangerAction,
	))
}

// actionCmd runs a confirmed action and refreshes the list.
func (v *SortedJobs) actionCmd(action *sortedJobAction, entry *sidekiq.SortedEntry) tea.Cmd {
	if action.runAll == nil && entry == nil {
		return nil
	}
	ctx := devtools.WithTracker(v.Context(), v.spec.tracker+"."+action.op)
	client, kind := v.client, v.spec.kind
	return func() tea.Msg {
		var err error
		if action.runAll != nil {
			err = action.runAll(client, ctx, kind)
		} else {
			err = action.runEntry(client, ctx, kind, entry)
		}
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func sortedJobName(entry *sidekiq.SortedEntry) string {
	if name := entry.DisplayClass(); name != "" {
		return name
	}
	return "selected"
}

// entryAge formats how long ago entry is due, or "-" without an entry.
func entryAge(now time.Time, entry *sidekiq.SortedEntry) string {
	if entry == nil {
		return "-"
	}
	return display.Duration(int64(now.Sub(entry.At()).Seconds()))
}

// entryWait formats how long until entry is due, or "-" without an entry.
func entryWait(now time.Time, entry *sidekiq.SortedEntry) string {
	if entry == nil {
		return "-"
	}
	return display.Duration(int64(entry.At().Sub(now).Seconds()))
}
