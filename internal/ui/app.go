// Package ui renders the Bubble Tea application UI.
package ui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/contextbar"
	"github.com/kpumuk/lazykiq/internal/ui/components/errorpopup"
	"github.com/kpumuk/lazykiq/internal/ui/components/metrics"
	"github.com/kpumuk/lazykiq/internal/ui/components/navbar"
	"github.com/kpumuk/lazykiq/internal/ui/components/stackbar"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	devtoolsdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/devtools"
	helpdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/help"
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
	viewQueueDetails
	viewQueuesList
	viewProcessesList
	viewRetries
	viewScheduled
	viewDead
	viewErrorsSummary
	viewErrorsDetails
	viewJobDetail
	viewMetrics
	viewJobMetrics
)

const contextbarDefaultHeight = 5

// App is the main application model.
type App struct {
	keys                    KeyMap
	width                   int
	height                  int
	ready                   bool
	viewStack               []viewID
	viewOrder               []viewID
	viewRegistry            map[viewID]views.View
	metrics                 metrics.Model
	contextbar              contextbar.Model
	stackbar                stackbar.Model
	navbar                  navbar.Model
	errorPopup              errorpopup.Model
	dialogs                 dialogs.DialogCmp
	styles                  theme.Styles
	sidekiq                 sidekiq.API
	connectionError         error
	dangerousActionsEnabled bool
	brand                   string
	devMode                 bool
	devTracker              *devtools.Tracker
	devToolsOpen            bool
	devToolsPanel           *devtoolsdialog.Model
	devViewKeys             map[viewID]string
	devViewLabels           map[viewID]string
}

// New creates a new App instance.
func New(client sidekiq.API, version string, dangerousActionsEnabled bool, devTracker *devtools.Tracker) App {
	styles := theme.NewStyles()
	keys := DefaultKeyMap()
	keys.DevTools.SetEnabled(devTracker != nil)
	brand := "Lazykiq"
	if version != "" {
		brand = "Lazykiq v" + version
	}

	viewOrder := []viewID{
		viewDashboard,
		viewBusy,
		viewQueueDetails,
		viewRetries,
		viewScheduled,
		viewDead,
		viewErrorsSummary,
		viewMetrics,
	}
	viewRegistry := map[viewID]views.View{
		viewDashboard:     views.NewDashboard(client),
		viewBusy:          views.NewBusy(client),
		viewQueueDetails:  views.NewQueueDetails(client),
		viewQueuesList:    views.NewQueuesList(client),
		viewProcessesList: views.NewProcessesList(client),
		viewRetries:       views.NewRetries(client),
		viewScheduled:     views.NewScheduled(client),
		viewDead:          views.NewDead(client),
		viewErrorsSummary: views.NewErrorsSummary(client),
		viewErrorsDetails: views.NewErrorsDetails(client),
		viewJobDetail:     views.NewJobDetail(),
		viewMetrics:       views.NewMetrics(client),
		viewJobMetrics:    views.NewJobMetrics(client),
	}

	devViewKeys := map[viewID]string{
		viewDashboard:     "dashboard",
		viewBusy:          "busy",
		viewQueueDetails:  "queue_details",
		viewQueuesList:    "queues_list",
		viewProcessesList: "processes",
		viewRetries:       "retries",
		viewScheduled:     "scheduled",
		viewDead:          "dead",
		viewErrorsSummary: "errors",
		viewErrorsDetails: "errors_details",
		viewMetrics:       "metrics",
		viewJobMetrics:    "job_metrics",
	}
	devViewLabels := map[viewID]string{
		viewDashboard:     "Dashboard",
		viewBusy:          "Busy",
		viewQueueDetails:  "Queues",
		viewQueuesList:    "Queue list",
		viewProcessesList: "Processes",
		viewRetries:       "Retries",
		viewScheduled:     "Scheduled",
		viewDead:          "Dead",
		viewErrorsSummary: "Errors",
		viewErrorsDetails: "Error details",
		viewMetrics:       "Metrics",
		viewJobMetrics:    "Job metrics",
	}

	var devToolsPanel *devtoolsdialog.Model
	if devTracker != nil {
		devToolsPanel = devtoolsdialog.New(
			devtoolsdialog.WithStyles(devtoolsdialog.Styles{
				Title:          styles.ViewTitle,
				Border:         styles.FocusBorder,
				Text:           styles.ViewText,
				Muted:          styles.ViewMuted,
				TableHeader:    styles.TableHeader,
				TableSelected:  styles.TableSelected,
				TableSeparator: styles.TableSeparator,
			}),
			devtoolsdialog.WithTitle("Dev Commands"),
			devtoolsdialog.WithTracker(devTracker),
		)
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
		ChartAxis:       styles.ChartAxis,
		ChartLabel:      styles.ChartLabel,
		ChartSuccess:    styles.ChartSuccess,
		ChartFailure:    styles.ChartFailure,
		ChartHistogram:  styles.ChartHistogram,
		JSONKey:         styles.JSONKey,
		JSONString:      styles.JSONString,
		JSONNumber:      styles.JSONNumber,
		JSONBool:        styles.JSONBool,
		JSONNull:        styles.JSONNull,
		JSONPunctuation: styles.JSONPunctuation,
		QueueText:       styles.QueueText,
		QueueWeight:     styles.QueueWeight,
		FilterFocused:   styles.FilterFocused,
		FilterBlurred:   styles.FilterBlurred,
		DangerAction:    styles.ContextDangerKey,
		NeutralAction:   styles.ContextKey,
	}
	for _, id := range viewOrder {
		viewRegistry[id] = viewRegistry[id].SetStyles(viewStyles)
	}
	viewRegistry[viewQueuesList] = viewRegistry[viewQueuesList].SetStyles(viewStyles)
	viewRegistry[viewProcessesList] = viewRegistry[viewProcessesList].SetStyles(viewStyles)
	viewRegistry[viewErrorsDetails] = viewRegistry[viewErrorsDetails].SetStyles(viewStyles)
	viewRegistry[viewJobDetail] = viewRegistry[viewJobDetail].SetStyles(viewStyles)
	viewRegistry[viewJobMetrics] = viewRegistry[viewJobMetrics].SetStyles(viewStyles)

	for _, view := range viewRegistry {
		if toggle, ok := view.(views.DangerousActionsToggle); ok {
			toggle.SetDangerousActionsEnabled(dangerousActionsEnabled)
		}
	}

	if devTracker != nil {
		for id, key := range devViewKeys {
			if view, ok := viewRegistry[id]; ok {
				if setter, ok := view.(views.DevelopmentSetter); ok {
					setter.SetDevelopment(devTracker, key)
				}
			}
		}
	}

	// Build navbar view infos
	navViews := make([]navbar.ViewInfo, len(viewOrder))
	for i, id := range viewOrder {
		navViews[i] = navbar.ViewInfo{Name: viewRegistry[id].Name()}
	}

	return App{
		keys:         keys,
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
		contextbar: contextbar.New(
			contextbar.WithStyles(contextbar.Styles{
				Bar:        styles.ContextBar,
				Label:      styles.ContextLabel,
				Value:      styles.ContextValue,
				Key:        styles.ContextKey,
				Desc:       styles.ContextDesc,
				DangerKey:  styles.ContextDangerKey,
				DangerDesc: styles.ContextDangerDesc,
			}),
			contextbar.WithHeight(contextbarDefaultHeight),
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
			navbar.WithHelp(keys.Help),
		),
		errorPopup: errorpopup.New(
			errorpopup.WithStyles(errorpopup.Styles{
				Title:   styles.ErrorTitle,
				Message: styles.ViewMuted,
				Border:  styles.ErrorBorder,
			}),
		),
		dialogs:                 dialogs.NewDialogCmp(),
		styles:                  styles,
		sidekiq:                 client,
		dangerousActionsEnabled: dangerousActionsEnabled,
		brand:                   brand,
		devMode:                 devTracker != nil,
		devTracker:              devTracker,
		devToolsPanel:           devToolsPanel,
		devViewKeys:             devViewKeys,
		devViewLabels:           devViewLabels,
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
	dialogUpdated := false

	switch msg.(type) {
	case dialogs.OpenDialogMsg, dialogs.CloseDialogMsg:
		updated, cmd := a.dialogs.Update(msg)
		a.dialogs = updated
		a.syncContextbar()
		return a, cmd
	}

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

	case views.ShowQueuesListMsg:
		cmds = append(cmds, a.pushView(viewQueuesList))

	case views.ShowQueueDetailsMsg:
		if setter, ok := a.viewRegistry[viewQueueDetails].(views.QueueDetailsSetter); ok {
			setter.SetQueue(msg.QueueName)
		}
		cmds = append(cmds, a.setActiveView(viewQueueDetails))
		// Trigger immediate refresh to load the selected queue's jobs
		cmds = append(cmds, a.updateView(viewQueueDetails, views.RefreshMsg{}))

	case views.ShowProcessesListMsg:
		cmds = append(cmds, a.pushView(viewProcessesList))

	case views.ShowProcessSelectMsg:
		if selector, ok := a.viewRegistry[viewBusy].(views.ProcessSelector); ok {
			selector.SetProcessIdentity(msg.Identity)
		}
		cmds = append(cmds, a.popAndRefresh(viewBusy))

	case tea.KeyMsg:
		if a.dialogs.HasDialogs() {
			if key.Matches(msg, a.keys.Quit) {
				return a, tea.Quit
			}
			updated, cmd := a.dialogs.Update(msg)
			a.dialogs = updated
			return a, cmd
		}
		activeID := a.activeViewID()

		if a.devToolsOpen {
			if key.Matches(msg, a.keys.Quit) {
				return a, tea.Quit
			}
			if key.Matches(msg, a.keys.Help) {
				cmd := a.toggleHelpDialog()
				a.syncContextbar()
				a.syncDevToolsPanel()
				return a, cmd
			}
			if key.Matches(msg, a.keys.DevTools) || msg.String() == "esc" {
				a.toggleDevToolsPanel()
				a.syncContextbar()
				a.syncDevToolsPanel()
				return a, nil
			}

			switch {
			case key.Matches(msg, a.keys.View1):
				cmds = append(cmds, a.setActiveView(viewDashboard))
			case key.Matches(msg, a.keys.View2):
				cmds = append(cmds, a.setActiveView(viewBusy))
			case key.Matches(msg, a.keys.View3):
				cmds = append(cmds, a.setActiveView(viewQueueDetails))
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
				if a.devToolsPanel != nil {
					updated, cmd := a.devToolsPanel.Update(msg)
					a.devToolsPanel = updated
					a.syncContextbar()
					a.syncDevToolsPanel()
					return a, cmd
				}
				a.syncContextbar()
				a.syncDevToolsPanel()
				return a, nil
			}

			a.syncContextbar()
			a.syncDevToolsPanel()
			return a, tea.Batch(cmds...)
		}

		if msg.String() == "esc" && len(a.viewStack) > 1 {
			a.popView()
			return a, tea.Batch(cmds...)
		}

		// Handle global keybindings first
		switch {
		case key.Matches(msg, a.keys.Quit):
			return a, tea.Quit
		case key.Matches(msg, a.keys.Help):
			return a, a.toggleHelpDialog()
		case a.devMode && key.Matches(msg, a.keys.DevTools):
			a.toggleDevToolsPanel()
			return a, nil

		case key.Matches(msg, a.keys.View1):
			cmds = append(cmds, a.setActiveView(viewDashboard))

		case key.Matches(msg, a.keys.View2):
			cmds = append(cmds, a.setActiveView(viewBusy))

		case key.Matches(msg, a.keys.View3):
			cmds = append(cmds, a.setActiveView(viewQueueDetails))

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
		a.contextbar.SetWidth(msg.Width)
		a.stackbar.SetWidth(msg.Width)
		a.navbar.SetWidth(msg.Width)

		a.resizeViews()
		updated, cmd := a.dialogs.Update(msg)
		a.dialogs = updated
		cmds = append(cmds, cmd)
		dialogUpdated = true

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

	if a.dialogs.HasDialogs() && !dialogUpdated {
		updated, cmd := a.dialogs.Update(msg)
		a.dialogs = updated
		cmds = append(cmds, cmd)
	}

	a.syncContextbar()
	a.syncDevToolsPanel()
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
	items := a.contextHeaderItems()
	if len(items) == 0 {
		items = a.contextItems()
	}
	a.contextbar.SetItems(items)
	a.contextbar.SetHints(a.contextHints())
	a.navbar.SetBrand(a.brandLine())
	sections := []string{
		a.metrics.View(),
		a.contextbar.View(),
		content,
	}
	if a.devToolsOpen && a.devToolsPanel != nil {
		sections = append(sections, a.devToolsPanel.View())
	}
	sections = append(sections, a.stackbar.View(), a.navbar.View())
	base := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// If there's a connection error, overlay the error popup
	if a.connectionError != nil || a.dialogs.HasDialogs() {
		layers := []*lipgloss.Layer{
			lipgloss.NewLayer(base),
		}

		if a.connectionError != nil {
			a.errorPopup.SetMessage(a.connectionError.Error())
			errorPanel := a.errorPopup.View()
			if errorPanel != "" {
				panelWidth := lipgloss.Width(errorPanel)
				panelHeight := lipgloss.Height(errorPanel)
				availableHeight := a.height - a.metrics.Height() - a.contextbar.Height() - a.stackbar.Height() - a.navbar.Height()
				contentHeight := max(availableHeight-a.devToolsPanelHeight(availableHeight), 0)
				panelX := max((a.width-panelWidth)/2, 0)
				panelY := a.metrics.Height() + a.contextbar.Height() + max((contentHeight-panelHeight)/2, 0)
				layers = append(layers, lipgloss.NewLayer(errorPanel).X(panelX).Y(panelY).Z(1))
			}
		}

		if a.dialogs.HasDialogs() {
			layers = append(layers, a.dialogs.GetLayers()...)
		}

		canvas := lipgloss.NewCanvas(layers...)
		v.SetContent(canvas.Render())
		return v
	}
	// Build the layout: metrics (top) + content (middle) + navbar (bottom)
	v.SetContent(base)

	return v
}

func (a *App) syncContextbar() {
	if !a.ready || a.width == 0 || a.height == 0 {
		return
	}
	active := a.viewRegistry[a.activeViewID()]
	desired := contextbarDefaultHeight
	if provider, ok := active.(views.HeaderLinesProvider); ok {
		if lines := provider.HeaderLines(); len(lines) > 0 {
			desired = len(lines)
		}
	}
	if desired <= 0 {
		desired = contextbarDefaultHeight
	}
	if desired == a.contextbar.Height() {
		return
	}
	a.contextbar.SetHeight(desired)
	a.resizeViews()
}

func (a *App) syncDevToolsPanel() {
	if !a.devToolsOpen || a.devToolsPanel == nil {
		return
	}
	activeID := a.activeViewID()
	key := a.devViewKeys[activeID]
	label := a.devViewLabels[activeID]
	if label == "" {
		label = a.viewRegistry[activeID].Name()
	}
	a.devToolsPanel.SetKey(key)
	a.devToolsPanel.SetMeta(label)
}

func (a *App) resizeViews() {
	availableHeight := a.height - a.metrics.Height() - a.contextbar.Height() - a.stackbar.Height() - a.navbar.Height()
	panelHeight := a.devToolsPanelHeight(availableHeight)
	contentHeight := max(availableHeight-panelHeight, 0)
	contentWidth := a.width
	for id, view := range a.viewRegistry {
		a.viewRegistry[id] = view.SetSize(contentWidth, contentHeight)
	}
	a.errorPopup.SetSize(contentWidth, contentHeight)
	if a.devToolsOpen && a.devToolsPanel != nil && panelHeight > 0 {
		a.devToolsPanel.SetSize(contentWidth, panelHeight)
	}
}

func (a *App) devToolsPanelHeight(available int) int {
	if !a.devToolsOpen || a.devToolsPanel == nil {
		return 0
	}
	if available <= 0 {
		return 0
	}
	desired := max(available/3, 10)
	maxPanel := max(available-4, 0)
	if maxPanel == 0 {
		return 0
	}
	return min(desired, maxPanel)
}

func (a App) contextItems() []contextbar.Item {
	active := a.viewRegistry[a.activeViewID()]
	provider, ok := active.(views.ContextProvider)
	if !ok {
		return nil
	}
	items := provider.ContextItems()
	if len(items) == 0 {
		return nil
	}

	result := make([]contextbar.Item, 0, len(items))
	for _, item := range items {
		result = append(result, contextbar.KeyValueItem{
			Label: item.Label,
			Value: item.Value,
		})
	}
	return result
}

func (a App) contextHeaderItems() []contextbar.Item {
	active := a.viewRegistry[a.activeViewID()]
	provider, ok := active.(views.HeaderLinesProvider)
	if !ok {
		return nil
	}
	lines := provider.HeaderLines()
	if len(lines) == 0 {
		return nil
	}

	items := make([]contextbar.Item, 0, len(lines))
	for _, line := range lines {
		items = append(items, contextbar.FormattedItem{Line: line})
	}
	return items
}

func (a App) contextHints() []contextbar.Hint {
	normal := a.globalHeaderHints()
	mutations := []key.Binding{}

	active := a.viewRegistry[a.activeViewID()]
	if provider, ok := active.(views.HintProvider); ok {
		normal = append(normal, provider.HintBindings()...)
	}
	if a.dangerousActionsEnabled {
		if provider, ok := active.(views.MutationHintProvider); ok {
			mutations = append(mutations, provider.MutationBindings()...)
		}
	}

	normal = dedupeBindings(filterMiniHelpBindings(normal))
	mutations = dedupeBindings(filterMiniHelpBindings(mutations))

	result := make([]contextbar.Hint, 0, len(normal)+len(mutations))
	seen := map[string]bool{}
	appendHint := func(binding key.Binding, kind contextbar.HintKind) {
		help := binding.Help()
		if help.Key == "" {
			return
		}
		key := help.Key + "\x00" + help.Desc
		if seen[key] {
			return
		}
		seen[key] = true
		result = append(result, contextbar.Hint{Binding: binding, Kind: kind})
	}

	for _, binding := range normal {
		appendHint(binding, contextbar.HintNormal)
	}
	for _, binding := range mutations {
		appendHint(binding, contextbar.HintDanger)
	}

	return result
}

func (a App) brandLine() string {
	if a.brand == "" {
		return ""
	}
	if !a.devMode || a.devTracker == nil {
		return a.brand
	}

	activeID := a.activeViewID()
	key := a.devViewKeys[activeID]
	if key == "" {
		return a.brand
	}

	sample, ok := a.devTracker.Sample(key)
	if !ok {
		return a.brand
	}

	callLabel := "calls"
	if sample.Count == 1 {
		callLabel = "call"
	}

	return fmt.Sprintf("%s | %d %s %s", a.brand, sample.Count, callLabel, devtools.FormatDuration(sample.Duration))
}

func (a App) globalHeaderHints() []key.Binding {
	if len(a.viewStack) > 1 {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("esc"),
				key.WithHelp("esc", "back"),
			),
		}
	}
	return nil
}

func (a App) toggleHelpDialog() tea.Cmd {
	if a.dialogs.ActiveDialogID() == helpdialog.DialogID {
		return func() tea.Msg { return dialogs.CloseDialogMsg{} }
	}
	active := a.viewRegistry[a.activeViewID()]
	sections := a.helpSections(active)
	return func() tea.Msg {
		return dialogs.OpenDialogMsg{
			Model: helpdialog.New(
				helpdialog.WithStyles(helpdialog.Styles{
					Title:   a.styles.ViewTitle,
					Border:  a.styles.FocusBorder,
					Section: a.styles.ViewTitle,
					Key:     a.styles.ContextKey,
					Desc:    a.styles.ViewText,
					Muted:   a.styles.ViewMuted,
				}),
				helpdialog.WithSections(sections),
			),
		}
	}
}

func (a *App) toggleDevToolsPanel() {
	if !a.devMode || a.devToolsPanel == nil {
		return
	}
	a.devToolsOpen = !a.devToolsOpen
	a.resizeViews()
	a.syncDevToolsPanel()
}

func (a App) helpSections(active views.View) []helpdialog.Section {
	sections := []helpdialog.Section{
		{
			Title:    "Global",
			Bindings: a.globalHelpBindings(),
		},
	}

	if provider, ok := active.(views.HelpProvider); ok {
		for _, section := range provider.HelpSections() {
			sections = append(sections, helpdialog.Section{
				Title:    section.Title,
				Bindings: section.Bindings,
				Lines:    section.Lines,
				Column:   helpColumn(section.Column),
			})
		}
	}

	if provider, ok := active.(views.TableHelpProvider); ok {
		if bindings := provider.TableHelp(); len(bindings) > 0 {
			sections = append(sections, helpdialog.Section{
				Title:    "Table",
				Bindings: bindings,
				Column:   helpdialog.ColumnAuto,
			})
		}
	}

	return sections
}

func helpColumn(column views.HelpColumn) helpdialog.Column {
	switch column {
	case views.HelpColumnAuto:
		return helpdialog.ColumnAuto
	case views.HelpColumnLeft:
		return helpdialog.ColumnLeft
	case views.HelpColumnRight:
		return helpdialog.ColumnRight
	default:
		return helpdialog.ColumnAuto
	}
}

func (a App) globalHelpBindings() []key.Binding {
	bindings := []key.Binding{
		a.keys.View1,
		a.keys.View2,
		a.keys.View3,
		a.keys.View4,
		a.keys.View5,
		a.keys.View6,
		a.keys.View7,
		a.keys.View8,
	}
	if a.devMode {
		bindings = append(bindings, a.keys.DevTools)
	}
	bindings = append(bindings, a.keys.Help, a.keys.Quit)
	if len(a.viewStack) > 1 {
		bindings = append(bindings, key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		))
	}
	return bindings
}

func dedupeBindings(bindings []key.Binding) []key.Binding {
	seen := map[string]bool{}
	result := make([]key.Binding, 0, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		help := binding.Help()
		if help.Key == "" {
			continue
		}
		key := help.Key + "\x00" + help.Desc
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, binding)
	}
	return result
}

func filterMiniHelpBindings(bindings []key.Binding) []key.Binding {
	result := make([]key.Binding, 0, len(bindings))
	for _, binding := range bindings {
		help := binding.Help()
		if help.Key == "q" || help.Key == "?" {
			continue
		}
		result = append(result, binding)
	}
	return result
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

func (a *App) popAndRefresh(id viewID) tea.Cmd {
	if len(a.viewStack) <= 1 {
		return a.updateView(id, views.RefreshMsg{})
	}

	popped := a.viewStack[len(a.viewStack)-1]
	a.viewStack = a.viewStack[:len(a.viewStack)-1]
	if popped != viewDashboard {
		if disposable, ok := a.viewRegistry[popped].(views.Disposable); ok {
			disposable.Dispose()
		}
	}
	a.stackbar.SetStack(a.stackNames())
	return a.updateView(id, views.RefreshMsg{})
}
