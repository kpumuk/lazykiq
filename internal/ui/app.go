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
}

// New creates a new App instance
func New() App {
	styles := theme.NewStyles(theme.Dark)

	viewList := []views.View{
		views.NewDashboard(),
		views.NewQueues(),
		views.NewBusy(),
		views.NewRetries(),
		views.NewScheduled(),
		views.NewDead(),
	}

	// Apply styles to views
	viewStyles := views.Styles{
		Text:  styles.ViewText,
		Muted: styles.ViewMuted,
	}
	for i := range viewList {
		viewList[i] = viewList[i].SetStyles(viewStyles)
	}

	return App{
		keys:       DefaultKeyMap(),
		activeView: 0,
		views:      viewList,
		metrics:    components.NewMetrics(&styles),
		navbar:     components.NewNavbar(viewList, &styles),
		errorPopup: components.NewErrorPopup(&styles),
		styles:     styles,
		darkMode:   true,
		sidekiq:    sidekiq.NewClient(),
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

// Update implements tea.Model
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		// Fetch stats and schedule next tick
		cmds = append(cmds, func() tea.Msg {
			return a.fetchStatsCmd()
		}, tickCmd())

	case connectionErrorMsg:
		// Store the connection error
		a.connectionError = msg.err

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
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View4):
			a.activeView = 3
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View5):
			a.activeView = 4
			cmds = append(cmds, a.views[a.activeView].Init())

		case key.Matches(msg, a.keys.View6):
			a.activeView = 5
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
			a.views[i] = a.views[i].SetSize(contentWidth, contentHeight)
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

	content := a.renderBorderedBox(title, a.views[a.activeView].View(), contentWidth, contentHeight)

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
		Text:  a.styles.ViewText,
		Muted: a.styles.ViewMuted,
	}
	for i := range a.views {
		a.views[i] = a.views[i].SetStyles(viewStyles)
	}
}
