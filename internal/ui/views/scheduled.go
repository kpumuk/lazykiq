package views

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/jobsbox"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// ScheduledUpdateMsg carries scheduled jobs data from App to Scheduled view
type ScheduledUpdateMsg struct {
	Jobs        []*sidekiq.SortedEntry
	CurrentPage int
	TotalPages  int
	TotalSize   int64
}

// ScheduledPageRequestMsg requests a specific page of scheduled jobs
type ScheduledPageRequestMsg struct {
	Page int
}

// Scheduled shows jobs scheduled for future execution
type Scheduled struct {
	width       int
	height      int
	styles      Styles
	jobs        []*sidekiq.SortedEntry
	table       *table.Table
	ready       bool
	currentPage int
	totalPages  int
	totalSize   int64
}

// NewScheduled creates a new Scheduled view
func NewScheduled() *Scheduled {
	return &Scheduled{}
}

// Init implements View
func (s *Scheduled) Init() tea.Cmd {
	return nil
}

// Update implements View
func (s *Scheduled) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case ScheduledUpdateMsg:
		s.jobs = msg.Jobs
		s.currentPage = msg.CurrentPage
		s.totalPages = msg.TotalPages
		s.totalSize = msg.TotalSize
		s.ready = true
		s.updateTableRows()
		return s, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "alt+left", "[":
			if s.currentPage > 1 {
				return s, func() tea.Msg {
					return ScheduledPageRequestMsg{Page: s.currentPage - 1}
				}
			}
			return s, nil
		case "alt+right", "]":
			if s.currentPage < s.totalPages {
				return s, func() tea.Msg {
					return ScheduledPageRequestMsg{Page: s.currentPage + 1}
				}
			}
			return s, nil
		}

		// Pass other keys to table for navigation
		if s.table != nil {
			s.table.Update(msg)
		}
		return s, nil
	}

	return s, nil
}

// View implements View
func (s *Scheduled) View() string {
	if !s.ready {
		return s.renderMessage("Loading...")
	}

	if len(s.jobs) == 0 && s.totalSize == 0 {
		return s.renderMessage("No scheduled jobs")
	}

	return s.renderJobsBox()
}

func (s *Scheduled) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  s.styles.Title,
		Muted:  s.styles.Muted,
		Border: s.styles.BorderStyle,
	}, "Scheduled", msg, s.width, s.height)
}

// Name implements View
func (s *Scheduled) Name() string {
	return "Scheduled"
}

// ShortHelp implements View
func (s *Scheduled) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View
func (s *Scheduled) SetSize(width, height int) View {
	s.width = width
	s.height = height
	s.updateTableSize()
	return s
}

// SetStyles implements View
func (s *Scheduled) SetStyles(styles Styles) View {
	s.styles = styles
	if s.table != nil {
		s.table.SetStyles(table.Styles{
			Text:      styles.Text,
			Muted:     styles.Muted,
			Header:    styles.TableHeader,
			Selected:  styles.TableSelected,
			Separator: styles.TableSeparator,
		})
	}
	return s
}

// Table columns for scheduled job list
var scheduledJobColumns = []table.Column{
	{Title: "When", Width: 12},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size
func (s *Scheduled) updateTableSize() {
	if s.table == nil {
		return
	}
	// Calculate table height: total height - box borders
	tableHeight := s.height - 2
	if tableHeight < 3 {
		tableHeight = 3
	}
	// Table width: view width - box borders - padding
	tableWidth := s.width - 4
	s.table.SetSize(tableWidth, tableHeight)
}

// updateTableRows converts job data to table rows
func (s *Scheduled) updateTableRows() {
	s.ensureTable()

	rows := make([][]string, 0, len(s.jobs))
	now := time.Now().Unix()
	for _, job := range s.jobs {
		// Format "when" as time until job runs (job.At() is in the future)
		when := format.Duration(job.At() - now)

		row := []string{
			when,
			job.Queue(),
			job.DisplayClass(),
			format.Args(job.Args()),
		}
		rows = append(rows, row)
	}
	s.table.SetRows(rows)
	s.updateTableSize()
}

// ensureTable creates the table if it doesn't exist
func (s *Scheduled) ensureTable() {
	if s.table != nil {
		return
	}
	s.table = table.New(scheduledJobColumns)
	s.table.SetEmptyMessage("No scheduled jobs")
	s.table.SetStyles(table.Styles{
		Text:      s.styles.Text,
		Muted:     s.styles.Muted,
		Header:    s.styles.TableHeader,
		Selected:  s.styles.TableSelected,
		Separator: s.styles.TableSeparator,
	})
}

// renderJobsBox renders the bordered box containing the jobs table
func (s *Scheduled) renderJobsBox() string {
	// Build meta: SIZE and PAGE info
	sep := s.styles.Muted.Render(" • ")
	sizeInfo := s.styles.MetricLabel.Render("SIZE: ") + s.styles.MetricValue.Render(format.Number(s.totalSize))
	pageInfo := s.styles.MetricLabel.Render("PAGE: ") + s.styles.MetricValue.Render(fmt.Sprintf("%d/%d", s.currentPage, s.totalPages))
	meta := sizeInfo + sep + pageInfo

	// Get table content
	content := ""
	if s.table != nil {
		content = s.table.View()
	}

	return jobsbox.Render(jobsbox.Styles{
		Title:  s.styles.Title,
		Border: s.styles.BorderStyle,
	}, "Scheduled", meta, content, s.width, s.height)
}

