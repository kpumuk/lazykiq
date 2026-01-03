package views

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/filterinput"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

const scheduledPageSize = 25

// scheduledDataMsg carries scheduled jobs data internally.
type scheduledDataMsg struct {
	jobs        []*sidekiq.SortedEntry
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
	table       table.Model
	ready       bool
	currentPage int
	totalPages  int
	totalSize   int64
	filter      filterinput.Model
}

// NewScheduled creates a new Scheduled view.
func NewScheduled(client sidekiq.API) *Scheduled {
	return &Scheduled{
		client:      client,
		currentPage: 1,
		totalPages:  1,
		filter:      filterinput.New(),
		table: table.New(
			table.WithColumns(scheduledJobColumns),
			table.WithEmptyMessage("No scheduled jobs"),
		),
	}
}

// Init implements View.
func (s *Scheduled) Init() tea.Cmd {
	s.reset()
	s.filter.Init()
	return s.fetchDataCmd()
}

// Update implements View.
func (s *Scheduled) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case scheduledDataMsg:
		s.jobs = msg.jobs
		s.currentPage = msg.currentPage
		s.totalPages = msg.totalPages
		s.totalSize = msg.totalSize
		s.ready = true
		s.updateTableRows()
		return s, nil

	case RefreshMsg:
		return s, s.fetchDataCmd()

	case filterinput.ActionMsg:
		if msg.Action != filterinput.ActionNone {
			s.currentPage = 1
			s.table.SetCursor(0)
			return s, s.fetchDataCmd()
		}
		return s, nil

	case tea.KeyMsg:
		wasFocused := s.filter.Focused()
		var cmd tea.Cmd
		s.filter, cmd = s.filter.Update(msg)
		if wasFocused || msg.String() == "/" || msg.String() == "esc" || msg.String() == "ctrl+u" {
			return s, cmd
		}

		switch msg.String() {
		case "alt+left", "[":
			if s.filter.Query() != "" {
				return s, nil
			}
			if s.currentPage > 1 {
				s.currentPage--
				return s, s.fetchDataCmd()
			}
			return s, nil
		case "alt+right", "]":
			if s.filter.Query() != "" {
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

	if len(s.jobs) == 0 && s.totalSize == 0 && s.filter.Query() == "" && !s.filter.Focused() {
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
	s.filter = filterinput.New()
	s.SetStyles(s.styles)
	s.updateTableSize()
}

// FilterFocused reports whether the filter input is capturing keys.
func (s *Scheduled) FilterFocused() bool {
	return s.filter.Focused()
}

// SetStyles implements View.
func (s *Scheduled) SetStyles(styles Styles) View {
	s.styles = styles
	s.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	s.filter.SetStyles(filterinput.Styles{
		Prompt:      styles.MetricLabel,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	})
	return s
}

// fetchDataCmd fetches scheduled jobs data from Redis.
func (s *Scheduled) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		if s.filter.Query() != "" {
			jobs, err := s.client.ScanScheduledJobs(ctx, s.filter.Query())
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}

			return scheduledDataMsg{
				jobs:        jobs,
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
	tableHeight := max(s.height-3, 3)
	// Table width: view width - box borders - padding
	tableWidth := s.width - 4
	s.table.SetSize(tableWidth, tableHeight)
	s.filter.SetWidth(tableWidth)
}

// updateTableRows converts job data to table rows.
func (s *Scheduled) updateTableRows() {
	if s.filter.Query() != "" {
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

// renderJobsBox renders the bordered box containing the jobs table.
func (s *Scheduled) renderJobsBox() string {
	// Build meta: SIZE and PAGE info
	sep := s.styles.Muted.Render(" • ")
	sizeInfo := s.styles.MetricLabel.Render("SIZE: ") + s.styles.MetricValue.Render(format.ShortNumber(s.totalSize))
	pageInfo := s.styles.MetricLabel.Render("PAGE: ") + s.styles.MetricValue.Render(fmt.Sprintf("%d/%d", s.currentPage, s.totalPages))
	meta := sizeInfo + sep + pageInfo

	// Get table content
	content := s.filter.View() + "\n" + s.table.View()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  s.styles.Title,
				Border: s.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  s.styles.Title,
				Border: s.styles.BorderStyle,
			},
		}),
		frame.WithTitle("Scheduled"),
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
