// Package ui renders the Bubble Tea application UI.
package ui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/errorpopup"
	"github.com/kpumuk/lazykiq/internal/ui/components/metrics"
	"github.com/kpumuk/lazykiq/internal/ui/components/navbar"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

// tickMsg is sent every 5 seconds to trigger a metrics update.
type tickMsg time.Time

// connectionErrorMsg indicates a Redis connection error occurred.
type connectionErrorMsg struct {
	err error
}

// App is the main application model.
type App struct {
	keys            KeyMap
	width           int
	height          int
	ready           bool
	activeView      int
	views           []views.View
	metrics         metrics.Model
	navbar          navbar.Model
	errorPopup      errorpopup.Model
	styles          theme.Styles
	sidekiq         *sidekiq.Client
	connectionError error
}

// New creates a new App instance.
func New(client *sidekiq.Client) App {
	styles := theme.NewStyles()

	viewList := []views.View{
		views.NewDashboard(client),
		views.NewBusy(client),
		views.NewQueues(client),
		views.NewRetries(client),
		views.NewScheduled(client),
		views.NewDead(client),
	}

	// Apply styles to views
	viewStyles := views.Styles{
		Text:           styles.ViewText,
		Muted:          styles.ViewMuted,
		Title:          styles.ViewTitle,
		MetricLabel:    styles.MetricLabel,
		MetricValue:    styles.MetricValue,
		TableHeader:    styles.TableHeader,
		TableSelected:  styles.TableSelected,
		TableSeparator: styles.TableSeparator,
		BoxPadding:     styles.BoxPadding,
		BorderStyle:    styles.BorderStyle,
		FocusBorder:    styles.FocusBorder,
		NavKey:         styles.NavKey,
		ChartSuccess:   styles.ChartSuccess,
		ChartFailure:   styles.ChartFailure,
	}
	for i := range viewList {
		viewList[i] = viewList[i].SetStyles(viewStyles)
	}

	// Build navbar view infos
	navViews := make([]navbar.ViewInfo, len(viewList))
	for i, v := range viewList {
		navViews[i] = navbar.ViewInfo{Name: v.Name()}
	}

	return App{
		keys:       DefaultKeyMap(),
		activeView: 0,
		views:      viewList,
		metrics: metrics.New(
			metrics.WithStyles(metrics.Styles{
				Bar:   styles.MetricsBar,
				Fill:  styles.MetricsFill,
				Label: styles.MetricsLabel,
				Value: styles.MetricsValue,
			}),
		),
		navbar: navbar.New(
			navbar.WithStyles(navbar.Styles{
				Bar:  styles.NavBar,
				Key:  styles.NavKey,
				Item: styles.NavItem,
				Quit: styles.NavQuit,
			}),
			navbar.WithViews(navViews),
		),
		errorPopup: errorpopup.New(
			errorpopup.WithStyles(errorpopup.Styles{
				Title:   styles.ErrorTitle,
				Message: styles.ViewMuted,
				Border:  styles.ErrorBorder,
			}),
		),
		styles:  styles,
		sidekiq: client,
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.views[a.activeView].Init(),
		a.metrics.Init(),
		func() tea.Msg { return a.fetchStatsCmd() }, // Fetch stats immediately
		tickCmd(), // Start the ticker for subsequent updates
	)
}

// tickCmd returns a command that sends a tick message after 5 seconds.
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchStatsCmd fetches Sidekiq stats and returns a metrics.UpdateMsg or connectionErrorMsg.
func (a App) fetchStatsCmd() tea.Msg {
	ctx := context.Background()
	stats, err := a.sidekiq.GetStats(ctx)
	if err != nil {
		// Return connection error message
		return connectionErrorMsg{err: err}
	}

	return metrics.UpdateMsg{
		Data: metrics.Data{
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

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		// Always fetch stats for metrics bar
		cmds = append(cmds, func() tea.Msg {
			return a.fetchStatsCmd()
		})

		// Broadcast refresh to active view (views now fetch their own data)
		updatedView, cmd := a.views[a.activeView].Update(views.RefreshMsg{})
		a.views[a.activeView] = updatedView
		cmds = append(cmds, cmd)

		cmds = append(cmds, tickCmd())

	case connectionErrorMsg:
		// Store the connection error
		a.connectionError = msg.err

	case views.ConnectionErrorMsg:
		// Handle connection errors from views
		a.connectionError = msg.Err

	case tea.KeyMsg:
		if view, ok := a.views[a.activeView].(interface{ FilterFocused() bool }); ok && view.FilterFocused() {
			updatedView, cmd := a.views[a.activeView].Update(msg)
			a.views[a.activeView] = updatedView
			return a, cmd
		}

		// Handle global keybindings first
		switch {
		case key.Matches(msg, a.keys.Quit):
			return a, tea.Quit

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
		a.metrics.SetWidth(msg.Width)
		a.navbar.SetWidth(msg.Width)

		// Calculate content size (total - metrics - navbar)
		contentHeight := msg.Height - a.metrics.Height() - a.navbar.Height()
		contentWidth := msg.Width
		for i := range a.views {
			a.views[i] = a.views[i].SetSize(contentWidth, contentHeight)
		}
		a.errorPopup.SetSize(contentWidth, contentHeight)

	default:
		// Clear connection error on successful metrics update
		if _, ok := msg.(metrics.UpdateMsg); ok {
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

// View implements tea.Model.
func (a App) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if !a.ready {
		v.SetContent("Initializing...")
		return v
	}

	content := a.views[a.activeView].View()

	// If there's a connection error, overlay the error popup
	if a.connectionError != nil {
		a.errorPopup.SetMessage(a.connectionError.Error())
		a.errorPopup.SetBackground(content)
		content = a.errorPopup.View()
	}

	// Build the layout: metrics (top) + content (middle) + navbar (bottom)
	v.SetContent(lipgloss.JoinVertical(
		lipgloss.Left,
		a.metrics.View(),
		content,
		a.navbar.View(),
	))

	return v
}
