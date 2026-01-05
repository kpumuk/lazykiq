package views

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

const scheduledPageSize = 25

// scheduledDataMsg carries scheduled jobs data internally.
type scheduledDataMsg struct {
	jobs        []*sidekiq.SortedEntry
	firstEntry  *sidekiq.SortedEntry
	lastEntry   *sidekiq.SortedEntry
	currentPage int
	totalPages  int
	totalSize   int64
}

// Scheduled shows jobs scheduled for future execution.
type Scheduled struct {
	client      sidekiq.API
	width       int
	height      int
	styles      Styles
	jobs        []*sidekiq.SortedEntry
	firstEntry  *sidekiq.SortedEntry
	lastEntry   *sidekiq.SortedEntry
	table       table.Model
	ready       bool
	currentPage int
	totalPages  int
	totalSize   int64
	filter      string
	frameStyles frame.Styles
	filterStyle filterdialog.Styles
}

// NewScheduled creates a new Scheduled view.
func NewScheduled(client sidekiq.API) *Scheduled {
	return &Scheduled{
		client:      client,
		currentPage: 1,
		totalPages:  1,
		table: table.New(
			table.WithColumns(scheduledJobColumns),
			table.WithEmptyMessage("No scheduled jobs"),
		),
	}
}

// Init implements View.
func (s *Scheduled) Init() tea.Cmd {
	s.reset()
	return s.fetchDataCmd()
}

// Update implements View.
func (s *Scheduled) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case scheduledDataMsg:
		s.jobs = msg.jobs
		s.firstEntry = msg.firstEntry
		s.lastEntry = msg.lastEntry
		s.currentPage = msg.currentPage
		s.totalPages = msg.totalPages
		s.totalSize = msg.totalSize
		s.ready = true
		s.updateTableRows()
		return s, nil

	case RefreshMsg:
		return s, s.fetchDataCmd()

	case filterdialog.ActionMsg:
		if msg.Action == filterdialog.ActionNone {
			return s, nil
		}
		if msg.Query == s.filter {
			return s, nil
		}
		s.filter = msg.Query
		s.currentPage = 1
		s.table.SetCursor(0)
		return s, s.fetchDataCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "/":
			return s, s.openFilterDialog()
		case "ctrl+u":
			if s.filter != "" {
				s.filter = ""
				s.currentPage = 1
				s.table.SetCursor(0)
				return s, s.fetchDataCmd()
			}
			return s, nil
		}

		switch msg.String() {
		case "alt+left", "[":
			if s.filter != "" {
				return s, nil
			}
			if s.currentPage > 1 {
				s.currentPage--
				return s, s.fetchDataCmd()
			}
			return s, nil
		case "alt+right", "]":
			if s.filter != "" {
				return s, nil
			}
			if s.currentPage < s.totalPages {
				s.currentPage++
				return s, s.fetchDataCmd()
			}
			return s, nil
		case "enter":
			// Show detail for selected job
			if idx := s.table.Cursor(); idx >= 0 && idx < len(s.jobs) {
				return s, func() tea.Msg {
					return ShowJobDetailMsg{Job: s.jobs[idx].JobRecord}
				}
			}
			return s, nil
		}

		s.table, _ = s.table.Update(msg)
		return s, nil
	}

	return s, nil
}

// View implements View.
func (s *Scheduled) View() string {
	if !s.ready {
		return s.renderMessage("Loading...")
	}

	if len(s.jobs) == 0 && s.totalSize == 0 && s.filter == "" {
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
	now := time.Now().Unix()
	nextScheduled := "-"
	latestScheduled := "-"
	if s.firstEntry != nil {
		nextScheduled = format.Duration(s.firstEntry.At() - now)
	}
	if s.lastEntry != nil {
		latestScheduled = format.Duration(s.lastEntry.At() - now)
	}

	items := []ContextItem{
		{Label: "Next scheduled in", Value: nextScheduled},
		{Label: "Latest scheduled in", Value: latestScheduled},
		{Label: "Total items", Value: format.Number(s.totalSize)},
	}
	return items
}

// HintBindings implements HintProvider.
func (s *Scheduled) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"/"}, "/", "filter"),
		helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"),
		helpBinding([]string{"["}, "[", "prev page"),
		helpBinding([]string{"]"}, "]", "next page"),
		helpBinding([]string{"enter"}, "enter", "job detail"),
	}
}

// HelpSections implements HelpProvider.
func (s *Scheduled) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Scheduled",
			Bindings: []key.Binding{
				helpBinding([]string{"/"}, "/", "filter"),
				helpBinding([]string{"ctrl+u"}, "ctrl+u", "clear filter"),
				helpBinding([]string{"["}, "[", "previous page"),
				helpBinding([]string{"]"}, "]", "next page"),
				helpBinding([]string{"enter"}, "enter", "job detail"),
			},
		},
	}
}

// TableHelp implements TableHelpProvider.
func (s *Scheduled) TableHelp() []key.Binding {
	return tableHelpBindings(s.table.KeyMap)
}

// SetSize implements View.
func (s *Scheduled) SetSize(width, height int) View {
	s.width = width
	s.height = height
	s.updateTableSize()
	return s
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
	s.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	return s
}

// fetchDataCmd fetches scheduled jobs data from Redis.
func (s *Scheduled) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		if s.filter != "" {
			jobs, err := s.client.ScanScheduledJobs(ctx, s.filter)
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}
			firstEntry, lastEntry := sortedEntryBounds(jobs)

			return scheduledDataMsg{
				jobs:        jobs,
				firstEntry:  firstEntry,
				lastEntry:   lastEntry,
				currentPage: 1,
				totalPages:  1,
				totalSize:   int64(len(jobs)),
			}
		}

		currentPage := s.currentPage
		totalPages := 1

		start := (currentPage - 1) * scheduledPageSize
		jobs, totalSize, err := s.client.GetScheduledJobs(ctx, start, scheduledPageSize)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		var firstEntry *sidekiq.SortedEntry
		var lastEntry *sidekiq.SortedEntry
		if totalSize > 0 {
			firstEntry, lastEntry, err = s.client.GetScheduledBounds(ctx)
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}
		}

		if totalSize > 0 {
			totalPages = int((totalSize + scheduledPageSize - 1) / scheduledPageSize)
		}

		if currentPage > totalPages {
			currentPage = totalPages
		}
		if currentPage < 1 {
			currentPage = 1
		}

		return scheduledDataMsg{
			jobs:        jobs,
			firstEntry:  firstEntry,
			lastEntry:   lastEntry,
			currentPage: currentPage,
			totalPages:  totalPages,
			totalSize:   totalSize,
		}
	}
}

func (s *Scheduled) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  s.styles.Title,
		Muted:  s.styles.Muted,
		Border: s.styles.FocusBorder,
	}, "Scheduled", msg, s.width, s.height)
}

func (s *Scheduled) reset() {
	s.currentPage = 1
	s.totalPages = 1
	s.totalSize = 0
	s.jobs = nil
	s.firstEntry = nil
	s.lastEntry = nil
	s.ready = false
	s.table.SetRows(nil)
	s.table.SetCursor(0)
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
	s.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows.
func (s *Scheduled) updateTableRows() {
	if s.filter != "" {
		s.table.SetEmptyMessage("No matches")
	} else {
		s.table.SetEmptyMessage("No scheduled jobs")
	}

	rows := make([]table.Row, 0, len(s.jobs))
	now := time.Now().Unix()
	for _, job := range s.jobs {
		// Format "when" as time until job runs (job.At() is in the future)
		when := format.Duration(job.At() - now)

		row := table.Row{
			ID: job.JID(),
			Cells: []string{
				when,
				job.Queue(),
				job.DisplayClass(),
				format.Args(job.DisplayArgs()),
			},
		}
		rows = append(rows, row)
	}
	s.table.SetRows(rows)
	s.updateTableSize()
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

// renderJobsBox renders the bordered box containing the jobs table.
func (s *Scheduled) renderJobsBox() string {
	// Build meta: page only
	pageInfo := s.styles.MetricLabel.Render("page: ") + s.styles.MetricValue.Render(fmt.Sprintf("%d/%d", s.currentPage, s.totalPages))
	meta := pageInfo

	// Get table content
	content := s.table.View()

	box := frame.New(
		frame.WithStyles(s.frameStyles),
		frame.WithTitle("Scheduled"),
		frame.WithFilter(s.filter),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(s.width, s.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

// renderJobDetail renders the job detail view.
