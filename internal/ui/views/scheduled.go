package views

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

const (
	scheduledWindowPages      = 3
	scheduledFallbackPageSize = 25
)

type scheduledJobAction int

const (
	scheduledJobActionNone scheduledJobAction = iota
	scheduledJobActionDelete
	scheduledJobActionAddToQueue
	scheduledJobActionDeleteAll
	scheduledJobActionAddAllToQueue
)

// Scheduled shows jobs scheduled for future execution.
type Scheduled struct {
	client sidekiq.API
	sortedJobsView
	dangerousActionsEnabled bool
	pendingConfirm          pendingConfirm[scheduledJobAction]
}

// NewScheduled creates a new Scheduled view.
func NewScheduled(client sidekiq.API) *Scheduled {
	s := &Scheduled{
		client: client,
		sortedJobsView: newSortedJobsView(
			"Scheduled",
			scheduledJobColumns,
			"No scheduled jobs",
			scheduledWindowPages,
			scheduledFallbackPageSize,
		),
	}
	s.lazy.SetFetcher(s.fetchWindow)
	return s
}

// Init implements View.
func (s *Scheduled) Init() tea.Cmd {
	return s.init(s.reset)
}

// Update implements View.
func (s *Scheduled) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if handled, cmd := s.handleSortedEntriesData(msg); handled {
			return s, cmd
		}
		return s, nil

	case RefreshMsg:
		return s, s.refreshWindow()

	case filterdialog.ActionMsg:
		return s, s.handleFilterAction(msg, s.updateEmptyMessage)

	case confirmdialog.ActionMsg:
		action, entry, ok := s.pendingConfirm.Confirm(msg, s.dangerousActionsEnabled, scheduledJobActionNone)
		if !ok {
			return s, nil
		}
		switch action {
		case scheduledJobActionNone:
			return s, nil
		case scheduledJobActionDelete:
			if entry == nil {
				return s, nil
			}
			return s, s.deleteJobCmd(entry)
		case scheduledJobActionAddToQueue:
			if entry == nil {
				return s, nil
			}
			return s, s.addToQueueJobCmd(entry)
		case scheduledJobActionDeleteAll:
			return s, s.deleteAllCmd()
		case scheduledJobActionAddAllToQueue:
			return s, s.addAllToQueueCmd()
		}

	case tea.KeyPressMsg:
		if handled, cmd := s.handleKeyPress(msg, s.updateEmptyMessage); handled {
			return s, cmd
		}

		switch msg.String() {
		case "c":
			if entry, ok := s.selectedEntry(); ok {
				return s, copyTextCmd(entry.JID())
			}
			return s, nil
		case "enter":
			// Show detail for selected job
			if idx := s.lazy.Table().Cursor(); idx >= 0 && idx < len(s.jobs) {
				return s, func() tea.Msg {
					return ShowJobDetailMsg{Job: s.jobs[idx].JobRecord}
				}
			}
			return s, nil
		}

		if s.dangerousActionsEnabled {
			switch msg.String() {
			case "D":
				if entry, ok := s.selectedEntry(); ok {
					s.pendingConfirm.SetForEntry(scheduledJobActionDelete, entry)
					return s, s.openDeleteConfirm(entry)
				}
				return s, nil
			case "R":
				if entry, ok := s.selectedEntry(); ok {
					s.pendingConfirm.SetForEntry(scheduledJobActionAddToQueue, entry)
					return s, s.openAddToQueueConfirm(entry)
				}
				return s, nil
			case "ctrl+d":
				s.pendingConfirm.Set(scheduledJobActionDeleteAll, nil, "scheduled.delete_all")
				return s, s.openDeleteAllConfirm()
			case "ctrl+r":
				s.pendingConfirm.Set(scheduledJobActionAddAllToQueue, nil, "scheduled.add_all")
				return s, s.openAddAllToQueueConfirm()
			}
		}

		return s, s.updateKeyPress(msg)
	}

	return s, nil
}

// View implements View.
func (s *Scheduled) View() string {
	if !s.ready {
		return s.renderLoadingMessage()
	}

	return s.renderJobsBox()
}

// Name implements View.
func (s *Scheduled) Name() string {
	return "Scheduled"
}

// ShortHelp implements View.
func (s *Scheduled) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (s *Scheduled) ContextItems() []ContextItem {
	now := time.Now()
	nextScheduled := "-"
	latestScheduled := "-"
	if s.firstEntry != nil {
		nextScheduled = display.Duration(int64(s.firstEntry.At().Sub(now).Seconds()))
	}
	if s.lastEntry != nil {
		latestScheduled = display.Duration(int64(s.lastEntry.At().Sub(now).Seconds()))
	}

	items := []ContextItem{
		{Label: "Next scheduled in", Value: nextScheduled},
		{Label: "Latest scheduled in", Value: latestScheduled},
		{Label: "Total items", Value: display.Number(s.lazy.Total())},
	}
	return items
}

// HintBindings implements HintProvider.
func (s *Scheduled) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"[", "]"}, "[ ⋰ ]", "page up/down"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// MutationBindings implements MutationHintProvider.
func (s *Scheduled) MutationBindings() []key.Binding {
	if !s.dangerousActionsEnabled {
		return nil
	}
	return []key.Binding{
		helpBinding([]string{"D"}, "shift+d", "delete job"),
		helpBinding([]string{"R"}, "shift+r", "add to queue"),
		helpBinding([]string{"ctrl+d"}, "ctrl+d", "delete all"),
		helpBinding([]string{"ctrl+r"}, "ctrl+r", "add all to queue"),
	}
}

// HelpSections implements HelpProvider.
func (s *Scheduled) HelpSections() []HelpSection {
	sections := []HelpSection{
		{
			Title: "Scheduled",
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
	if s.dangerousActionsEnabled {
		sections = append(sections, HelpSection{
			Title: "Dangerous Actions",
			Bindings: []key.Binding{
				helpBinding([]string{"D"}, "shift+d", "delete job"),
				helpBinding([]string{"R"}, "shift+r", "add to queue"),
				helpBinding([]string{"ctrl+d"}, "ctrl+d", "delete all"),
				helpBinding([]string{"ctrl+r"}, "ctrl+r", "add all to queue"),
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (s *Scheduled) TableHelp() []key.Binding {
	return s.tableHelp()
}

// SetSize implements View.
func (s *Scheduled) SetSize(width, height int) View {
	s.setSize(width, height)
	return s
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (s *Scheduled) SetDangerousActionsEnabled(enabled bool) {
	s.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (s *Scheduled) Dispose() {
	s.dispose(s.reset)
}

// CancelRequests stops in-flight fetches when the view is hidden.
func (s *Scheduled) CancelRequests() {
	s.cancelRequests()
}

// SetStyles implements View.
func (s *Scheduled) SetStyles(styles Styles) View {
	s.setStyles(styles)
	return s
}

func (s *Scheduled) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	return fetchSortedEntriesWindow(ctx, sortedEntriesFetchConfig{
		tracker:          "scheduled.fetchWindow",
		client:           s.client,
		kind:             sidekiq.SortedSetScheduled,
		filter:           s.filter,
		windowStart:      windowStart,
		windowSize:       windowSize,
		fallbackPageSize: scheduledFallbackPageSize,
		windowPages:      scheduledWindowPages,
		buildRows:        s.buildRows,
	})
}

func (s *Scheduled) reset() {
	s.resetSortedJobs(s.updateEmptyMessage)
}

func (s *Scheduled) selectedEntry() (*sidekiq.SortedEntry, bool) {
	return s.selectedSortedEntry()
}

// Table columns for scheduled job list.
var scheduledJobColumns = []table.Column{
	{Title: "When", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 60},
}

func (s *Scheduled) updateEmptyMessage() {
	msg := "No scheduled jobs"
	if s.filter != "" {
		msg = "No matches"
	}
	s.lazy.SetEmptyMessage(msg)
}

func (s *Scheduled) buildRows(jobs []*sidekiq.SortedEntry) []table.Row {
	rows := make([]table.Row, 0, len(jobs))
	now := time.Now()
	for _, job := range jobs {
		when := display.Duration(int64(job.At().Sub(now).Seconds()))
		rows = append(rows, table.Row{
			ID: job.JID(),
			Cells: []string{
				when,
				s.styles.QueueText.Render(job.Queue()),
				job.DisplayClass(),
				display.Args(job.DisplayArgs()),
			},
		})
	}
	return rows
}

func (s *Scheduled) openDeleteConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := s.jobName(entry)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				s.styles,
				"Delete job",
				fmt.Sprintf(
					"Are you sure you want to delete the %s job?\n\nThis action is not recoverable.",
					s.styles.Text.Bold(true).Render(jobName),
				),
				entry.JID(),
				s.styles.DangerAction,
			),
		}
	}
}

func (s *Scheduled) openAddToQueueConfirm(entry *sidekiq.SortedEntry) tea.Cmd {
	jobName := s.jobName(entry)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				s.styles,
				"Add to queue",
				fmt.Sprintf(
					"Add the %s job to the queue now?\n\nThis will enqueue it immediately.",
					s.styles.Text.Bold(true).Render(jobName),
				),
				entry.JID(),
				s.styles.DangerAction,
			),
		}
	}
}

func (s *Scheduled) openDeleteAllConfirm() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				s.styles,
				"Delete all scheduled",
				"Are you sure you want to delete all scheduled jobs?\n\nThis action is not recoverable.",
				"scheduled.delete_all",
				s.styles.DangerAction,
			),
		}
	}
}

func (s *Scheduled) openAddAllToQueueConfirm() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: newConfirmDialog(
				s.styles,
				"Add all to queue",
				"Add all scheduled jobs to the queue now?\n\nThis will enqueue them immediately.",
				"scheduled.add_all",
				s.styles.DangerAction,
			),
		}
	}
}

func (s *Scheduled) deleteJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "scheduled.deleteJobCmd")
		if err := s.client.DeleteSortedEntry(ctx, sidekiq.SortedSetScheduled, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (s *Scheduled) deleteAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "scheduled.deleteAllCmd")
		if err := s.client.DeleteAllSortedEntries(ctx, sidekiq.SortedSetScheduled); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (s *Scheduled) addToQueueJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "scheduled.addToQueueJobCmd")
		if err := s.client.EnqueueSortedEntry(ctx, sidekiq.SortedSetScheduled, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (s *Scheduled) addAllToQueueCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "scheduled.addAllToQueueCmd")
		if err := s.client.EnqueueAllSortedEntries(ctx, sidekiq.SortedSetScheduled); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
func (s *Scheduled) renderJobsBox() string {
	return s.renderSortedJobsBox("Scheduled")
}

// renderJobDetail renders the job detail view.
