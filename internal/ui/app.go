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
	"github.com/kpumuk/lazykiq/internal/ui/components/stackbar"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

// tickMsg is sent every 5 seconds to trigger a metrics update.
type tickMsg time.Time

// connectionErrorMsg indicates a Redis connection error occurred.
type connectionErrorMsg struct {
	err error
}

type viewID int

const (
	viewDashboard viewID = iota
	viewBusy
	viewQueues
	viewRetries
	viewScheduled
	viewDead
	viewErrorsSummary
	viewErrorsDetails
	viewJobDetail
	viewMetrics
	viewJobMetrics
)

// App is the main application model.
type App struct {
	keys            KeyMap
	width           int
	height          int
	ready           bool
	viewStack       []viewID
	viewOrder       []viewID
	viewRegistry    map[viewID]views.View
	metrics         metrics.Model
	stackbar        stackbar.Model
	navbar          navbar.Model
	errorPopup      errorpopup.Model
	styles          theme.Styles
	sidekiq         sidekiq.API
	connectionError error
}

// New creates a new App instance.
func New(client sidekiq.API, version string) App {
	styles := theme.NewStyles()
	brand := "Lazykiq"
	if version != "" {
		brand = "Lazykiq v" + version
	}

	viewOrder := []viewID{
		viewDashboard,
		viewBusy,
		viewQueues,
		viewRetries,
		viewScheduled,
		viewDead,
		viewErrorsSummary,
		viewMetrics,
	}
	viewRegistry := map[viewID]views.View{
		viewDashboard:     views.NewDashboard(client),
		viewBusy:          views.NewBusy(client),
		viewQueues:        views.NewQueues(client),
		viewRetries:       views.NewRetries(client),
		viewScheduled:     views.NewScheduled(client),
		viewDead:          views.NewDead(client),
		viewErrorsSummary: views.NewErrorsSummary(client),
		viewErrorsDetails: views.NewErrorsDetails(client),
		viewJobDetail:     views.NewJobDetail(),
		viewMetrics:       views.NewMetrics(client),
		viewJobMetrics:    views.NewJobMetrics(client),
	}

	// Apply styles to views
	viewStyles := views.Styles{
		Text:            styles.ViewText,
		Muted:           styles.ViewMuted,
		Title:           styles.ViewTitle,
		MetricLabel:     styles.MetricLabel,
		MetricValue:     styles.MetricValue,
		TableHeader:     styles.TableHeader,
		TableSelected:   styles.TableSelected,
		TableSeparator:  styles.TableSeparator,
		BoxPadding:      styles.BoxPadding,
		BorderStyle:     styles.BorderStyle,
		FocusBorder:     styles.FocusBorder,
		NavKey:          styles.NavKey,
		ChartSuccess:    styles.ChartSuccess,
		ChartFailure:    styles.ChartFailure,
		JSONKey:         styles.JSONKey,
		JSONString:      styles.JSONString,
		JSONNumber:      styles.JSONNumber,
		JSONBool:        styles.JSONBool,
		JSONNull:        styles.JSONNull,
		JSONPunctuation: styles.JSONPunctuation,
		QueueText:       styles.QueueText,
		QueueWeight:     styles.QueueWeight,
	}
	for _, id := range viewOrder {
		viewRegistry[id] = viewRegistry[id].SetStyles(viewStyles)
	}
	viewRegistry[viewErrorsDetails] = viewRegistry[viewErrorsDetails].SetStyles(viewStyles)
	viewRegistry[viewJobDetail] = viewRegistry[viewJobDetail].SetStyles(viewStyles)
	viewRegistry[viewJobMetrics] = viewRegistry[viewJobMetrics].SetStyles(viewStyles)

	// Build navbar view infos
	navViews := make([]navbar.ViewInfo, len(viewOrder))
	for i, id := range viewOrder {
		navViews[i] = navbar.ViewInfo{Name: viewRegistry[id].Name()}
	}

	return App{
		keys:         DefaultKeyMap(),
		viewStack:    []viewID{viewDashboard},
		viewOrder:    viewOrder,
		viewRegistry: viewRegistry,
		metrics: metrics.New(
			metrics.WithStyles(metrics.Styles{
				Bar:   styles.MetricsBar,
				Fill:  styles.MetricsFill,
				Label: styles.MetricsLabel,
				Value: styles.MetricsValue,
			}),
		),
		stackbar: stackbar.New(
			stackbar.WithStyles(stackbar.Styles{
				Bar:   styles.StackBar,
				Item:  styles.StackItem,
				Arrow: styles.StackArrow,
			}),
			stackbar.WithStack([]string{viewRegistry[viewDashboard].Name()}),
		),
		navbar: navbar.New(
			navbar.WithStyles(navbar.Styles{
				Bar:   styles.NavBar,
				Key:   styles.NavKey,
				Item:  styles.NavItem,
				Quit:  styles.NavQuit,
				Brand: styles.NavBrand,
			}),
			navbar.WithViews(navViews),
			navbar.WithBrand(brand),
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
	activeID := a.activeViewID()
	return tea.Batch(
		a.viewRegistry[activeID].Init(),
		a.metrics.Init(),
		a.fetchStatsCmd, // Fetch stats immediately
		tickCmd(),       // Start the ticker for subsequent updates
	)
}

// tickCmd returns a command that sends a tick message after 5 seconds.
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		// Always fetch stats for metrics bar
		cmds = append(cmds, a.fetchStatsCmd)

		// Broadcast refresh to active view (views now fetch their own data)
		cmds = append(cmds, a.updateView(a.activeViewID(), views.RefreshMsg{}))

		cmds = append(cmds, tickCmd())

	case connectionErrorMsg:
		// Store the connection error
		a.connectionError = msg.err

	case views.ConnectionErrorMsg:
		// Handle connection errors from views
		a.connectionError = msg.Err

	case views.DashboardRedisInfoMsg:
		cmds = append(cmds, a.updateView(viewDashboard, msg))

	case views.DashboardHistoryMsg:
		cmds = append(cmds, a.updateView(viewDashboard, msg))

	case views.ShowJobDetailMsg:
		if setter, ok := a.viewRegistry[viewJobDetail].(views.JobDetailSetter); ok {
			setter.SetJob(msg.Job)
		}
		cmds = append(cmds, a.pushView(viewJobDetail))

	case views.ShowErrorDetailsMsg:
		if setter, ok := a.viewRegistry[viewErrorsDetails].(views.ErrorDetailsSetter); ok {
			setter.SetErrorGroup(msg.DisplayClass, msg.ErrorClass, msg.Queue, msg.Query)
		}
		cmds = append(cmds, a.pushView(viewErrorsDetails))

	case views.ShowJobMetricsMsg:
		if setter, ok := a.viewRegistry[viewJobMetrics].(views.JobMetricsSetter); ok {
			setter.SetJobMetrics(msg.Job, msg.Period)
		}
		cmds = append(cmds, a.pushView(viewJobMetrics))

	case tea.KeyMsg:
		activeID := a.activeViewID()
		if view, ok := a.viewRegistry[activeID].(interface{ FilterFocused() bool }); ok && view.FilterFocused() {
			return a, a.updateView(activeID, msg)
		}

		if msg.String() == "esc" && len(a.viewStack) > 1 {
			a.popView()
			return a, tea.Batch(cmds...)
		}

		// Handle global keybindings first
		switch {
		case key.Matches(msg, a.keys.Quit):
			return a, tea.Quit

		case key.Matches(msg, a.keys.View1):
			cmds = append(cmds, a.setActiveView(viewDashboard))

		case key.Matches(msg, a.keys.View2):
			cmds = append(cmds, a.setActiveView(viewBusy))

		case key.Matches(msg, a.keys.View3):
			cmds = append(cmds, a.setActiveView(viewQueues))

		case key.Matches(msg, a.keys.View4):
			cmds = append(cmds, a.setActiveView(viewRetries))

		case key.Matches(msg, a.keys.View5):
			cmds = append(cmds, a.setActiveView(viewScheduled))

		case key.Matches(msg, a.keys.View6):
			cmds = append(cmds, a.setActiveView(viewDead))

		case key.Matches(msg, a.keys.View7):
			cmds = append(cmds, a.setActiveView(viewErrorsSummary))

		case key.Matches(msg, a.keys.View8):
			cmds = append(cmds, a.setActiveView(viewMetrics))

		default:
			// Pass to active view
			cmds = append(cmds, a.updateView(activeID, msg))
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true

		// Update component dimensions
		a.metrics.SetWidth(msg.Width)
		a.stackbar.SetWidth(msg.Width)
		a.navbar.SetWidth(msg.Width)

		// Calculate content size (total - metrics - stackbar - navbar)
		contentHeight := msg.Height - a.metrics.Height() - a.stackbar.Height() - a.navbar.Height()
		contentWidth := msg.Width
		for id, view := range a.viewRegistry {
			a.viewRegistry[id] = view.SetSize(contentWidth, contentHeight)
		}
		a.errorPopup.SetSize(contentWidth, contentHeight)

	case metrics.UpdateMsg:
		// Clear connection error on successful metrics update
		a.connectionError = nil

		// Pass to metrics bar
		updatedMetrics, cmd := a.metrics.Update(msg)
		a.metrics = updatedMetrics
		cmds = append(cmds, cmd)

		// Always forward to dashboard (for realtime chart tracking, even when not active)
		cmds = append(cmds, a.updateView(viewDashboard, msg))

	default:
		// Pass to active view
		cmds = append(cmds, a.updateView(a.activeViewID(), msg))
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

	content := a.viewRegistry[a.activeViewID()].View()

	// If there's a connection error, overlay the error popup
	if a.connectionError != nil {
		a.errorPopup.SetMessage(a.connectionError.Error())
		errorPanel := a.errorPopup.View()
		if errorPanel != "" {
			panelWidth := lipgloss.Width(errorPanel)
			panelHeight := lipgloss.Height(errorPanel)
			contentHeight := a.height - a.metrics.Height() - a.stackbar.Height() - a.navbar.Height()
			panelX := max((a.width-panelWidth)/2, 0)
			panelY := a.metrics.Height() + max((contentHeight-panelHeight)/2, 0)

			canvas := lipgloss.NewCanvas(
				lipgloss.NewLayer(lipgloss.JoinVertical(
					lipgloss.Left,
					a.metrics.View(),
					content,
					a.stackbar.View(),
					a.navbar.View(),
				)),
				lipgloss.NewLayer(errorPanel).X(panelX).Y(panelY).Z(1),
			)
			v.SetContent(canvas.Render())
			return v
		}
	}

	// Build the layout: metrics (top) + content (middle) + navbar (bottom)
	v.SetContent(lipgloss.JoinVertical(
		lipgloss.Left,
		a.metrics.View(),
		content,
		a.stackbar.View(),
		a.navbar.View(),
	))

	return v
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
			UpdatedAt: time.Now(),
		},
	}
}

func (a App) activeViewID() viewID {
	if len(a.viewStack) == 0 {
		return viewDashboard
	}
	return a.viewStack[len(a.viewStack)-1]
}

func (a App) stackNames() []string {
	if len(a.viewStack) == 0 {
		return nil
	}
	names := make([]string, 0, len(a.viewStack))
	for _, id := range a.viewStack {
		if view, ok := a.viewRegistry[id]; ok {
			names = append(names, view.Name())
		}
	}
	return names
}

func (a *App) updateView(id viewID, msg tea.Msg) tea.Cmd {
	view, ok := a.viewRegistry[id]
	if !ok {
		return nil
	}
	updatedView, cmd := view.Update(msg)
	a.viewRegistry[id] = updatedView
	return cmd
}

func (a *App) setActiveView(id viewID) tea.Cmd {
	for _, existing := range a.viewStack {
		if existing == viewDashboard || existing == id {
			continue
		}
		if disposable, ok := a.viewRegistry[existing].(views.Disposable); ok {
			disposable.Dispose()
		}
	}
	a.viewStack = []viewID{id}
	a.stackbar.SetStack(a.stackNames())
	if view, ok := a.viewRegistry[id]; ok {
		return view.Init()
	}
	return nil
}

func (a *App) pushView(id viewID) tea.Cmd {
	if len(a.viewStack) > 0 && a.viewStack[len(a.viewStack)-1] == id {
		a.stackbar.SetStack(a.stackNames())
		return nil
	}
	a.viewStack = append(a.viewStack, id)
	a.stackbar.SetStack(a.stackNames())
	if view, ok := a.viewRegistry[id]; ok {
		return view.Init()
	}
	return nil
}

func (a *App) popView() {
	if len(a.viewStack) <= 1 {
		return
	}

	popped := a.viewStack[len(a.viewStack)-1]
	a.viewStack = a.viewStack[:len(a.viewStack)-1]
	if popped != viewDashboard {
		if disposable, ok := a.viewRegistry[popped].(views.Disposable); ok {
			disposable.Dispose()
		}
	}
	a.stackbar.SetStack(a.stackNames())
}
