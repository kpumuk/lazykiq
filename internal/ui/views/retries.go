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

// retriesDataMsg is internal to the Retries view.
type retriesDataMsg struct {
	jobs        []*sidekiq.SortedEntry
	currentPage int
	totalPages  int
	totalSize   int64
}

const retriesPageSize = 25

// Retries shows failed jobs pending retry.
type Retries struct {
	client      *sidekiq.Client
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

// NewRetries creates a new Retries view.
func NewRetries(client *sidekiq.Client) *Retries {
	return &Retries{
		client:      client,
		currentPage: 1,
		totalPages:  1,
		filter:      filterinput.New(),
		table: table.New(
			table.WithColumns(retryJobColumns),
			table.WithEmptyMessage("No retries"),
		),
	}
}

// fetchDataCmd fetches retry jobs data from Redis.
func (r *Retries) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		if r.filter.Query() != "" {
			jobs, err := r.client.ScanRetryJobs(ctx, r.filter.Query())
			if err != nil {
				return ConnectionErrorMsg{Err: err}
			}

			return retriesDataMsg{
				jobs:        jobs,
				currentPage: 1,
				totalPages:  1,
				totalSize:   int64(len(jobs)),
			}
		}

		currentPage := r.currentPage
		totalPages := 1

		start := (currentPage - 1) * retriesPageSize
		jobs, totalSize, err := r.client.GetRetryJobs(ctx, start, retriesPageSize)
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}

		if totalSize > 0 {
			totalPages = int((totalSize + retriesPageSize - 1) / retriesPageSize)
		}

		if currentPage > totalPages {
			currentPage = totalPages
		}
		if currentPage < 1 {
			currentPage = 1
		}

		return retriesDataMsg{
			jobs:        jobs,
			currentPage: currentPage,
			totalPages:  totalPages,
			totalSize:   totalSize,
		}
	}
}

// Init implements View.
func (r *Retries) Init() tea.Cmd {
	r.reset()
	r.filter.Init()
	return r.fetchDataCmd()
}

// Update implements View.
func (r *Retries) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case retriesDataMsg:
		r.jobs = msg.jobs
		r.currentPage = msg.currentPage
		r.totalPages = msg.totalPages
		r.totalSize = msg.totalSize
		r.ready = true
		r.updateTableRows()
		return r, nil

	case RefreshMsg:
		return r, r.fetchDataCmd()

	case filterinput.ActionMsg:
		if msg.Action != filterinput.ActionNone {
			r.currentPage = 1
			r.table.SetCursor(0)
			return r, r.fetchDataCmd()
		}
		return r, nil

	case tea.KeyMsg:
		wasFocused := r.filter.Focused()
		var cmd tea.Cmd
		r.filter, cmd = r.filter.Update(msg)
		if wasFocused || msg.String() == "/" || msg.String() == "esc" || msg.String() == "ctrl+u" {
			return r, cmd
		}

		switch msg.String() {
		case "alt+left", "[":
			if r.filter.Query() != "" {
				return r, nil
			}
			if r.currentPage > 1 {
				r.currentPage--
				return r, r.fetchDataCmd()
			}
			return r, nil
		case "alt+right", "]":
			if r.filter.Query() != "" {
				return r, nil
			}
			if r.currentPage < r.totalPages {
				r.currentPage++
				return r, r.fetchDataCmd()
			}
			return r, nil
		case "enter":
			// Show detail for selected job
			if idx := r.table.Cursor(); idx >= 0 && idx < len(r.jobs) {
				return r, func() tea.Msg {
					return ShowJobDetailMsg{Job: r.jobs[idx].JobRecord}
				}
			}
			return r, nil
		}

		// Pass other keys to table for navigation
		r.table, _ = r.table.Update(msg)
		return r, nil
	}

	return r, nil
}

// View implements View.
func (r *Retries) View() string {
	if !r.ready {
		return r.renderMessage("Loading...")
	}

	if len(r.jobs) == 0 && r.totalSize == 0 && r.filter.Query() == "" && !r.filter.Focused() {
		return r.renderMessage("No retries")
	}

	return r.renderJobsBox()
}

func (r *Retries) renderMessage(msg string) string {
	return messagebox.Render(messagebox.Styles{
		Title:  r.styles.Title,
		Muted:  r.styles.Muted,
		Border: r.styles.FocusBorder,
	}, "Retries", msg, r.width, r.height)
}

// Name implements View.
func (r *Retries) Name() string {
	return "Retries"
}

// ShortHelp implements View.
func (r *Retries) ShortHelp() []key.Binding {
	return nil
}

// SetSize implements View.
func (r *Retries) SetSize(width, height int) View {
	r.width = width
	r.height = height
	r.updateTableSize()
	return r
}

func (r *Retries) reset() {
	r.currentPage = 1
	r.totalPages = 1
	r.totalSize = 0
	r.jobs = nil
	r.ready = false
	r.table.SetRows(nil)
	r.table.SetCursor(0)
}

// Dispose clears cached data when the view is removed from the stack.
func (r *Retries) Dispose() {
	r.reset()
	r.filter = filterinput.New()
	r.SetStyles(r.styles)
	r.updateTableSize()
}

// FilterFocused reports whether the filter input is capturing keys.
func (r *Retries) FilterFocused() bool {
	return r.filter.Focused()
}

// SetStyles implements View.
func (r *Retries) SetStyles(styles Styles) View {
	r.styles = styles
	r.table.SetStyles(table.Styles{
		Text:      styles.Text,
		Muted:     styles.Muted,
		Header:    styles.TableHeader,
		Selected:  styles.TableSelected,
		Separator: styles.TableSeparator,
	})
	r.filter.SetStyles(filterinput.Styles{
		Prompt:      styles.MetricLabel,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	})
	return r
}

// Table columns for retry job list.
var retryJobColumns = []table.Column{
	{Title: "Next Retry", Width: 12},
	{Title: "Retries", Width: 7},
	{Title: "Queue", Width: 15},
	{Title: "Job", Width: 30},
	{Title: "Arguments", Width: 40},
	{Title: "Error", Width: 60},
}

// updateTableSize updates the table dimensions based on current view size.
func (r *Retries) updateTableSize() {
	// Calculate table height: total height - box borders
	tableHeight := max(r.height-3, 3)
	// Table width: view width - box borders - padding
	tableWidth := r.width - 4
	r.table.SetSize(tableWidth, tableHeight)
	r.filter.SetWidth(tableWidth)
}

// updateTableRows converts job data to table rows.
func (r *Retries) updateTableRows() {
	if r.filter.Query() != "" {
		r.table.SetEmptyMessage("No matches")
	} else {
		r.table.SetEmptyMessage("No retries")
	}

	rows := make([]table.Row, 0, len(r.jobs))
	now := time.Now().Unix()
	for _, job := range r.jobs {
		// Format "next retry" as relative time (negative means in the past/due)
		nextRetry := format.Duration(now - job.At())

		// Format retry count
		retryCount := fmt.Sprintf("%d", job.RetryCount())

		// Format error
		errorStr := ""
		if job.HasError() {
			errorStr = fmt.Sprintf("%s: %s", job.ErrorClass(), job.ErrorMessage())
			// Truncate if too long
			if len(errorStr) > 100 {
				errorStr = errorStr[:97] + "..."
			}
		}

		row := table.Row{
			nextRetry,
			retryCount,
			job.Queue(),
			job.DisplayClass(),
			format.Args(job.DisplayArgs()),
			errorStr,
		}
		rows = append(rows, row)
	}
	r.table.SetRows(rows)
	r.updateTableSize()
}

// renderJobsBox renders the bordered box containing the jobs table.
func (r *Retries) renderJobsBox() string {
	// Build meta: SIZE and PAGE info
	sep := r.styles.Muted.Render(" • ")
	sizeInfo := r.styles.MetricLabel.Render("SIZE: ") + r.styles.MetricValue.Render(format.Number(r.totalSize))
	pageInfo := r.styles.MetricLabel.Render("PAGE: ") + r.styles.MetricValue.Render(fmt.Sprintf("%d/%d", r.currentPage, r.totalPages))
	meta := sizeInfo + sep + pageInfo

	// Get table content
	content := r.filter.View() + "\n" + r.table.View()

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  r.styles.Title,
				Border: r.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  r.styles.Title,
				Border: r.styles.BorderStyle,
			},
		}),
		frame.WithTitle("Retries"),
		frame.WithTitlePadding(0),
		frame.WithMeta(meta),
		frame.WithContent(content),
		frame.WithPadding(1),
		frame.WithSize(r.width, r.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

// renderJobDetail renders the job detail view.
