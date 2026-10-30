// Package ui renders the Bubble Tea application UI.
package ui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/contextbar"
	"github.com/kpumuk/lazykiq/internal/ui/components/errorpopup"
	"github.com/kpumuk/lazykiq/internal/ui/components/navbar"
	"github.com/kpumuk/lazykiq/internal/ui/components/stackbar"
	"github.com/kpumuk/lazykiq/internal/ui/components/stats"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	devtoolsdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/devtools"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	helpdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/help"
	"github.com/kpumuk/lazykiq/internal/ui/requestctx"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

// tickMsg is sent every 5 seconds to trigger a metrics update.
type tickMsg time.Time

// statsResultMsg binds stats to their originating app and request generation.
type statsResultMsg struct {
	owner   *requestctx.Controller
	context context.Context
	data    stats.UpdateMsg
	err     error
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
	viewPeriodic
	viewPeriodicDetail
)

const contextbarDefaultHeight = 5

type dialogOwner struct {
	id         viewID
	view       views.View
	generation uint64
}

// viewIntentOwner identifies the activation that created a local UI command.
type viewIntentOwner struct {
	owner            dialogOwner
	lifetime         context.Context
	dialogGeneration uint64
}

// App is the main application model.
type App struct {
	ctx                     context.Context
	cancel                  context.CancelFunc
	keys                    KeyMap
	width                   int
	height                  int
	ready                   bool
	viewStack               []viewID
	viewOrder               []viewID
	viewRegistry            map[viewID]views.View
	metrics                 stats.Model
	contextbar              contextbar.Model
	stackbar                stackbar.Model
	navbar                  navbar.Model
	errorPopup              errorpopup.Model
	dialogs                 dialogs.DialogCmp
	styles                  theme.Styles
	darkBackground          bool
	sidekiq                 sidekiq.API
	connectionError         error
	dangerousActionsEnabled bool
	devTracker              *devtools.Tracker
	statsRequest            *requestctx.Controller
	viewGeneration          uint64
	dialogGeneration        uint64
}

// New creates a new App instance.
func New(client sidekiq.API, version string, dangerousActionsEnabled bool, devTracker *devtools.Tracker) App {
	return NewWithContext(context.Background(), client, version, dangerousActionsEnabled, devTracker)
}

// NewWithContext creates an App whose service work follows the supplied lifetime.
// Use Run to also cancel work on every program exit.
func NewWithContext(parent context.Context, client sidekiq.API, version string, dangerousActionsEnabled bool, devTracker *devtools.Tracker) App {
	ctx, cancel := context.WithCancel(parent)
	styles := theme.NewStyles()

	keys := DefaultKeyMap()
	keys.DevTools.SetEnabled(devTracker != nil)
	periodicEnabled := client != nil && client.GetCapabilities().SupportsPeriodicJobs()
	keys.View9.SetEnabled(periodicEnabled)
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
	if periodicEnabled {
		viewOrder = append(viewOrder, viewPeriodic)
		viewRegistry[viewPeriodic] = views.NewPeriodic(client)
		viewRegistry[viewPeriodicDetail] = views.NewPeriodicDetail(client)
	}

	for _, view := range viewRegistry {
		if setter, ok := view.(views.ContextSetter); ok {
			setter.SetContext(ctx)
		}
		if toggle, ok := view.(views.DangerousActionsToggle); ok {
			toggle.SetDangerousActionsEnabled(dangerousActionsEnabled)
		}
	}

	// Build navbar view infos
	navViews := make([]navbar.ViewInfo, len(viewOrder))
	for i, id := range viewOrder {
		navViews[i] = navbar.ViewInfo{Name: viewRegistry[id].Name()}
	}

	app := App{
		ctx:                     ctx,
		cancel:                  cancel,
		statsRequest:            &requestctx.Controller{},
		keys:                    keys,
		viewStack:               []viewID{viewDashboard},
		viewOrder:               viewOrder,
		viewRegistry:            viewRegistry,
		metrics:                 stats.New(),
		contextbar:              contextbar.New(contextbar.WithHeight(contextbarDefaultHeight)),
		stackbar:                stackbar.New(stackbar.WithStack([]string{viewRegistry[viewDashboard].Name()})),
		navbar:                  navbar.New(navbar.WithViews(navViews), navbar.WithBrand(brand), navbar.WithHelp(keys.Help)),
		errorPopup:              errorpopup.New(),
		dialogs:                 dialogs.NewDialogCmp(),
		darkBackground:          true,
		sidekiq:                 client,
		dangerousActionsEnabled: dangerousActionsEnabled,
		devTracker:              devTracker,
	}
	app.applyStyles(styles)
	return app
}

// Run starts the terminal UI and cancels all owned service work on return.
// Cancellation is cooperative and does not undo submitted Redis mutations.
func (a App) Run(opts ...tea.ProgramOption) (tea.Model, error) {
	defer a.cancel()
	opts = append(opts, tea.WithContext(a.ctx))
	return tea.NewProgram(a, opts...).Run()
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	activeID := a.activeViewID()
	return tea.Batch(
		requestBackgroundCmd(),
		a.prepareView(activeID).Init(),
		a.metrics.Init(),
		a.fetchStatsCmd(), // Fetch stats immediately
		tickCmd(),         // Start the ticker for subsequent updates
	)
}

// requestBackgroundCmd subscribes to live appearance changes (DEC mode 2031)
// and queries the initial background for terminals that only support OSC 11.
func requestBackgroundCmd() tea.Cmd {
	return tea.Batch(tea.Raw(ansi.SetModeLightDark), tea.RequestBackgroundColor)
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

	switch msg := msg.(type) {
	case views.IntentMsg:
		origin, ok := msg.Owner.(viewIntentOwner)
		if !ok || origin.lifetime != a.ctx || a.ctx.Err() != nil ||
			!a.currentOwner(origin.owner) || origin.dialogGeneration != a.dialogGeneration {
			return a, nil
		}
		if open, ok := msg.Msg.(dialogs.OpenDialogMsg); ok {
			open.Owner = origin.owner
			return a.Update(open)
		}
		return a.Update(msg.Msg)
	case devtoolsdialog.CommandResultMsg:
		msg.Collect(a.devTracker)
		return a, nil
	case dialogs.OpenDialogMsg:
		if msg.Owner == nil {
			msg.Owner = dialogOwner{a.activeViewID(), a.viewRegistry[a.activeViewID()], a.viewGeneration}
		}
		previous := a.dialogs.ActiveSession()
		updated, cmd := a.dialogs.Update(msg)
		a.dialogs = updated
		if a.dialogs.ActiveSession() != previous {
			a.dialogGeneration++
		}
		a.styleDialogs()
		a.syncContextbar()
		return a, cmd
	case dialogs.CloseDialogMsg:
		if msg.Session != nil && msg.Session != a.dialogs.ActiveSession() {
			return a, nil
		}
		previous := a.dialogs.ActiveSession()
		updated, cmd := a.dialogs.Update(msg)
		a.dialogs = updated
		if a.dialogs.ActiveSession() != previous {
			a.dialogGeneration++
		}
		if msg.Session != nil && msg.Action != nil {
			if owner, ok := msg.Session.Owner.(dialogOwner); ok && a.currentOwner(owner) {
				cmds = append(cmds, a.updateView(owner.id, msg.Action))
			}
		}
		a.syncContextbar()
		return a, tea.Batch(append(cmds, cmd)...)
	case filterdialog.ActionMsg, confirmdialog.ActionMsg:
		// Dialog actions are accepted only inside an owned close transition.
		return a, nil
	}

	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if msg.Color != nil {
			a.setDarkBackground(msg.IsDark())
		}
		return a, nil

	// Appearance preference can differ from the configured terminal theme.
	// Resolve notifications against the actual background, just like startup.
	case uv.DarkColorSchemeEvent, uv.LightColorSchemeEvent:
		return a, tea.RequestBackgroundColor

	case tea.FocusMsg:
		cmds = append(cmds, tea.RequestBackgroundColor, a.updateView(a.activeViewID(), msg))

	case tea.ResumeMsg:
		return a, requestBackgroundCmd()

	case tickMsg:
		// Config reloads can change colors without an appearance notification.
		cmds = append(cmds, tea.RequestBackgroundColor)
		// Always fetch stats for metrics bar
		cmds = append(cmds, a.fetchStatsCmd())

		// Broadcast refresh to active view (views now fetch their own data)
		cmds = append(cmds, a.updateView(a.activeViewID(), views.RefreshMsg{}))

		cmds = append(cmds, tickCmd())

	case statsResultMsg:
		if msg.owner != a.statsRequest || msg.context == nil || msg.context.Err() != nil {
			return a, nil
		}
		if msg.err != nil {
			if !requestctx.IsCanceled(msg.err) {
				a.connectionError = msg.err
			}
			return a, nil
		}
		return a.Update(msg.data)

	case views.ReadResultMsg:
		if msg.Owner != a.viewRegistry[a.activeViewID()] || !msg.Current() {
			return a, nil
		}
		cmds = append(cmds, a.updateView(a.activeViewID(), msg))
		if a.activeViewID() == viewPeriodicDetail {
			a.stackbar.SetStack(a.stackNames())
		}
		if msg.Err != nil && !requestctx.IsCanceled(msg.Err) {
			a.connectionError = msg.Err
		}

	case views.ConnectionErrorMsg:
		if msg.RequestID != 0 {
			active, ok := a.viewRegistry[a.activeViewID()].(views.RequestIDProvider)
			if !ok || active.RequestID() != msg.RequestID {
				break
			}
		}
		// Handle connection errors from views
		a.connectionError = msg.Err
	case views.QueueActionResultMsg:
		if msg.Err != nil {
			a.connectionError = msg.Err
		}
		cmds = append(cmds, a.updateView(a.activeViewID(), msg))

	case views.PeriodicActionResultMsg:
		if msg.Err != nil {
			a.connectionError = msg.Err
		}
		cmds = append(cmds, a.updateView(a.activeViewID(), msg))

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
			setter.SetErrorGroup(msg.Key, msg.Query)
		}
		cmds = append(cmds, a.pushView(viewErrorsDetails))

	case views.ShowJobMetricsMsg:
		if setter, ok := a.viewRegistry[viewJobMetrics].(views.JobMetricsSetter); ok {
			setter.SetJobMetrics(msg.Job, msg.Period)
		}
		cmds = append(cmds, a.pushView(viewJobMetrics))

	case views.ShowPeriodicDetailMsg:
		if setter, ok := a.viewRegistry[viewPeriodicDetail].(views.PeriodicJobSetter); ok {
			setter.SetPeriodicJob(msg.LID)
			cmds = append(cmds, a.pushView(viewPeriodicDetail))
		}

	case views.ShowQueuesListMsg:
		cmds = append(cmds, a.pushView(viewQueuesList))

	case views.ShowQueueDetailsMsg:
		if setter, ok := a.viewRegistry[viewQueueDetails].(views.QueueDetailsSetter); ok {
			setter.SetQueue(msg.QueueName)
		}
		cmds = append(cmds, a.setActiveView(viewQueueDetails))

	case views.ShowProcessesListMsg:
		cmds = append(cmds, a.pushView(viewProcessesList))

	case views.ShowProcessSelectMsg:
		if selector, ok := a.viewRegistry[viewBusy].(views.ProcessSelector); ok {
			selector.SetProcessIdentity(msg.Identity)
		}
		cmds = append(cmds, a.popAndRefresh(viewBusy))

	case tea.KeyPressMsg:
		if a.dialogs.HasDialogs() {
			if key.Matches(msg, a.keys.QuitModal) {
				return a, tea.Quit
			}
			updated, cmd := a.dialogs.Update(msg)
			a.dialogs = updated
			return a, cmd
		}
		activeID := a.activeViewID()

		if msg.String() == "esc" && len(a.viewStack) > 1 {
			if activeID == viewPeriodicDetail {
				cmd := a.popAndRefresh(viewPeriodic)
				return a, cmd
			}
			a.popView()
			return a, tea.Batch(cmds...)
		}

		// Handle global keybindings first
		switch {
		case key.Matches(msg, a.keys.Quit):
			return a, tea.Quit
		case key.Matches(msg, a.keys.Help):
			return a, a.toggleHelpDialog()
		case a.devTracker != nil && key.Matches(msg, a.keys.DevTools):
			return a, a.toggleDevToolsDialog()

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

		case key.Matches(msg, a.keys.View9):
			cmds = append(cmds, a.setActiveView(viewPeriodic))

		default:
			// Pass to active view
			cmds = append(cmds, a.updateView(activeID, msg))
		}

	case tea.WindowSizeMsg:
		a.width = max(msg.Width, 0)
		a.height = max(msg.Height, 0)
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

	case stats.UpdateMsg:
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
	return a, tea.Batch(cmds...)
}

// View implements tea.Model.
func (a App) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.ReportFocus = true

	if !a.ready {
		v.SetContent("Initializing...")
		return v
	}
	if a.width == 0 || a.height == 0 {
		return v
	}
	if a.width < 4 || a.height < 5 {
		v.SetContent(fitRectangle(a.styles.ViewMuted.Render("Terminal too small"), a.width, a.height))
		return v
	}

	content := fitRectangle(a.viewRegistry[a.activeViewID()].View(), a.width, a.contentHeight())
	items := a.contextHeaderItems()
	if len(items) == 0 {
		items = a.contextItems()
	}
	a.contextbar.SetItems(items)
	a.contextbar.SetHints(a.contextHints())
	regions := []string{fitRectangle(a.metrics.View(), a.width, a.metrics.Height())}
	if a.contextbar.Height() > 0 {
		regions = append(regions, fitRectangle(a.contextbar.View(), a.width, a.contextbar.Height()))
	}
	regions = append(regions, content,
		fitRectangle(a.stackbar.View(), a.width, a.stackbar.Height()),
		fitRectangle(a.navbar.View(), a.width, a.navbar.Height()))
	base := lipgloss.JoinVertical(lipgloss.Left, regions...)

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
				contentHeight := a.contentHeight()
				panelX := max((a.width-panelWidth)/2, 0)
				panelY := a.metrics.Height() + a.contextbar.Height() + max((contentHeight-panelHeight)/2, 0)
				layers = append(layers, lipgloss.NewLayer(errorPanel).X(panelX).Y(panelY).Z(1))
			}
		}

		if a.dialogs.HasDialogs() {
			layers = append(layers, a.dialogs.GetLayers()...)
		}

		v.SetContent(fitRectangle(lipgloss.NewCompositor(layers...).Render(), a.width, a.height))
		return v
	}
	// Build the layout: metrics (top) + content (middle) + navbar (bottom)
	v.SetContent(fitRectangle(base, a.width, a.height))

	return v
}

func (a *App) syncContextbar() {
	if !a.ready || a.width == 0 || a.height == 0 {
		return
	}
	desired := a.contextHeight()
	if desired == a.contextbar.Height() {
		return
	}
	a.resizeViews()
}

func (a App) contextHeight() int {
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
	// Give short terminals five content rows before spending space on hints.
	available := a.height - a.metrics.Height() - a.stackbar.Height() - a.navbar.Height() - 5
	return min(desired, max(available, 0))
}

func (a App) contentHeight() int {
	return max(a.height-a.metrics.Height()-a.contextbar.Height()-a.stackbar.Height()-a.navbar.Height(), 0)
}

func (a *App) resizeViews() {
	a.contextbar.SetHeight(a.contextHeight())
	contentHeight := a.contentHeight()
	contentWidth := a.width
	for id, view := range a.viewRegistry {
		a.viewRegistry[id] = view.SetSize(contentWidth, contentHeight)
	}
	a.errorPopup.SetSize(contentWidth, contentHeight)
}

// fitRectangle bounds each region before composition, including ANSI and wide
// graphemes. Padding keeps the following chrome at its allocated row.
func fitRectangle(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), height)]
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n") + strings.Repeat("\n", height-len(lines))
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
				helpdialog.WithSections(sections),
			),
		}
	}
}

func (a App) toggleDevToolsDialog() tea.Cmd {
	if a.devTracker == nil {
		return nil
	}
	if a.dialogs.ActiveDialogID() == devtoolsdialog.DialogID {
		return func() tea.Msg { return dialogs.CloseDialogMsg{} }
	}

	return func() tea.Msg {
		opts := []devtoolsdialog.Option{
			devtoolsdialog.WithTitle("Dev Console"),
			devtoolsdialog.WithTracker(a.devTracker),
		}
		if commander, ok := a.sidekiq.(interface {
			Do(context.Context, ...any) (any, error)
		}); ok {
			opts = append(opts, devtoolsdialog.WithClient(commander))
		}

		model := devtoolsdialog.New(opts...)
		model.SetContext(a.ctx)
		return dialogs.OpenDialogMsg{Model: model}
	}
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
	if a.keys.View9.Enabled() {
		bindings = append(bindings, a.keys.View9)
	}
	if a.devTracker != nil {
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

// fetchStatsCmd fetches Sidekiq stats with ownership checked at delivery.
func (a *App) fetchStatsCmd() tea.Cmd {
	ctx := a.statsRequest.Start(devtools.WithTracker(a.ctx, "app.fetchStatsCmd"))
	client, owner := a.sidekiq, a.statsRequest
	return func() tea.Msg {
		sidekiqStats, err := client.GetStats(ctx)
		if err != nil {
			if requestctx.IsCanceled(err) {
				return nil
			}
			return statsResultMsg{owner: owner, context: ctx, err: err}
		}

		return statsResultMsg{owner: owner, context: ctx, data: stats.UpdateMsg{
			Data: stats.Data{
				Processed: sidekiqStats.Processed,
				Failed:    sidekiqStats.Failed,
				Busy:      sidekiqStats.Busy,
				Enqueued:  sidekiqStats.Enqueued,
				Retries:   sidekiqStats.Retries,
				Scheduled: sidekiqStats.Scheduled,
				Dead:      sidekiqStats.Dead,
				UpdatedAt: time.Now(),
			},
		}}
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
	_, ok := a.viewRegistry[id]
	if !ok {
		return nil
	}
	updatedView, cmd := a.prepareView(id).Update(msg)
	a.viewRegistry[id] = updatedView
	return cmd
}

func (a App) currentOwner(owner dialogOwner) bool {
	return owner.id == a.activeViewID() && owner.generation == a.viewGeneration &&
		owner.view == a.viewRegistry[owner.id]
}

func (a App) prepareView(id viewID) views.View {
	view := a.viewRegistry[id]
	if setter, ok := view.(views.IntentOwnerSetter); ok {
		setter.SetIntentOwner(viewIntentOwner{
			owner:            dialogOwner{id, view, a.viewGeneration},
			lifetime:         a.ctx,
			dialogGeneration: a.dialogGeneration,
		})
	}
	return view
}

func (a *App) cancelViewRequests(id viewID) {
	view, ok := a.viewRegistry[id]
	if !ok {
		return
	}
	if canceler, ok := view.(views.RequestCanceler); ok {
		canceler.CancelRequests()
	}
}

func (a *App) setActiveView(id viewID) tea.Cmd {
	a.viewGeneration++
	for _, existing := range a.viewStack {
		if existing != id {
			a.cancelViewRequests(existing)
		}
		if existing == viewDashboard || existing == id {
			continue
		}
		if disposable, ok := a.viewRegistry[existing].(views.Disposable); ok {
			disposable.Dispose()
		}
	}
	a.viewStack = []viewID{id}
	a.stackbar.SetStack(a.stackNames())
	if _, ok := a.viewRegistry[id]; ok {
		return a.prepareView(id).Init()
	}
	return nil
}

func (a *App) pushView(id viewID) tea.Cmd {
	if len(a.viewStack) > 0 && a.viewStack[len(a.viewStack)-1] == id {
		a.stackbar.SetStack(a.stackNames())
		return nil
	}
	a.viewGeneration++
	if len(a.viewStack) > 0 {
		a.cancelViewRequests(a.viewStack[len(a.viewStack)-1])
	}
	a.viewStack = append(a.viewStack, id)
	a.stackbar.SetStack(a.stackNames())
	if _, ok := a.viewRegistry[id]; ok {
		return a.prepareView(id).Init()
	}
	return nil
}

func (a *App) popTopView() {
	if len(a.viewStack) <= 1 {
		return
	}
	a.viewGeneration++

	popped := a.viewStack[len(a.viewStack)-1]
	a.cancelViewRequests(popped)
	a.viewStack = a.viewStack[:len(a.viewStack)-1]
	if popped != viewDashboard {
		if disposable, ok := a.viewRegistry[popped].(views.Disposable); ok {
			disposable.Dispose()
		}
	}
	a.stackbar.SetStack(a.stackNames())
}

func (a *App) popView() {
	a.popTopView()
}

func (a *App) popAndRefresh(id viewID) tea.Cmd {
	a.popTopView()
	return a.updateView(id, views.RefreshMsg{})
}

func (a *App) setDarkBackground(dark bool) {
	if a.darkBackground == dark {
		return
	}
	a.darkBackground = dark
	a.applyStyles(theme.NewStylesForBackground(dark))
}

// applyStyles updates existing models, preserving navigation and input state.
func (a *App) applyStyles(styles theme.Styles) {
	a.styles = styles
	viewStyles := views.Styles{
		Text:            styles.ViewText,
		Muted:           styles.ViewMuted,
		Title:           styles.ViewTitle,
		MetricLabel:     styles.MetricLabel,
		MetricValue:     styles.MetricValue,
		TableHeader:     styles.TableHeader,
		TableSelected:   styles.TableSelected,
		TableSeparator:  styles.TableSeparator,
		ScrollbarTrack:  styles.ScrollbarTrack,
		ScrollbarThumb:  styles.ScrollbarThumb,
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
		QueuePaused:     styles.QueuePaused,
		QueueText:       styles.QueueText,
		QueueWeight:     styles.QueueWeight,
		FilterFocused:   styles.FilterFocused,
		FilterBlurred:   styles.FilterBlurred,
		DangerAction:    styles.ContextDangerKey,
		NeutralAction:   styles.ContextKey,
	}
	for id, view := range a.viewRegistry {
		a.viewRegistry[id] = view.SetStyles(viewStyles)
	}
	a.metrics.SetStyles(stats.Styles{Bar: styles.MetricsBar, Fill: styles.MetricsFill, Label: styles.MetricsLabel, Value: styles.MetricsValue})
	a.contextbar.SetStyles(contextbar.Styles{Bar: styles.ContextBar, Label: styles.ContextLabel, Value: styles.ContextValue, Key: styles.ContextKey, Desc: styles.ContextDesc, DangerKey: styles.ContextDangerKey, DangerDesc: styles.ContextDangerDesc})
	a.stackbar.SetStyles(stackbar.Styles{Bar: styles.StackBar, Item: styles.StackItem})
	a.navbar.SetStyles(navbar.Styles{Bar: styles.NavBar, Key: styles.NavKey, Item: styles.NavItem, Quit: styles.NavQuit, Brand: styles.NavBrand})
	a.errorPopup.SetStyles(errorpopup.Styles{Title: styles.ErrorTitle, Message: styles.ViewMuted, Border: styles.ErrorBorder})
	a.styleDialogs()
}

// styleDialogs also handles dialogs opened by commands that captured an older palette.
func (a *App) styleDialogs() {
	for _, dialog := range a.dialogs.Dialogs() {
		switch m := dialog.(type) {
		case *helpdialog.Model:
			m.SetStyles(helpdialog.Styles{
				Title:          a.styles.ViewTitle,
				Border:         a.styles.FocusBorder,
				Section:        a.styles.ViewTitle,
				Key:            a.styles.ContextKey,
				Desc:           a.styles.ViewText,
				Muted:          a.styles.ViewMuted,
				ScrollbarTrack: a.styles.ScrollbarTrack,
				ScrollbarThumb: a.styles.ScrollbarThumb,
			})
		case *devtoolsdialog.Model:
			m.SetStyles(devtoolsdialog.Styles{
				Title:          a.styles.ViewTitle,
				Border:         a.styles.FocusBorder,
				Text:           a.styles.ViewText,
				Muted:          a.styles.ViewMuted,
				Prompt:         a.styles.MetricLabel,
				Placeholder:    a.styles.ViewMuted,
				Cursor:         a.styles.ViewText,
				TableHeader:    a.styles.TableHeader,
				TableSelected:  a.styles.TableSelected,
				TableSeparator: a.styles.TableSeparator,
				ScrollbarTrack: a.styles.ScrollbarTrack,
				ScrollbarThumb: a.styles.ScrollbarThumb,
			})
		case *filterdialog.Model:
			m.SetStyles(filterdialog.Styles{Title: a.styles.ViewTitle, Border: a.styles.FocusBorder, Prompt: a.styles.MetricLabel, Text: a.styles.ViewText, Placeholder: a.styles.ViewMuted, Cursor: a.styles.ViewText})
		case *confirmdialog.Model:
			yes := a.styles.ContextDangerKey
			if m.Styles().ButtonYesActive.GetBackground() == m.Styles().ButtonNoActive.GetBackground() {
				yes = a.styles.ContextKey
			}
			m.SetStyles(confirmdialog.Styles{Title: a.styles.ViewTitle, Border: a.styles.FocusBorder, Text: a.styles.ViewText, Muted: a.styles.ViewMuted, Button: a.styles.ViewMuted.Padding(0, 1), ButtonYesActive: yes, ButtonNoActive: a.styles.ContextKey})
		}
	}
}
