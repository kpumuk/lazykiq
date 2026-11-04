package views

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/display"
	"github.com/kpumuk/lazykiq/internal/ui/requestctx"
)

// ShowRateLimitDetailMsg opens a current limiter by its stable Redis key.
type ShowRateLimitDetailMsg struct {
	Key     string
	Owner   *RateLimits
	context context.Context
}

// Current rejects navigation from an obsolete page, filter, or view lifetime.
func (m ShowRateLimitDetailMsg) Current() bool { return m.context != nil && m.context.Err() == nil }

type limiterPageState struct {
	start    sidekiq.LimiterCursor
	page     sidekiq.LimiterPage
	selected string
}

type limiterDataMsg struct {
	page    sidekiq.LimiterPage
	limiter *sidekiq.Limiter
}

// RateLimits owns either the cursor-paged inspector or its stacked detail.
type RateLimits struct {
	IntentScope
	dialogs.Opener
	requestctx.Scope
	client                       sidekiq.API
	detail                       bool
	limiterKey                   string
	width, height                int
	styles                       Styles
	table, history               table.Model
	pages                        []limiterPageState
	pageIndex                    int
	filter                       string
	limiter                      *sidekiq.Limiter
	ready, loading, focusHistory bool
	err                          error
	request                      requestctx.Controller
	readContext                  context.Context
	navigation                   requestctx.Controller
	intentContext                context.Context
}

// NewRateLimits creates the palette-only Enterprise inspector.
func NewRateLimits(client sidekiq.API) *RateLimits {
	return &RateLimits{
		client:  client,
		table:   table.New(table.WithColumns([]table.Column{{Title: "Name", Width: 28}, {Title: "Type", Width: 14}, {Title: "Capacity / rate", Width: 27}, {Title: "Usage / notes", Width: 40}})),
		history: table.New(table.WithColumns([]table.Column{{Title: "Bucket start", Width: 26}, {Title: "Attempts", Width: 20}})),
	}
}

// NewRateLimitDetail creates a stacked type-specific observation.
func NewRateLimitDetail(client sidekiq.API) *RateLimits {
	m := NewRateLimits(client)
	m.detail = true
	m.table.SetColumns([]table.Column{{Title: "Property / metric", Width: 24}, {Title: "Value", Width: 45}})
	return m
}

// SetLimiterKey replaces the detail request, rejecting old completions.
func (m *RateLimits) SetLimiterKey(key string) { m.Dispose(); m.limiterKey = key }

// Init implements View.
func (m *RateLimits) Init() tea.Cmd { return m.fetch(false) }

// Update implements View.
func (m *RateLimits) Update(msg tea.Msg) (View, tea.Cmd) {
	if result, ok := msg.(ReadResultMsg); ok {
		if result.Owner != m || !result.Current() {
			return m, nil
		}
		m.loading, m.ready, m.err = false, true, result.Err
		if result.Err != nil {
			return m, nil
		}
		msg = result.Data
	}
	switch msg := msg.(type) {
	case limiterDataMsg:
		m.ready, m.loading, m.err = true, false, nil
		if m.detail {
			m.limiter = msg.limiter
		} else {
			m.ensurePage()
			m.rememberSelection()
			m.pages[m.pageIndex].page = msg.page
		}
		m.buildRows()
	case RefreshMsg:
		if !m.loading {
			return m, m.fetch(false)
		}
	case filterdialog.ActionMsg:
		if !m.detail && msg.Action != filterdialog.ActionNone && m.filter != msg.Query {
			m.filter = msg.Query
			return m, m.resetDiscovery()
		}
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

// View implements View within the app's allocated rectangle.
func (m *RateLimits) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if !m.ready {
		return renderStatusMessage(m.Name(), "Loading...", m.styles, m.width, m.height)
	}
	if m.err != nil && m.table.RowCount() == 0 {
		return renderStatusMessage(m.Name(), "Read failed; r retries. "+m.err.Error(), m.styles, m.width, m.height)
	}
	if m.detail && (m.limiter == nil || m.limiter.Missing) {
		return renderStatusMessage("Rate Limit", "Limiter disappeared or expired; r refreshes, esc returns", m.styles, m.width, m.height)
	}
	meta := ""
	if m.loading {
		meta = "Refreshing..."
	}
	if m.err != nil {
		meta = "Read failed; r retries"
	}
	if !m.detail {
		page := m.pages[m.pageIndex].page
		pageMeta := fmt.Sprintf("Page %d · %d records", m.pageIndex+1, len(page.Items))
		if page.More {
			pageMeta += " · ] more"
		} else {
			pageMeta += " · end"
		}
		if meta != "" {
			pageMeta += " · " + meta
		}
		return m.renderFrame("Rate Limits", m.table.View(), pageMeta, m.filter, m.width, m.height, true)
	}
	if m.hasHistory() {
		historyMeta := m.limiter.Interval + " · " + m.limiter.HistoryZone + " · absent ≠ zero"
		if m.width < 100 {
			if m.focusHistory {
				return m.renderFrame("Bucket Counters", m.history.View(), historyMeta, "", m.width, m.height, true)
			}
			return m.renderFrame(m.limiter.Type, m.table.View(), meta+" · tab: counters", "", m.width, m.height, true)
		}
		left := m.width * 55 / 100
		return lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderFrame(m.limiter.Type, m.table.View(), meta, "", left, m.height, !m.focusHistory),
			m.renderFrame("Bucket Counters", m.history.View(), historyMeta, "", m.width-left, m.height, m.focusHistory))
	}
	content := m.table.View()
	if m.utilizationHeight() > 0 {
		content = m.concurrentUtilization() + "\n\n" + content
	}
	return m.renderFrame(m.limiter.Type, content, meta, "", m.width, m.height, true)
}

// Name implements View.
func (m *RateLimits) Name() string {
	if m.detail {
		if m.limiter != nil {
			return m.limiter.Name
		}
		return "Rate Limit"
	}
	return "Rate Limits"
}

// ShortHelp implements View. Rate Limits adds no navbar entry or global binding.
func (m *RateLimits) ShortHelp() []key.Binding { return nil }

// ContextItems describes the current scan window or observation.
func (m *RateLimits) ContextItems() []ContextItem {
	items := []ContextItem{{Label: "Limiter Redis", Value: m.client.DisplayLimiterRedis()}}
	if m.detail && m.limiter != nil {
		return append(items, ContextItem{Label: "Name", Value: m.limiter.Name}, ContextItem{Label: "Type", Value: m.limiter.Type})
	}
	if len(m.pages) > 0 {
		items = append(items, ContextItem{Label: "Page", Value: strconv.Itoa(m.pageIndex + 1)}, ContextItem{Label: "Records", Value: strconv.Itoa(len(m.pages[m.pageIndex].page.Items))})
	}
	if m.filter != "" {
		items = append(items, ContextItem{Label: "Name contains", Value: m.filter})
	}
	return items
}

// HintBindings exposes only local inspector shortcuts in the context bar.
func (m *RateLimits) HintBindings() []key.Binding {
	bindings := []key.Binding{helpBinding([]string{"c"}, "c", "copy name"), helpBinding([]string{"C"}, "C", "copy key"), helpBinding([]string{"r"}, "r", "refresh")}
	if m.detail {
		if m.hasHistory() {
			bindings = append(bindings, helpBinding([]string{"tab", "shift+tab"}, "tab", "switch panel"))
		}
		return bindings
	}
	return append([]key.Binding{helpBinding([]string{"/"}, "/", "filter name"), helpBinding([]string{"ctrl+u"}, "ctrl+u", "reset filter"), helpBinding([]string{"[", "]"}, "[/]", "previous/next scan page"), helpBinding([]string{"ctrl+r"}, "ctrl+r", "restart discovery"), helpBinding([]string{"enter"}, "enter", "inspect")}, bindings...)
}

// HelpSections explains observation and retention semantics.
func (m *RateLimits) HelpSections() []HelpSection {
	return []HelpSection{{Title: "Rate Limits", Bindings: m.HintBindings(), Lines: []string{
		"Read-only Enterprise inspection. No limiter operations are executed.",
		"Name search is a literal, case-sensitive substring. SCAN order is not a snapshot.",
		"r refreshes this page; ctrl+r restarts discovery. Empty scan pages may have more results.",
		"Unavailable means Sidekiq has not saved a value, or it expired. It does not mean zero.",
		"Concurrent utilization shows used (solid) and free (shaded) slots; red means all slots are used.",
		"Concurrent capacity and metrics are read together; expired locks may still count as used.",
		"Held/wait durations are cumulative seconds; overtime/reclaimed are counts.",
		"Last bucket fill and points balance reflect the last limiter update.",
		"Buckets drain and points refill between calls; these values do not include that change.",
		"Bucket counters include attempts, may exceed size, and expire independently.",
		"History covers 60 seconds, 60 minutes, 24 hours, or 30 calendar days.",
		"Bucket calendar keys use --limiter-timezone (UTC by default). Missing is not zero.",
	}}}
}

// TableHelp implements TableHelpProvider.
func (m *RateLimits) TableHelp() []key.Binding { return tableHelpBindings(m.table.KeyMap) }

// SetSize implements View.
func (m *RateLimits) SetSize(width, height int) View {
	m.width, m.height = max(width, 0), max(height, 0)
	left, right := m.width, m.width
	if m.hasHistory() && m.width >= 100 {
		left = m.width * 55 / 100
		right = m.width - left
	}
	m.table.SetSize(max(left-4, 0), max(m.height-2-m.utilizationHeight(), 0))
	m.history.SetSize(max(right-4, 0), max(m.height-2, 0))
	return m
}

// SetStyles implements View.
func (m *RateLimits) SetStyles(styles Styles) View {
	m.styles = styles
	m.table.SetStyles(tableStylesFromTheme(styles))
	m.history.SetStyles(tableStylesFromTheme(styles))
	return m
}

// CancelRequests invalidates reads before another root or stacked view takes over.
func (m *RateLimits) CancelRequests() { m.request.Cancel(); m.navigation.Cancel(); m.loading = false }

// Dispose releases observations and cursor/selection state when leaving the root.
func (m *RateLimits) Dispose() {
	m.CancelRequests()
	m.pages, m.limiter = nil, nil
	m.pageIndex, m.ready, m.focusHistory, m.err = 0, false, false, nil
	m.table.SetRows(nil)
	m.history.SetRows(nil)
}

func (m *RateLimits) ensurePage() {
	if len(m.pages) == 0 {
		m.pages = []limiterPageState{{}}
		m.pageIndex = 0
	}
}

func (m *RateLimits) fetch(discover bool) tea.Cmd {
	m.ensurePage()
	if m.intentContext == nil || m.intentContext.Err() != nil {
		m.intentContext = m.navigation.Start(m.Context())
	}
	client, detail, selectedKey, filter := m.client, m.detail, m.limiterKey, m.filter
	page := m.pages[m.pageIndex]
	keys := make([]string, len(page.page.Items))
	for i, record := range page.page.Items {
		keys[i] = record.Key
	}
	m.loading, m.err = true, nil
	ctx := m.request.Start(devtools.WithTracker(m.Context(), "limiters.fetch"))
	m.readContext = ctx
	return readCmd(ctx, m, func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		readContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var data limiterDataMsg
		var err error
		switch {
		case detail:
			data.limiter, err = client.GetLimiter(readContext, selectedKey)
		case discover || len(keys) == 0:
			data.page, err = client.ScanLimiters(readContext, page.start, filter)
		default:
			data.page = page.page
			data.page.Items, err = client.RefreshLimiters(readContext, keys)
		}
		if err != nil {
			return ConnectionErrorMsg{Err: err}
		}
		return data
	})
}

func (m *RateLimits) resetDiscovery() tea.Cmd {
	m.CancelRequests()
	m.pages, m.pageIndex, m.ready = nil, 0, false
	m.table.SetRows(nil)
	return m.fetch(true)
}

func (m *RateLimits) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "r":
		return m.fetch(false)
	case "ctrl+r":
		if !m.detail {
			return m.resetDiscovery()
		}
		return m.fetch(false)
	case "/":
		if !m.detail {
			return m.OpenDialog(filterdialog.New(filterdialog.WithStyles(filterDialogStylesFromTheme(m.styles)), filterdialog.WithQuery(m.filter)))
		}
		return nil
	case "ctrl+u":
		if !m.detail && m.filter != "" {
			m.filter = ""
			return m.resetDiscovery()
		}
		return nil
	case "c", "C":
		if record := m.selectedLimiter(); record != nil {
			value := record.Name
			if msg.String() == "C" {
				value = record.Key
			}
			return copyTextCmd(value)
		}
		return nil
	case "enter":
		if !m.detail {
			if record := m.selectedLimiter(); record != nil {
				return m.EmitIntent(ShowRateLimitDetailMsg{Key: record.Key, Owner: m, context: m.intentContext})
			}
		}
		return nil
	case "tab", "shift+tab":
		if m.hasHistory() {
			m.focusHistory = !m.focusHistory
		}
		return nil
	case "[", "]":
		return m.changePage(msg.String())
	}
	if m.focusHistory && m.hasHistory() {
		m.history, _ = m.history.Update(msg)
	} else {
		m.table, _ = m.table.Update(msg)
	}
	return nil
}

func (m *RateLimits) changePage(direction string) tea.Cmd {
	if m.detail || !m.ready {
		return nil
	}
	m.ensurePage()
	m.rememberSelection()
	if direction == "[" {
		if m.pageIndex == 0 {
			return nil
		}
		m.CancelRequests()
		m.pageIndex--
		m.buildRows()
		return m.fetch(false)
	}
	if m.loading {
		return nil
	}
	if m.pageIndex+1 < len(m.pages) {
		m.CancelRequests()
		m.pageIndex++
		m.buildRows()
		return m.fetch(false)
	}
	page := m.pages[m.pageIndex].page
	if !page.More {
		return nil
	}
	m.CancelRequests()
	m.pages = append(m.pages, limiterPageState{start: page.Next})
	m.pageIndex++
	m.buildRows()
	return m.fetch(true)
}

func (m *RateLimits) rememberSelection() {
	if !m.detail && len(m.pages) > m.pageIndex && m.table.SelectedRow().ID != "" {
		m.pages[m.pageIndex].selected = m.table.SelectedRow().ID
	}
}

func (m *RateLimits) selectedLimiter() *sidekiq.Limiter {
	if m.detail {
		return m.limiter
	}
	if len(m.pages) == 0 {
		return nil
	}
	items := m.pages[m.pageIndex].page.Items
	index := slices.IndexFunc(items, func(item sidekiq.Limiter) bool { return item.Key == m.table.SelectedRow().ID })
	if index < 0 {
		return nil
	}
	return &items[index]
}

func (m *RateLimits) buildRows() {
	var rows []table.Row
	if m.detail {
		rows = limiterPropertyRows(m.limiter)
		m.history.SetRows(limiterHistoryRows(m.limiter))
		if !m.hasHistory() {
			m.focusHistory = false
		}
	} else if len(m.pages) > 0 {
		for _, record := range m.pages[m.pageIndex].page.Items {
			rows = append(rows, table.Row{ID: record.Key, Cells: []string{record.Name, record.Type, record.Rate(), record.UsageNote()}})
		}
	}
	empty := "No rate limiters"
	if m.filter != "" {
		empty = "No matching limiter names"
	}
	if !m.detail && len(m.pages) > 0 && m.pages[m.pageIndex].page.More {
		empty = "No matches in this scan window; press ] to continue"
	}
	m.table.SetEmptyMessage(empty)
	m.table.SetRows(rows)
	if !m.detail && len(m.pages) > 0 {
		selected := m.pages[m.pageIndex].selected
		if index := slices.IndexFunc(rows, func(row table.Row) bool { return row.ID == selected }); index >= 0 {
			m.table.SetCursor(index)
		}
	}
	m.SetSize(m.width, m.height)
}

func (m *RateLimits) hasHistory() bool {
	return m.detail && m.limiter != nil && len(m.limiter.History) > 0
}

func (m *RateLimits) renderFrame(title, content, meta, filter string, width, height int, focused bool) string {
	if width < 4 || height < 2 {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", max(width, 0))+"\n", max(height, 0)), "\n")
	}
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), height-2)]
	for i, line := range lines {
		lines[i] = display.HorizontalScroll(line, 0, width-4)
	}
	return frame.New(frame.WithStyles(frameStylesFromTheme(m.styles)), frame.WithTitle(title), frame.WithFilter(filter), frame.WithMeta(meta),
		frame.WithTitlePadding(0), frame.WithPadding(1), frame.WithContent(strings.Join(lines, "\n")), frame.WithSize(width, height), frame.WithFocused(focused)).View()
}
