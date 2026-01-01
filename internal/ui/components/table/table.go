// Package table provides a scrollable, selectable table component.
package table

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Row represents one line in the table. ID is used to preserve selection across refreshes.
type Row struct {
	ID    string
	Cells []string
}

// SelectionSpan defines the visible selection range for a row.
// End is exclusive; End < 0 means "to end of row".
type SelectionSpan struct {
	Start int
	End   int
}

// Column defines a table column.
type Column struct {
	Title string
	Width int
}

// KeyMap defines keybindings for the table.
type KeyMap struct {
	LineUp      key.Binding
	LineDown    key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	GotoTop     key.Binding
	GotoBottom  key.Binding
	ScrollLeft  key.Binding
	ScrollRight key.Binding
	Home        key.Binding
	End         key.Binding
}

// DefaultKeyMap returns a default set of keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		LineUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		LineDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "go to start"),
		),
		GotoBottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to end"),
		),
		ScrollLeft: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "scroll left"),
		),
		ScrollRight: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "scroll right"),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "0"),
			key.WithHelp("home/0", "scroll to start"),
		),
		End: key.NewBinding(
			key.WithKeys("end", "$"),
			key.WithHelp("end/$", "scroll to end"),
		),
	}
}

// Styles holds the styles needed by the table.
type Styles struct {
	Text      lipgloss.Style
	Muted     lipgloss.Style
	Header    lipgloss.Style
	Selected  lipgloss.Style
	Separator lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for this table.
func DefaultStyles() Styles {
	return Styles{
		Text:      lipgloss.NewStyle(),
		Muted:     lipgloss.NewStyle().Faint(true),
		Header:    lipgloss.NewStyle().Bold(true),
		Selected:  lipgloss.NewStyle().Reverse(true),
		Separator: lipgloss.NewStyle().Faint(true),
	}
}

// Model is a scrollable table component with selection support.
type Model struct {
	KeyMap KeyMap

	columns        []Column
	rows           []Row
	styles         Styles
	width          int
	height         int
	cursor         int
	yOffset        int
	xOffset        int
	maxRowWidth    int
	colWidths      []int // dynamic column widths (max of defined and actual)
	lastColWidth   int
	emptyMessage   string
	content        string // pre-rendered body content
	viewportHeight int
	fullRows       map[int]string // row index -> full-width content
	selectionSpans map[int]SelectionSpan
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new model for the table widget.
func New(opts ...Option) Model {
	m := Model{
		KeyMap:       DefaultKeyMap(),
		styles:       DefaultStyles(),
		emptyMessage: "No data",
	}

	for _, opt := range opts {
		opt(&m)
	}

	m.updateViewport()

	return m
}

// WithColumns sets the table columns (headers).
func WithColumns(cols []Column) Option {
	return func(m *Model) {
		m.columns = cols
	}
}

// WithRows sets the table rows (data).
func WithRows(rows []Row) Option {
	return func(m *Model) {
		m.rows = rows
	}
}

// WithStyles sets the table styles.
func WithStyles(s Styles) Option {
	return func(m *Model) {
		m.styles = s
	}
}

// WithKeyMap sets the key map.
func WithKeyMap(km KeyMap) Option {
	return func(m *Model) {
		m.KeyMap = km
	}
}

// WithWidth sets the width of the table.
func WithWidth(w int) Option {
	return func(m *Model) {
		m.width = w
	}
}

// WithHeight sets the height of the table.
func WithHeight(h int) Option {
	return func(m *Model) {
		m.height = h
		m.viewportHeight = max(h-2, 1) // minus header and separator
	}
}

// WithEmptyMessage sets the message shown when there are no rows.
func WithEmptyMessage(msg string) Option {
	return func(m *Model) {
		m.emptyMessage = msg
	}
}

// SetStyles sets the table styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.updateViewport()
}

// SetSize sets the table dimensions.
func (m *Model) SetSize(width, height int) {
	if m.width == width && m.height == height {
		return
	}
	m.width = width
	m.height = height
	m.viewportHeight = max(height-2, 1) // minus header and separator
	m.updateViewport()
	m.clampScroll()
}

// SetRows sets a new rows state.
func (m *Model) SetRows(rows []Row) {
	m.SetRowsWithMeta(rows, nil, nil)
}

// SetRowsWithMeta sets rows with optional full-row overrides and selection spans.
func (m *Model) SetRowsWithMeta(rows []Row, fullRows map[int]string, spans map[int]SelectionSpan) {
	selectedID := ""
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		selectedID = m.rows[m.cursor].ID
	}

	m.rows = rows
	m.fullRows = fullRows
	m.selectionSpans = spans

	if len(m.rows) == 0 {
		m.cursor = 0
	} else if selectedID != "" {
		if m.cursor < 0 || m.cursor >= len(m.rows) || m.rows[m.cursor].ID != selectedID {
			if idx := indexRowByID(m.rows, selectedID); idx >= 0 {
				m.cursor = idx
			}
		}
	}

	// Keep selection in bounds
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureSelectedVisible()
	m.updateViewport()
	m.clampScroll()
}

// SetColumns sets a new columns state.
func (m *Model) SetColumns(cols []Column) {
	m.columns = cols
	m.updateViewport()
}

// SetEmptyMessage sets the message shown when there are no rows.
func (m *Model) SetEmptyMessage(msg string) {
	m.emptyMessage = msg
}

// SetFullRows sets full-width row content overrides (row index -> content).
func (m *Model) SetFullRows(rows map[int]string) {
	m.fullRows = rows
	m.updateViewport()
	m.clampScroll()
}

// SetSelectionSpans sets selection spans for specific rows.
func (m *Model) SetSelectionSpans(spans map[int]SelectionSpan) {
	m.selectionSpans = spans
	m.updateViewport()
	m.clampScroll()
}

// SetCursor sets the cursor position in the table.
func (m *Model) SetCursor(n int) {
	m.cursor = clamp(n, 0, len(m.rows)-1)
	m.ensureSelectedVisible()
	m.updateViewport()
}

// Cursor returns the index of the selected row.
func (m Model) Cursor() int {
	return m.cursor
}

// SelectedRow returns the currently selected row.
func (m Model) SelectedRow() Row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return Row{}
	}
	return m.rows[m.cursor]
}

// Rows returns the current rows.
func (m Model) Rows() []Row {
	return m.rows
}

// Columns returns the current columns.
func (m Model) Columns() []Column {
	return m.columns
}

// Width returns the table width.
func (m Model) Width() int {
	return m.width
}

// Height returns the table height.
func (m Model) Height() int {
	return m.height
}

// RowCount returns the number of rows.
func (m Model) RowCount() int {
	return len(m.rows)
}

// Update handles key messages for navigation and scrolling.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.LineUp):
			m.MoveUp(1)
		case key.Matches(msg, m.KeyMap.LineDown):
			m.MoveDown(1)
		case key.Matches(msg, m.KeyMap.PageUp):
			m.MoveUp(10)
		case key.Matches(msg, m.KeyMap.PageDown):
			m.MoveDown(10)
		case key.Matches(msg, m.KeyMap.GotoTop):
			m.GotoTop()
		case key.Matches(msg, m.KeyMap.GotoBottom):
			m.GotoBottom()
		case key.Matches(msg, m.KeyMap.ScrollLeft):
			m.ScrollLeft()
		case key.Matches(msg, m.KeyMap.ScrollRight):
			m.ScrollRight()
		case key.Matches(msg, m.KeyMap.Home):
			m.ScrollToStart()
		case key.Matches(msg, m.KeyMap.End):
			m.ScrollToEnd()
		}
	}
	return m, nil
}

// View renders the table (header + visible rows).
func (m Model) View() string {
	header := m.renderHeader()
	body := m.getVisibleContent()
	return header + "\n" + body
}

// MoveUp moves the selection up by n rows.
func (m *Model) MoveUp(n int) {
	m.cursor = clamp(m.cursor-n, 0, len(m.rows)-1)
	m.ensureSelectedVisible()
	m.updateViewport()
}

// MoveDown moves the selection down by n rows.
func (m *Model) MoveDown(n int) {
	m.cursor = clamp(m.cursor+n, 0, len(m.rows)-1)
	m.ensureSelectedVisible()
	m.updateViewport()
}

// GotoTop moves the selection to the first row.
func (m *Model) GotoTop() {
	m.cursor = 0
	m.yOffset = 0
	m.xOffset = 0
	m.updateViewport()
}

// GotoBottom moves the selection to the last row.
func (m *Model) GotoBottom() {
	if len(m.rows) > 0 {
		m.cursor = len(m.rows) - 1
	}
	m.gotoBottomOffset()
	m.updateViewport()
}

// ScrollLeft scrolls content left.
func (m *Model) ScrollLeft() {
	m.xOffset -= 4
	if m.xOffset < 0 {
		m.xOffset = 0
	}
	m.updateViewport()
}

// ScrollRight scrolls content right.
func (m *Model) ScrollRight() {
	maxScroll := m.maxScrollOffset()
	m.xOffset += 4
	if m.xOffset > maxScroll {
		m.xOffset = maxScroll
	}
	m.updateViewport()
}

// ScrollToStart scrolls to the beginning horizontally.
func (m *Model) ScrollToStart() {
	m.xOffset = 0
	m.updateViewport()
}

// ScrollToEnd scrolls to the end horizontally.
func (m *Model) ScrollToEnd() {
	m.xOffset = m.maxScrollOffset()
	m.updateViewport()
}

// maxScrollOffset returns the maximum horizontal scroll offset.
func (m *Model) maxScrollOffset() int {
	maxScroll := m.maxRowWidth - m.width
	if maxScroll < 0 {
		return 0
	}
	return maxScroll
}

// clampScroll ensures scroll offsets are within valid bounds.
func (m *Model) clampScroll() {
	maxX := m.maxScrollOffset()
	if m.xOffset > maxX {
		m.xOffset = maxX
	}
	if m.xOffset < 0 {
		m.xOffset = 0
	}

	maxY := max(len(m.rows)-m.viewportHeight, 0)
	if m.yOffset > maxY {
		m.yOffset = maxY
	}
	if m.yOffset < 0 {
		m.yOffset = 0
	}
}

// ensureSelectedVisible scrolls to keep selected row visible.
func (m *Model) ensureSelectedVisible() {
	if m.cursor < m.yOffset {
		m.yOffset = m.cursor
	} else if m.cursor >= m.yOffset+m.viewportHeight {
		m.yOffset = m.cursor - m.viewportHeight + 1
	}
}

// gotoBottomOffset scrolls to show the last rows.
func (m *Model) gotoBottomOffset() {
	maxOffset := max(len(m.rows)-m.viewportHeight, 0)
	m.yOffset = maxOffset
}

// updateViewport rebuilds the pre-rendered body content.
func (m *Model) updateViewport() {
	m.content = m.renderBody()
}

// renderHeader renders the table header with separator.
func (m Model) renderHeader() string {
	var cols []string
	lastCol := len(m.columns) - 1
	for i, col := range m.columns {
		// Use dynamic column width if available, otherwise use defined width
		width := col.Width
		if m.colWidths != nil && i < len(m.colWidths) {
			width = m.colWidths[i]
		}

		if i < lastCol {
			cols = append(cols, padRight(col.Title, width))
		} else {
			// Last column: stretch to fill available width when shorter
			lastWidth := width
			if m.lastColWidth > 0 {
				lastWidth = m.lastColWidth
			}
			cols = append(cols, padRight(col.Title, lastWidth))
		}
	}
	header := strings.Join(cols, " ")

	// Use maxRowWidth for consistent scrolling with body
	totalWidth := m.maxRowWidth
	if totalWidth == 0 {
		for _, col := range m.columns {
			totalWidth += col.Width + 1
		}
	}
	headerWidth := lipgloss.Width(header)
	if headerWidth > totalWidth {
		totalWidth = headerWidth
	}

	// Pad header to match body width
	if headerWidth < totalWidth {
		header += strings.Repeat(" ", totalWidth-headerWidth)
	}

	// Apply horizontal scroll to header
	header = applyHorizontalScroll(header, m.xOffset, m.width)
	styledHeader := m.styles.Header.Render(header)

	// Separator line (also scrolled)
	separator := strings.Repeat("─", totalWidth)
	separator = applyHorizontalScroll(separator, m.xOffset, m.width)

	return styledHeader + "\n" + m.styles.Separator.Render(separator)
}

// renderBody renders all table rows (for scrolling).
func (m *Model) renderBody() string {
	if len(m.rows) == 0 {
		m.maxRowWidth = 0
		m.colWidths = nil
		m.lastColWidth = m.computeLastColWidth(nil)
		return m.styles.Muted.Render(m.emptyMessage)
	}

	// First pass: find max width for each column (at least the defined width)
	m.colWidths = make([]int, len(m.columns))
	lastCol := len(m.columns) - 1
	for i, col := range m.columns {
		if i < lastCol {
			m.colWidths[i] = col.Width
		}
	}
	for i, row := range m.rows {
		if m.fullRows != nil {
			if _, ok := m.fullRows[i]; ok {
				continue
			}
		}
		for i, cell := range row.Cells {
			cellWidth := lipgloss.Width(cell)
			if i < len(m.colWidths) && cellWidth > m.colWidths[i] {
				m.colWidths[i] = cellWidth
			}
		}
	}

	m.lastColWidth = m.computeLastColWidth(m.colWidths)

	// Second pass: build all rows using actual column widths (no truncation)
	rawRows := make([]string, 0, len(m.rows))
	maxWidth := 0
	for i, row := range m.rows {
		if m.fullRows != nil {
			if fullRow, ok := m.fullRows[i]; ok {
				rawRows = append(rawRows, fullRow)
				rowWidth := lipgloss.Width(fullRow)
				if rowWidth > maxWidth {
					maxWidth = rowWidth
				}
				continue
			}
		}
		var cols []string
		for i, cell := range row.Cells {
			if i < lastCol {
				cols = append(cols, padRight(cell, m.colWidths[i]))
			} else {
				// Last column: stretch to fill remaining width when needed
				cols = append(cols, padRight(cell, m.lastColWidth))
			}
		}
		rowStr := strings.Join(cols, " ")
		rawRows = append(rawRows, rowStr)

		rowWidth := lipgloss.Width(rowStr)
		if rowWidth > maxWidth {
			maxWidth = rowWidth
		}
	}
	m.maxRowWidth = maxWidth

	// Third pass: apply scroll and styling
	lines := make([]string, 0, len(rawRows))
	for i, row := range rawRows {
		isFullRow := false
		if m.fullRows != nil {
			_, isFullRow = m.fullRows[i]
		}
		span, hasSpan := SelectionSpan{}, false
		if m.selectionSpans != nil {
			span, hasSpan = m.selectionSpans[i]
		}

		// Pad row to max width for consistent selection highlight
		rowWidth := lipgloss.Width(row)
		if rowWidth < maxWidth {
			row += strings.Repeat(" ", maxWidth-rowWidth)
		}

		// Apply horizontal scroll offset (before styling)
		row = applyHorizontalScroll(row, m.xOffset, m.width)

		// Apply selection highlight
		if i == m.cursor {
			if hasSpan {
				row = applySelection(row, span, maxWidth, m.xOffset, m.width, m.styles.Selected)
			} else {
				row = m.styles.Selected.Render(row)
			}
		} else if !isFullRow {
			row = m.styles.Text.Render(row)
		}

		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

// getVisibleContent returns the visible portion of content based on yOffset.
func (m Model) getVisibleContent() string {
	if m.content == "" {
		return ""
	}

	lines := strings.Split(m.content, "\n")

	// Clamp yOffset to valid range
	yOffset := max(m.yOffset, 0)
	if yOffset >= len(lines) {
		yOffset = max(len(lines)-1, 0)
	}

	// Get visible slice
	end := min(yOffset+m.viewportHeight, len(lines))

	return strings.Join(lines[yOffset:end], "\n")
}

// applyHorizontalScroll applies horizontal scroll offset to a plain text line.
func applyHorizontalScroll(line string, offset, visibleWidth int) string {
	if visibleWidth <= 0 {
		return ""
	}
	if offset < 0 {
		offset = 0
	}

	cut := ansi.Cut(line, offset, offset+visibleWidth)
	cutWidth := lipgloss.Width(cut)
	if cutWidth < visibleWidth {
		cut += strings.Repeat(" ", visibleWidth-cutWidth)
	}
	return cut
}

func applySelection(line string, span SelectionSpan, maxWidth, xOffset, visibleWidth int, style lipgloss.Style) string {
	lineWidth := lipgloss.Width(line)
	if lineWidth == 0 {
		return line
	}

	start := span.Start
	end := span.End
	if start < 0 {
		start = 0
	}
	if end < 0 || end > maxWidth {
		end = maxWidth
	}
	start -= xOffset
	end -= xOffset
	if start < 0 {
		start = 0
	}
	if end > visibleWidth {
		end = visibleWidth
	}
	if start >= end || start >= lineWidth {
		return line
	}

	prefix := ansi.Cut(line, 0, start)
	mid := ansi.Cut(line, start, end)
	suffix := ansi.Cut(line, end, lineWidth)

	mid = ansi.Strip(mid)

	return prefix + style.Render(mid) + suffix
}

func (m Model) computeLastColWidth(colWidths []int) int {
	if len(m.columns) == 0 {
		return 0
	}

	lastCol := len(m.columns) - 1
	lastWidth := 0
	if colWidths != nil && lastCol < len(colWidths) {
		lastWidth = colWidths[lastCol]
	}

	fixedWidth := 0
	for i := range lastCol {
		width := m.columns[i].Width
		if colWidths != nil && i < len(colWidths) {
			width = colWidths[i]
		}
		fixedWidth += width
	}
	if len(m.columns) > 1 {
		fixedWidth += lastCol // spaces between columns
	}

	if m.width > 0 {
		remaining := m.width - fixedWidth
		if remaining > lastWidth {
			lastWidth = remaining
		}
	}

	return lastWidth
}

// padRight pads a string to the specified width (no truncation).
func padRight(s string, width int) string {
	if width <= 0 {
		return s
	}
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

// clamp restricts a value to a range.
func clamp(v, low, high int) int {
	if high < low {
		return low
	}
	return min(max(v, low), high)
}

func indexRowByID(rows []Row, id string) int {
	for i, row := range rows {
		if row.ID == id {
			return i
		}
	}
	return -1
}
