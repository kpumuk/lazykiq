// Package lazytable provides a lazily-loaded table with windowed data fetching.
package lazytable

import (
	"context"
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

// CursorIntent controls how the cursor should be positioned after loading.
type CursorIntent int

const (
	// CursorKeep keeps the cursor anchored to its current row.
	CursorKeep CursorIntent = iota
	// CursorStart moves the cursor to the first row.
	CursorStart
	// CursorEnd moves the cursor to the last row.
	CursorEnd
)

// FetchResult represents a loaded window of rows.
type FetchResult struct {
	Rows        []table.Row
	Total       int64
	WindowStart int
	Payload     any
}

// DataMsg carries a fetch result to the component.
type DataMsg struct {
	RequestID int
	Result    FetchResult
}

// Fetcher loads a window of rows.
type Fetcher func(ctx context.Context, start, size int, intent CursorIntent) (FetchResult, error)

// ErrorHandler converts fetch errors into tea messages.
type ErrorHandler func(error) tea.Msg

// Model is a lazily-loaded table component.
type Model struct {
	table            table.Model
	fetcher          Fetcher
	errorHandler     ErrorHandler
	spinner          spinner.Model
	windowPages      int
	fallbackPageSize int
	pageSize         int
	windowSize       int
	windowStart      int
	totalSize        int64
	loading          bool
	requestID        int
	pendingIntent    CursorIntent
	anchor           anchorState
}

type anchorState struct {
	abs          int
	screenOffset int
	pending      bool
	rowID        string
}

// Option configures the lazy table.
type Option func(*Model)

// New creates a new lazy table.
func New(opts ...Option) Model {
	m := Model{
		table:            table.New(),
		spinner:          spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		windowPages:      3,
		fallbackPageSize: 25,
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.ensurePagingDefaults()
	return m
}

// WithTableOptions sets table options.
func WithTableOptions(opts ...table.Option) Option {
	return func(m *Model) {
		m.table = table.New(opts...)
	}
}

// WithFetcher sets the fetcher used by the lazy table.
func WithFetcher(fetcher Fetcher) Option {
	return func(m *Model) {
		m.fetcher = fetcher
	}
}

// WithErrorHandler sets the error handler for fetch failures.
func WithErrorHandler(handler ErrorHandler) Option {
	return func(m *Model) {
		m.errorHandler = handler
	}
}

// WithWindowPages sets how many pages to keep in memory.
func WithWindowPages(pages int) Option {
	return func(m *Model) {
		m.windowPages = pages
	}
}

// WithFallbackPageSize sets the fallback page size.
func WithFallbackPageSize(size int) Option {
	return func(m *Model) {
		m.fallbackPageSize = size
	}
}

// SetFetcher updates the fetcher.
func (m *Model) SetFetcher(fetcher Fetcher) {
	m.fetcher = fetcher
}

// SetErrorHandler updates the error handler.
func (m *Model) SetErrorHandler(handler ErrorHandler) {
	m.errorHandler = handler
}

// SetSize updates the table dimensions.
func (m *Model) SetSize(width, height int) {
	m.table.SetSize(width, height)
	m.updatePaging()
	m.syncScrollbar()
}

// SetTableStyles updates table styles.
func (m *Model) SetTableStyles(styles table.Styles) {
	m.table.SetStyles(styles)
}

// SetSpinnerStyle updates the spinner style.
func (m *Model) SetSpinnerStyle(style lipgloss.Style) {
	m.spinner.Style = style
}

// SetEmptyMessage updates the table empty message.
func (m *Model) SetEmptyMessage(msg string) {
	m.table.SetEmptyMessage(msg)
}

// Reset clears the window state and table rows.
func (m *Model) Reset() {
	m.ensurePagingDefaults()
	m.windowStart = 0
	m.totalSize = 0
	m.loading = false
	m.requestID = 0
	m.anchor = anchorState{}
	m.table.SetRows(nil)
	m.table.SetCursor(0)
	m.table.ClearScrollbar()
}

// RequestWindow starts a fetch for the given window start.
func (m *Model) RequestWindow(windowStart int, intent CursorIntent) tea.Cmd {
	if m.fetcher == nil {
		return nil
	}
	windowStart = max(windowStart, 0)
	m.pendingIntent = intent
	if intent == CursorKeep {
		m.captureAnchor()
	} else {
		m.anchor.pending = false
	}
	m.loading = true
	m.requestID++
	requestID := m.requestID
	windowSize := m.effectiveWindowSize()

	return tea.Batch(
		m.spinner.Tick,
		m.fetchCmd(requestID, windowStart, windowSize, intent),
	)
}

// Update handles lazy table messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case DataMsg:
		if msg.RequestID != m.requestID {
			return m, nil
		}
		m.loading = false
		m.windowStart = msg.Result.WindowStart
		m.totalSize = msg.Result.Total
		m.table.SetRows(msg.Result.Rows)
		switch m.pendingIntent {
		case CursorKeep:
			m.applyAnchor()
		case CursorStart:
			if len(msg.Result.Rows) > 0 {
				m.table.SetCursor(0)
			}
		case CursorEnd:
			if len(msg.Result.Rows) > 0 {
				m.table.SetCursor(len(msg.Result.Rows) - 1)
			}
		}
		m.syncScrollbar()
		return m, m.maybePrefetch()

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if handled, cmd := m.handleJump(msg); handled {
			return m, cmd
		}
		m.table, _ = m.table.Update(msg)
		m.syncScrollbar()
		return m, m.maybePrefetch()
	}

	return m, nil
}

// View renders the table with a loading spinner in the scrollbar header.
func (m Model) View() string {
	spinnerView := ""
	if m.loading {
		spinnerView = m.spinner.View()
	}
	m.table.SetScrollbarHeader([]string{" ", spinnerView})
	return m.table.View()
}

// MovePage scrolls by one page.
func (m *Model) MovePage(delta int) {
	step := max(m.pageSize, 1)
	if delta < 0 {
		m.table.MoveUp(step)
	} else if delta > 0 {
		m.table.MoveDown(step)
	}
	m.syncScrollbar()
}

// GotoTop moves the selection to the top of the table.
func (m *Model) GotoTop() {
	m.table.GotoTop()
	m.syncScrollbar()
}

// GotoBottom moves the selection to the bottom of the table.
func (m *Model) GotoBottom() {
	m.table.GotoBottom()
	m.syncScrollbar()
}

// MaybePrefetch triggers prefetch when nearing the window edge.
func (m *Model) MaybePrefetch() tea.Cmd {
	return m.maybePrefetch()
}

// Table returns the underlying table model.
func (m *Model) Table() *table.Model {
	return &m.table
}

// Loading reports whether the table is loading data.
func (m Model) Loading() bool {
	return m.loading
}

// RequestID returns the current request ID.
func (m Model) RequestID() int {
	return m.requestID
}

// WindowStart returns the current window start offset.
func (m Model) WindowStart() int {
	return m.windowStart
}

// WindowSize returns the current window size.
func (m Model) WindowSize() int {
	return m.windowSize
}

// Total returns the total item count.
func (m Model) Total() int64 {
	return m.totalSize
}

// Range returns the visible range and total count.
func (m Model) Range() (int, int, int64) {
	rows := m.table.Rows()
	if m.totalSize == 0 || len(rows) == 0 {
		return 0, 0, m.totalSize
	}

	yOffset := m.table.YOffset()
	viewport := m.table.ViewportHeight()
	start := max(m.windowStart+yOffset+1, 0)
	end := max(m.windowStart+min(yOffset+viewport, len(rows)), 0)
	if start > end {
		start = end
	}
	return start, end, m.totalSize
}

func (m *Model) handleJump(msg tea.KeyMsg) (bool, tea.Cmd) {
	if key.Matches(msg, m.table.KeyMap.GotoTop) {
		return true, m.jumpToStart()
	}
	if key.Matches(msg, m.table.KeyMap.GotoBottom) {
		return true, m.jumpToEnd()
	}
	return false, nil
}

func (m *Model) jumpToStart() tea.Cmd {
	if m.totalSize == 0 || m.windowStart == 0 {
		return m.gotoTopLocal()
	}
	return m.RequestWindow(0, CursorStart)
}

func (m *Model) jumpToEnd() tea.Cmd {
	rows := m.table.Rows()
	if m.totalSize == 0 || len(rows) == 0 {
		return m.gotoBottomLocal()
	}

	windowSize := m.effectiveWindowSize()
	maxStart := max(int(m.totalSize)-windowSize, 0)
	if m.windowStart == maxStart {
		return m.gotoBottomLocal()
	}
	return m.RequestWindow(maxStart, CursorEnd)
}

func (m *Model) gotoTopLocal() tea.Cmd {
	m.table.GotoTop()
	m.syncScrollbar()
	return nil
}

func (m *Model) gotoBottomLocal() tea.Cmd {
	m.table.GotoBottom()
	m.syncScrollbar()
	return nil
}

func (m Model) effectiveWindowSize() int {
	if m.windowSize > 0 {
		return m.windowSize
	}
	return max(m.pageSize, m.fallbackPageSize) * max(m.windowPages, 1)
}

func (m *Model) fetchCmd(requestID, windowStart, windowSize int, intent CursorIntent) tea.Cmd {
	fetcher := m.fetcher
	handler := m.errorHandler
	return func() tea.Msg {
		if fetcher == nil {
			return nil
		}
		result, err := fetcher(context.Background(), windowStart, windowSize, intent)
		if err != nil {
			if handler != nil {
				return handler(err)
			}
			return nil
		}
		return DataMsg{RequestID: requestID, Result: result}
	}
}

func (m *Model) ensurePagingDefaults() {
	if m.pageSize <= 0 {
		m.pageSize = m.fallbackPageSize
	}
	if m.pageSize <= 0 {
		m.pageSize = 1
	}
	if m.windowPages <= 0 {
		m.windowPages = 1
	}
	m.windowSize = m.pageSize * m.windowPages
}

func (m *Model) updatePaging() {
	pageSize := max(m.table.ViewportHeight(), 1)
	if pageSize == m.pageSize {
		return
	}
	m.pageSize = pageSize
	m.windowSize = pageSize * max(m.windowPages, 1)
}

func (m *Model) captureAnchor() {
	rows := m.table.Rows()
	if len(rows) == 0 {
		m.anchor.pending = false
		return
	}
	cursor := m.table.Cursor()
	m.anchor = anchorState{
		abs:          m.windowStart + cursor,
		screenOffset: cursor - m.table.YOffset(),
		pending:      true,
	}
	if cursor >= 0 && cursor < len(rows) {
		m.anchor.rowID = rows[cursor].ID
	}
}

func (m *Model) applyAnchor() {
	if !m.anchor.pending {
		return
	}
	m.anchor.pending = false
	rows := m.table.Rows()
	if len(rows) == 0 {
		return
	}
	rel := m.anchor.abs - m.windowStart
	if m.anchor.rowID != "" {
		if idx := slices.IndexFunc(rows, func(row table.Row) bool {
			return row.ID == m.anchor.rowID
		}); idx >= 0 {
			rel = idx
		}
	}
	rel = mathutil.Clamp(rel, 0, len(rows)-1)
	m.table.SetCursor(rel)

	offset := mathutil.Clamp(m.anchor.screenOffset, 0, max(m.table.ViewportHeight()-1, 0))
	m.table.SetYOffset(rel - offset)
}

func (m *Model) maybePrefetch() tea.Cmd {
	if m.loading || m.fetcher == nil || m.totalSize == 0 {
		return nil
	}
	rows := m.table.Rows()
	if len(rows) == 0 {
		return nil
	}
	if int64(len(rows)) >= m.totalSize {
		return nil
	}

	preload := max(3, min(10, m.pageSize/3))
	windowEnd := m.windowStart + len(rows)
	cursorAbs := m.windowStart + m.table.Cursor()
	maxStart := max(int(m.totalSize)-m.windowSize, 0)

	if cursorAbs >= windowEnd-preload && windowEnd < int(m.totalSize) {
		nextStart := min(m.windowStart+m.pageSize, maxStart)
		if nextStart != m.windowStart {
			return m.RequestWindow(nextStart, CursorKeep)
		}
	}

	if cursorAbs <= m.windowStart+preload && m.windowStart > 0 {
		prevStart := max(m.windowStart-m.pageSize, 0)
		if prevStart != m.windowStart {
			return m.RequestWindow(prevStart, CursorKeep)
		}
	}

	return nil
}

func (m *Model) syncScrollbar() {
	rows := m.table.Rows()
	if m.totalSize <= 0 {
		m.table.ClearScrollbar()
		return
	}
	total := int(m.totalSize)
	if m.windowStart == 0 && total <= len(rows) {
		m.table.ClearScrollbar()
		return
	}
	m.table.SetScrollbar(total, m.windowStart+m.table.YOffset())
}
