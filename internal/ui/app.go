package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

// tickMsg is sent every 5 seconds to trigger a metrics update
type tickMsg time.Time

// connectionErrorMsg indicates a Redis connection error occurred
type connectionErrorMsg struct {
	err error
}

// App is the main application model
type App struct {
	keys            KeyMap
	width           int
	height          int
	ready           bool
	activeView      int
	views           []views.View
	metrics         components.Metrics
	navbar          components.Navbar
	errorPopup      components.ErrorPopup
	styles          theme.Styles
	darkMode        bool
	sidekiq         *sidekiq.Client
	connectionError error
	queuesPage    int // current page for Queues view (1-indexed)
	selectedQueue int // selected queue index for Queues view (0-indexed)
	retriesPage   int // current page for Retries view (1-indexed)
	scheduledPage int // current page for Scheduled view (1-indexed)
	deadPage      int // current page for Dead view (1-indexed)
}

// New creates a new App instance
func New() App {
	styles := theme.NewStyles(theme.Dark)

	viewList := []views.View{
		views.NewDashboard(),
		views.NewBusy(),
		views.NewQueues(),
		views.NewRetries(),
		views.NewScheduled(),
		views.NewDead(),
	}

	// Apply styles to views
	viewStyles := views.Styles{
		Text:           styles.ViewText,
		Muted:          styles.ViewMuted,
		Title:          styles.ViewTitle,
		Border:         styles.Theme.Border,
		MetricLabel:    styles.MetricLabel,
		MetricValue:    styles.MetricValue,
		TableHeader:    styles.TableHeader,
		TableSelected:  styles.TableSelected,
		TableSeparator: styles.TableSeparator,
		BoxPadding:     styles.BoxPadding,
		BorderStyle:    styles.BorderStyle,
		NavKey:         styles.NavKey,
	}
	for i := range viewList {
		viewList[i] = viewList[i].SetStyles(viewStyles)
	}

	return App{
		keys:        DefaultKeyMap(),
		activeView:  0,
		views:       viewList,
		metrics:     components.NewMetrics(&styles),
		navbar:      components.NewNavbar(viewList, &styles),
		errorPopup:  components.NewErrorPopup(&styles),
		styles:      styles,
		darkMode:    true,
		sidekiq:       sidekiq.NewClient(),
		queuesPage:    1,
		retriesPage:   1,
		scheduledPage: 1,
		deadPage:      1,
	}
}

// Init implements tea.Model
func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.views[a.activeView].Init(),
		a.metrics.Init(),
		func() tea.Msg { return a.fetchStatsCmd() }, // Fetch stats immediately
		tickCmd(), // Start the ticker for subsequent updates
	)
}

// tickCmd returns a command that sends a tick message after 5 seconds
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchStatsCmd fetches Sidekiq stats and returns a MetricsUpdateMsg or connectionErrorMsg
func (a App) fetchStatsCmd() tea.Msg {
	ctx := context.Background()
	stats, err := a.sidekiq.GetStats(ctx)
	if err != nil {
		// Return connection error message
		return connectionErrorMsg{err: err}
	}

	return components.MetricsUpdateMsg{
		Data: components.MetricsData{
			Processed: stats.Processed,
			Failed:    stats.Failed,
			Busy:      stats.Busy,
			Enqueued:  stats.Enqueued,
			Retries:   stats.Retries,
			Scheduled: stats.Scheduled,
			Dead:      stats.Dead,
		},
	}
}

// fetchBusyDataCmd fetches busy data for the Busy view
func (a App) fetchBusyDataCmd() tea.Msg {
	ctx := context.Background()
	data, err := a.sidekiq.GetBusyData(ctx)
	if err != nil {
		return connectionErrorMsg{err: err}
	}
	return views.BusyUpdateMsg{Data: data}
}

const queuesPageSize = 25
const retriesPageSize = 25
const scheduledPageSize = 25
const deadPageSize = 25

// fetchQueuesDataCmd fetches queues data for the Queues view
func (a App) fetchQueuesDataCmd() tea.Msg {
	ctx := context.Background()

	// Get all queues
	queues, err := a.sidekiq.GetQueues(ctx)
	if err != nil {
		return connectionErrorMsg{err: err}
	}

	// Build queue info list with size and latency
	queueInfos := make([]*views.QueueInfo, len(queues))
	for i, q := range queues {
		size, _ := q.Size(ctx)
		latency, _ := q.Latency(ctx)
		queueInfos[i] = &views.QueueInfo{
			Name:    q.Name(),
			Size:    size,
			Latency: latency,
		}
	}

	// Get jobs from the selected queue (if any) with pagination
	var jobs []*sidekiq.JobRecord
	var totalSize int64
	currentPage := a.queuesPage
	totalPages := 1
	selectedQueue := a.selectedQueue

	// Clamp selected queue to valid range
	if selectedQueue >= len(queues) {
		selectedQueue = 0
	}

	if len(queues) > 0 && selectedQueue < len(queues) {
		start := (currentPage - 1) * queuesPageSize
		jobs, totalSize, _ = queues[selectedQueue].GetJobs(ctx, start, queuesPageSize)

		// Calculate total pages
		if totalSize > 0 {
			totalPages = int((totalSize + queuesPageSize - 1) / queuesPageSize)
		}

		// Clamp current page to valid range
		if currentPage > totalPages {
			currentPage = totalPages
		}
		if currentPage < 1 {
			currentPage = 1
		}
	}

	return views.QueuesUpdateMsg{
		Queues:        queueInfos,
		Jobs:          jobs,
		CurrentPage:   currentPage,
		TotalPages:    totalPages,
		SelectedQueue: selectedQueue,
	}
}

// fetchRetriesDataCmd fetches retry jobs data for the Retries view
func (a App) fetchRetriesDataCmd() tea.Msg {
	ctx := context.Background()

	currentPage := a.retriesPage
	totalPages := 1

	// Calculate pagination
	start := (currentPage - 1) * retriesPageSize
	jobs, totalSize, err := a.sidekiq.GetRetryJobs(ctx, start, retriesPageSize)
	if err != nil {
		return connectionErrorMsg{err: err}
	}

	// Calculate total pages
	if totalSize > 0 {
		totalPages = int((totalSize + retriesPageSize - 1) / retriesPageSize)
	}

	// Clamp current page to valid range
	if currentPage > totalPages {
		currentPage = totalPages
	}
	if currentPage < 1 {
		currentPage = 1
	}

	return views.RetriesUpdateMsg{
		Jobs:        jobs,
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		TotalSize:   totalSize,
	}
}

// fetchScheduledDataCmd fetches scheduled jobs data for the Scheduled view
func (a App) fetchScheduledDataCmd() tea.Msg {
	ctx := context.Background()

	currentPage := a.scheduledPage
	totalPages := 1

	// Calculate pagination
	start := (currentPage - 1) * scheduledPageSize
	jobs, totalSize, err := a.sidekiq.GetScheduledJobs(ctx, start, scheduledPageSize)
	if err != nil {
		return connectionErrorMsg{err: err}
	}

	// Calculate total pages
	if totalSize > 0 {
		totalPages = int((totalSize + scheduledPageSize - 1) / scheduledPageSize)
	}

	// Clamp current page to valid range
	if currentPage > totalPages {
		currentPage = totalPages
	}
	if currentPage < 1 {
		currentPage = 1
	}

	return views.ScheduledUpdateMsg{
		Jobs:        jobs,
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		TotalSize:   totalSize,
	}
}

// fetchDeadDataCmd fetches dead jobs data for the Dead view
func (a App) fetchDeadDataCmd() tea.Msg {
	ctx := context.Background()

	currentPage := a.deadPage
	totalPages := 1

	// Calculate pagination
	start := (currentPage - 1) * deadPageSize
	jobs, totalSize, err := a.sidekiq.GetDeadJobs(ctx, start, deadPageSize)
	if err != nil {
		return connectionErrorMsg{err: err}
	}

	// Calculate total pages
	if totalSize > 0 {
		totalPages = int((totalSize + deadPageSize - 1) / deadPageSize)
	}

	// Clamp current page to valid range
	if currentPage > totalPages {
		currentPage = totalPages
	}
	if currentPage < 1 {
		currentPage = 1
	}

	return views.DeadUpdateMsg{
		Jobs:        jobs,
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		TotalSize:   totalSize,
	}
}

// Update implements tea.Model
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		// Always fetch stats for metrics bar
		cmds = append(cmds, func() tea.Msg {
			return a.fetchStatsCmd()
		})

		// Additionally fetch view-specific data
		switch a.activeView {
		case 1: // Busy view
			cmds = append(cmds, func() tea.Msg {
				return a.fetchBusyDataCmd()
			})
		case 2: // Queues view
			cmds = append(cmds, func() tea.Msg {
				return a.fetchQueuesDataCmd()
			})
		case 3: // Retries view
			cmds = append(cmds, func() tea.Msg {
				return a.fetchRetriesDataCmd()
			})
		case 4: // Scheduled view
			cmds = append(cmds, func() tea.Msg {
				return a.fetchScheduledDataCmd()
			})
		case 5: // Dead view
			cmds = append(cmds, func() tea.Msg {
				return a.fetchDeadDataCmd()
			})
		}

		cmds = append(cmds, tickCmd())

	case connectionErrorMsg:
		// Store the connection error
		a.connectionError = msg.err

	case views.QueuesPageRequestMsg:
		// Handle page change request from Queues view
		a.queuesPage = msg.Page
		cmds = append(cmds, func() tea.Msg {
			return a.fetchQueuesDataCmd()
		})

	case views.QueuesQueueSelectMsg:
		// Handle queue selection from Queues view
		a.selectedQueue = msg.Index
		a.queuesPage = 1 // Reset to first page when changing queues
		cmds = append(cmds, func() tea.Msg {
			return a.fetchQueuesDataCmd()
		})

	case views.RetriesPageRequestMsg:
		// Handle page change request from Retries view
		a.retriesPage = msg.Page
		cmds = append(cmds, func() tea.Msg {
			return a.fetchRetriesDataCmd()
		})

	case views.ScheduledPageRequestMsg:
		// Handle page change request from Scheduled view
		a.scheduledPage = msg.Page
		cmds = append(cmds, func() tea.Msg {
			return a.fetchScheduledDataCmd()
		})

	case views.DeadPageRequestMsg:
		// Handle page change request from Dead view
		a.deadPage = msg.Page
		cmds = append(cmds, func() tea.Msg {
			return a.fetchDeadDataCmd()
		})

	case tea.KeyMsg:
		// Handle global keybindings first
		switch {
		case key.Matches(msg, a.keys.Quit):
			return a, tea.Quit

		case key.Matches(msg, a.keys.ToggleTheme):
			a.darkMode = !a.darkMode
			a.applyTheme()

		case key.Matches(msg, a.keys.View1):
			a.activeView = 0
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View2):
			a.activeView = 1
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View3):
			a.activeView = 2
			a.queuesPage = 1      // Reset to first page when switching to Queues view
			a.selectedQueue = 0   // Reset to first queue when switching to Queues view
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View4):
			a.activeView = 3
			a.retriesPage = 1 // Reset to first page when switching to Retries view
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View5):
			a.activeView = 4
			a.scheduledPage = 1 // Reset to first page when switching to Scheduled view
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View6):
			a.activeView = 5
			a.deadPage = 1 // Reset to first page when switching to Dead view
			cmds = append(cmds, a.views[a.activeView].Init())

		default:
			// Pass to active view
			updatedView, cmd := a.views[a.activeView].Update(msg)
			a.views[a.activeView] = updatedView
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true

		// Update component dimensions
		a.metrics = a.metrics.SetWidth(msg.Width)
		a.navbar = a.navbar.SetWidth(msg.Width)

		// Calculate content height (total - metrics - navbar - border)
		// Border takes 2 lines (top + bottom)
		contentHeight := msg.Height - a.metrics.Height() - a.navbar.Height() - 2
		// Border takes 2 chars (left + right)
		contentWidth := msg.Width - 2
		for i := range a.views {
			// Busy (1) and Queues (2) views render their own border with header area outside, so give them extra height
			if i == 1 || i == 2 {
				a.views[i] = a.views[i].SetSize(contentWidth+2, contentHeight+3)
			} else if i == 3 || i == 4 || i == 5 {
				// Retries (3), Scheduled (4), and Dead (5) render their own border but have no header area outside
				a.views[i] = a.views[i].SetSize(contentWidth+2, contentHeight+2)
			} else {
				a.views[i] = a.views[i].SetSize(contentWidth, contentHeight)
			}
		}
		a.errorPopup = a.errorPopup.SetSize(contentWidth, contentHeight)

	default:
		// Clear connection error on successful metrics update
		if _, ok := msg.(components.MetricsUpdateMsg); ok {
			a.connectionError = nil
		}

		// Pass messages to metrics for updates
		updatedMetrics, cmd := a.metrics.Update(msg)
		a.metrics = updatedMetrics
		cmds = append(cmds, cmd)

		// Pass to active view
		updatedView, cmd := a.views[a.activeView].Update(msg)
		a.views[a.activeView] = updatedView
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

// View implements tea.Model
func (a App) View() string {
	if !a.ready {
		return "Initializing..."
	}

	// Content area with border and title on border
	title := a.views[a.activeView].Name()
	contentHeight := a.height - a.metrics.Height() - a.navbar.Height() - 2
	contentWidth := a.width - 2

	var content string
	// Busy (1), Queues (2), Retries (3), Scheduled (4), and Dead (5) views handle their own border
	if a.activeView == 1 || a.activeView == 2 || a.activeView == 3 || a.activeView == 4 || a.activeView == 5 {
		content = a.views[a.activeView].View()
	} else {
		content = a.renderBorderedBox(title, a.views[a.activeView].View(), contentWidth, contentHeight)
	}

	// If there's a connection error, overlay the error popup
	if a.connectionError != nil {
		popup := a.errorPopup.SetMessage(a.connectionError.Error())
		content = popup.Render(content)
	}

	// Build the layout: metrics (top) + content (middle) + navbar (bottom)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		a.metrics.View(),
		content,
		a.navbar.View(),
	)
}

// renderBorderedBox renders content in a box with title on the top border
func (a App) renderBorderedBox(title, content string, width, height int) string {
	border := lipgloss.RoundedBorder()
	borderStyle := lipgloss.NewStyle().Foreground(a.styles.Theme.Border)
	titleStyle := a.styles.ViewTitle

	// Build top border with title
	// ╭─ Title ─────────────────╮
	titleText := " " + title + " "
	styledTitle := titleStyle.Render(titleText)
	titleWidth := lipgloss.Width(styledTitle)

	topLeft := borderStyle.Render(border.TopLeft)
	topRight := borderStyle.Render(border.TopRight)
	hBar := borderStyle.Render(border.Top)

	// Calculate remaining width for horizontal bars
	remainingWidth := width - 2 - titleWidth // -2 for corners
	leftPad := 1
	rightPad := remainingWidth - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	topBorder := topLeft + strings.Repeat(hBar, leftPad) + styledTitle + strings.Repeat(hBar, rightPad) + topRight

	// Build content area with side borders
	vBar := borderStyle.Render(border.Left)
	vBarRight := borderStyle.Render(border.Right)

	innerWidth := width - 2
	contentStyle := lipgloss.NewStyle().
		Width(innerWidth).
		Height(height)

	renderedContent := contentStyle.Render(content)
	contentLines := strings.Split(renderedContent, "\n")

	var middleLines []string
	for _, line := range contentLines {
		// Pad line to inner width
		lineWidth := lipgloss.Width(line)
		if lineWidth < innerWidth {
			line += strings.Repeat(" ", innerWidth-lineWidth)
		}
		middleLines = append(middleLines, vBar+line+vBarRight)
	}

	// Build bottom border
	bottomLeft := borderStyle.Render(border.BottomLeft)
	bottomRight := borderStyle.Render(border.BottomRight)
	bottomBorder := bottomLeft + strings.Repeat(hBar, width-2) + bottomRight

	// Combine all parts
	result := topBorder + "\n"
	result += strings.Join(middleLines, "\n") + "\n"
	result += bottomBorder

	return result
}

// applyTheme updates all components with the current theme
func (a *App) applyTheme() {
	var t theme.Theme
	if a.darkMode {
		t = theme.Dark
	} else {
		t = theme.Light
	}

	a.styles = theme.NewStyles(t)

	// Update components
	a.metrics = a.metrics.SetStyles(&a.styles)
	a.navbar = a.navbar.SetStyles(&a.styles)
	a.errorPopup = a.errorPopup.SetStyles(&a.styles)

	// Update views
	viewStyles := views.Styles{
		Text:           a.styles.ViewText,
		Muted:          a.styles.ViewMuted,
		Title:          a.styles.ViewTitle,
		Border:         a.styles.Theme.Border,
		MetricLabel:    a.styles.MetricLabel,
		MetricValue:    a.styles.MetricValue,
		TableHeader:    a.styles.TableHeader,
		TableSelected:  a.styles.TableSelected,
		TableSeparator: a.styles.TableSeparator,
		BoxPadding:     a.styles.BoxPadding,
		BorderStyle:    a.styles.BorderStyle,
		NavKey:         a.styles.NavKey,
	}
	for i := range a.views {
		a.views[i] = a.views[i].SetStyles(viewStyles)
	}
}
