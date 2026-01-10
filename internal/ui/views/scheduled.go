package views

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

const (
	scheduledWindowPages      = 3
	scheduledFallbackPageSize = 25
)

type scheduledPayload struct {
	jobs       []*sidekiq.SortedEntry
	firstEntry *sidekiq.SortedEntry
	lastEntry  *sidekiq.SortedEntry
}

type scheduledJobAction int

const (
	scheduledJobActionNone scheduledJobAction = iota
	scheduledJobActionDelete
	scheduledJobActionAddToQueue
)

// Scheduled shows jobs scheduled for future execution.
type Scheduled struct {
	client                  sidekiq.API
	width                   int
	height                  int
	styles                  Styles
	jobs                    []*sidekiq.SortedEntry
	firstEntry              *sidekiq.SortedEntry
	lastEntry               *sidekiq.SortedEntry
	lazy                    lazytable.Model
	ready                   bool
	filter                  string
	dangerousActionsEnabled bool
	frameStyles             frame.Styles
	filterStyle             filterdialog.Styles
	pendingJobAction        scheduledJobAction
	pendingJobEntry         *sidekiq.SortedEntry
	pendingJobTarget        string
}

// NewScheduled creates a new Scheduled view.
func NewScheduled(client sidekiq.API) *Scheduled {
	s := &Scheduled{
		client: client,
		lazy: lazytable.New(
			lazytable.WithTableOptions(
				table.WithColumns(scheduledJobColumns),
				table.WithEmptyMessage("No scheduled jobs"),
			),
			lazytable.WithWindowPages(scheduledWindowPages),
			lazytable.WithFallbackPageSize(scheduledFallbackPageSize),
		),
	}
	s.lazy.SetFetcher(s.fetchWindow)
	s.lazy.SetErrorHandler(func(err error) tea.Msg {
		return ConnectionErrorMsg{Err: err}
	})
	return s
}

// Init implements View.
func (s *Scheduled) Init() tea.Cmd {
	s.reset()
	return s.lazy.RequestWindow(0, lazytable.CursorStart)
}

// Update implements View.
func (s *Scheduled) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case lazytable.DataMsg:
		if msg.RequestID != s.lazy.RequestID() {
			return s, nil
		}
		if payload, ok := msg.Result.Payload.(scheduledPayload); ok {
			s.jobs = payload.jobs
			s.firstEntry = payload.firstEntry
			s.lastEntry = payload.lastEntry
		}
		s.ready = true
		var cmd tea.Cmd
		s.lazy, cmd = s.lazy.Update(msg)
		return s, cmd

	case RefreshMsg:
		if s.lazy.Loading() {
			return s, nil
		}
		return s, s.lazy.RequestWindow(s.lazy.WindowStart(), lazytable.CursorKeep)

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone || msg.Query == s.filter {
			return s, nil
		}
		s.filter = msg.Query
		s.updateEmptyMessage()
		s.lazy.Table().SetCursor(0)
		return s, s.lazy.RequestWindow(0, lazytable.CursorStart)

	case confirmdialog.ActionMsg:
		if !s.dangerousActionsEnabled || s.pendingJobEntry == nil || (s.pendingJobTarget != "" && msg.Target != s.pendingJobTarget) {
			return s, nil
		}
		action := s.pendingJobAction
		entry := s.pendingJobEntry
		s.pendingJobAction = scheduledJobActionNone
		s.pendingJobEntry = nil
		s.pendingJobTarget = ""
		if !msg.Confirmed {
			return s, nil
		}
		switch action {
		case scheduledJobActionNone:
			return s, nil
		case scheduledJobActionDelete:
			return s, s.deleteJobCmd(entry)
		case scheduledJobActionAddToQueue:
			return s, s.addToQueueJobCmd(entry)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return s, s.openFilterDialog()
		case "ctrl+u":
			if s.filter == "" {
				return s, nil
			}
			s.filter = ""
			s.updateEmptyMessage()
			s.lazy.Table().SetCursor(0)
			return s, s.lazy.RequestWindow(0, lazytable.CursorStart)
		case "c":
			if entry, ok := s.selectedEntry(); ok {
				return s, copyTextCmd(entry.JID())
			}
			return s, nil
		}

		switch msg.String() {
		case "alt+left", "[":
			if s.filter == "" {
				s.lazy.MovePage(-1)
				return s, s.lazy.MaybePrefetch()
			}
			return s, nil
		case "alt+right", "]":
			if s.filter == "" {
				s.lazy.MovePage(1)
				return s, s.lazy.MaybePrefetch()
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
					s.pendingJobAction = scheduledJobActionDelete
					s.pendingJobEntry = entry
					s.pendingJobTarget = entry.JID()
					return s, s.openDeleteConfirm(entry)
				}
				return s, nil
			case "R":
				if entry, ok := s.selectedEntry(); ok {
					s.pendingJobAction = scheduledJobActionAddToQueue
					s.pendingJobEntry = entry
					s.pendingJobTarget = entry.JID()
					return s, s.openAddToQueueConfirm(entry)
				}
				return s, nil
			}
		}

		var cmd tea.Cmd
		s.lazy, cmd = s.lazy.Update(msg)
		return s, cmd
	}

	return s, nil
}

// View implements View.
func (s *Scheduled) View() string {
	if !s.ready {
		return s.renderMessage("Loading...")
	}

	if len(s.jobs) == 0 && s.lazy.Total() == 0 && s.filter == "" {
		return s.renderMessage("No scheduled jobs")
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
		nextScheduled = format.Duration(int64(s.firstEntry.At().Sub(now).Seconds()))
	}
	if s.lastEntry != nil {
		latestScheduled = format.Duration(int64(s.lastEntry.At().Sub(now).Seconds()))
	}

	items := []ContextItem{
		{Label: "Next scheduled in", Value: nextScheduled},
		{Label: "Latest scheduled in", Value: latestScheduled},
		{Label: "Total items", Value: format.Number(s.lazy.Total())},
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
			},
		})
	}
	return sections
}

// TableHelp implements TableHelpProvider.
func (s *Scheduled) TableHelp() []key.Binding {
	return tableHelpBindings(s.lazy.Table().KeyMap)
}

// SetSize implements View.
func (s *Scheduled) SetSize(width, height int) View {
	s.width = width
	s.height = height
	s.updateTableSize()
	return s
}

// SetDangerousActionsEnabled toggles mutational actions for the view.
func (s *Scheduled) SetDangerousActionsEnabled(enabled bool) {
	s.dangerousActionsEnabled = enabled
}

// Dispose clears cached data when the view is removed from the stack.
func (s *Scheduled) Dispose() {
	s.reset()
	s.filter = ""
	s.SetStyles(s.styles)
	s.updateTableSize()
}

// SetStyles implements View.
func (s *Scheduled) SetStyles(styles Styles) View {
	s.styles = styles
	s.frameStyles = frame.Styles{
		Focused: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterFocused,
			Border: styles.FocusBorder,
		},
		Blurred: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterBlurred,
			Border: styles.BorderStyle,
		},
	}
	s.filterStyle = filterdialog.Styles{
		Title:       styles.Title,
		Border:      styles.FocusBorder,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	}
	s.lazy.SetSpinnerStyle(styles.Muted)
	s.lazy.SetTableStyles(table.Styles{
		Text:           styles.Text,
		Muted:          styles.Muted,
		Header:         styles.TableHeader,
		Selected:       styles.TableSelected,
		Separator:      styles.TableSeparator,
		ScrollbarTrack: styles.ScrollbarTrack,
		ScrollbarThumb: styles.ScrollbarThumb,
	})
	return s
}

func (s *Scheduled) fetchWindow(
	ctx context.Context,
	windowStart int,
	windowSize int,
	_ lazytable.CursorIntent,
) (lazytable.FetchResult, error) {
	ctx = devtools.WithTracker(ctx, "scheduled.fetchWindow")

	if s.filter != "" {
		jobs, err := s.client.ScanScheduledJobs(ctx, s.filter)
		if err != nil {
			return lazytable.FetchResult{}, err
		}
		firstEntry, lastEntry := sortedEntryBounds(jobs)
		return lazytable.FetchResult{
			Rows:        s.buildRows(jobs),
			Total:       int64(len(jobs)),
			WindowStart: 0,
			Payload: scheduledPayload{
				jobs:       jobs,
				firstEntry: firstEntry,
				lastEntry:  lastEntry,
			},
		}, nil
	}

	if windowSize <= 0 {
		windowSize = max(scheduledFallbackPageSize, 1) * scheduledWindowPages
	}

	jobs, totalSize, err := s.client.GetScheduledJobs(ctx, windowStart, windowSize)
	if err != nil {
		return lazytable.FetchResult{}, err
	}

	if totalSize > 0 {
		maxStart := max(int(totalSize)-windowSize, 0)
		if windowStart > maxStart {
			windowStart = maxStart
			jobs, totalSize, err = s.client.GetScheduledJobs(ctx, windowStart, windowSize)
			if err != nil {
				return lazytable.FetchResult{}, err
			}
		}
	} else {
		windowStart = 0
	}

	var firstEntry *sidekiq.SortedEntry
	var lastEntry *sidekiq.SortedEntry
	if totalSize > 0 {
		firstEntry, lastEntry, err = s.client.GetScheduledBounds(ctx)
		if err != nil {
			return lazytable.FetchResult{}, err
		}
	}

	return lazytable.FetchResult{
		Rows:        s.buildRows(jobs),
		Total:       totalSize,
		WindowStart: windowStart,
		Payload: scheduledPayload{
			jobs:       jobs,
			firstEntry: firstEntry,
			lastEntry:  lastEntry,
		},
	}, nil
}

func (s *Scheduled) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  s.styles.Title,
		Muted:  s.styles.Muted,
		Border: s.styles.FocusBorder,
	}, "Scheduled", msg, s.width, s.height)
}

func (s *Scheduled) reset() {
	s.jobs = nil
	s.firstEntry = nil
	s.lastEntry = nil
	s.ready = false
	s.lazy.Reset()
	s.updateEmptyMessage()
}

func (s *Scheduled) selectedEntry() (*sidekiq.SortedEntry, bool) {
	idx := s.lazy.Table().Cursor()
	if idx < 0 || idx >= len(s.jobs) {
		return nil, false
	}
	return s.jobs[idx], true
}

// Table columns for scheduled job list.
var scheduledJobColumns = []table.Column{
	{Title: "When", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (s *Scheduled) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(s.height-2, 3)
	// Table width: view width - box borders - padding
	tableWidth := s.width - 4
	s.lazy.SetSize(tableWidth, tableHeight)
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
		when := format.Duration(int64(job.At().Sub(now).Seconds()))
		rows = append(rows, table.Row{
			ID: job.JID(),
			Cells: []string{
				when,
				s.styles.QueueText.Render(job.Queue()),
				job.DisplayClass(),
				format.Args(job.DisplayArgs()),
			},
		})
	}
	return rows
}

func (s *Scheduled) rowsMeta() string {
	start, end, total := s.lazy.Range()
	label := s.styles.MetricLabel.Render("rows: ")
	if total == 0 || len(s.jobs) == 0 {
		return label + s.styles.MetricValue.Render("0/0")
	}

	rangeLabel := fmt.Sprintf(
		"%s-%s/%s",
		format.Number(int64(start)),
		format.Number(int64(end)),
		format.Number(total),
	)
	return label + s.styles.MetricValue.Render(rangeLabel)
}

func (s *Scheduled) openFilterDialog() tea.Cmd {
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: filterdialog.New(
				filterdialog.WithStyles(s.filterStyle),
				filterdialog.WithQuery(s.filter),
			),
		}
	}
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

func (s *Scheduled) deleteJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "scheduled.deleteJobCmd")
		if err := s.client.DeleteScheduledJob(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

func (s *Scheduled) addToQueueJobCmd(entry *sidekiq.SortedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := devtools.WithTracker(context.Background(), "scheduled.addToQueueJobCmd")
		if err := s.client.AddScheduledJobToQueue(ctx, entry); err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return RefreshMsg{}
	}
}

// renderJobsBox renders the bordered box containing the jobs table.
func (s *Scheduled) renderJobsBox() string {
	return frame.New(
		frame.WithStyles(s.frameStyles),
		frame.WithTitle("Scheduled"),
		frame.WithFilter(s.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(s.rowsMeta()),
		frame.WithContent(s.lazy.View()),
		frame.WithPadding(1),
		frame.WithSize(s.width, s.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	).View()
}

func (s *Scheduled) jobName(entry *sidekiq.SortedEntry) string {
	if name := entry.DisplayClass(); name != "" {
		return name
	}
	return "selected"
}

// renderJobDetail renders the job detail view.
