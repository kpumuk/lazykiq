// Package table provides a scrollable, selectable table component.
package table

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/ui/components/scrollbar"
	"github.com/kpumuk/lazykiq/internal/ui/display"
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

// Alignment defines how cell content should be aligned.
type Alignment int

const (
	// AlignLeft aligns content to the left (default).
	AlignLeft Alignment = iota
	// AlignRight aligns content to the right.
	AlignRight
	// AlignCenter centers content.
	AlignCenter
)

// Column defines a table column.
type Column struct {
	Title string
	Width int
	Align Alignment
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
	Text           lipgloss.Style
	Muted          lipgloss.Style
	Header         lipgloss.Style
	Selected       lipgloss.Style
	Separator      lipgloss.Style
	ScrollbarTrack lipgloss.Style
	ScrollbarThumb lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for this table.
func DefaultStyles() Styles {
	return Styles{
		Text:           lipgloss.NewStyle(),
		Muted:          lipgloss.NewStyle().Faint(true),
		Header:         lipgloss.NewStyle().Bold(true),
		Selected:       lipgloss.NewStyle().Reverse(true),
		Separator:      lipgloss.NewStyle().Faint(true),
		ScrollbarTrack: lipgloss.NewStyle(),
		ScrollbarThumb: lipgloss.NewStyle(),
	}
}

// Model is a scrollable table component with selection support.
type Model struct {
	KeyMap KeyMap

	columns           []Column
	rows              []Row
	styles            Styles
	width             int
	height            int
	cursor            int
	yOffset           int
	xOffset           int
	maxRowWidth       int
	colWidths         []int // dynamic column widths (max of defined and actual)
	lastColWidth      int
	emptyMessage      string
	content           []string // pre-rendered visible body rows
	header            string   // pre-rendered header and separator
	rowContent        []string // clipped, unstyled visible rows
	rowContentX       int
	rowContentY       int
	rowContentWidth   int
	layoutValid       bool
	viewportHeight    int
	fullRows          map[int]string // row index -> full-width content
	selectionSpans    map[int]SelectionSpan
	scrollbar         scrollbar.Model
	scrollbarOverride bool
	scrollbarTotal    int
	scrollbarOffset   int
	scrollbarHeader   []string
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
	m.updateScrollbar()

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
	m.header = ""
	m.updateViewport()
	m.updateScrollbar()
}

// SetSize sets the table dimensions.
func (m *Model) SetSize(width, height int) {
	if m.width == width && m.height == height {
		return
	}
	if m.width != width {
		m.layoutValid = false
	}
	m.width = width
	m.height = height
	m.viewportHeight = max(height-2, 1) // minus header and separator
	m.updateViewport()
	m.updateScrollbar()
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
	m.rowContent = nil
	m.layoutValid = false
	m.fullRows = fullRows
	m.selectionSpans = spans

	if len(m.rows) == 0 {
		m.cursor = 0
		m.xOffset = 0
		m.yOffset = 0
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
	m.rowContent = nil
	m.layoutValid = false
	m.colWidths = nil
	m.lastColWidth = 0
	m.updateViewport()
}

// SetEmptyMessage sets the message shown when there are no rows.
func (m *Model) SetEmptyMessage(msg string) {
	m.emptyMessage = msg
}

// SetFullRows sets full-width row content overrides (row index -> content).
func (m *Model) SetFullRows(rows map[int]string) {
	m.fullRows = rows
	m.rowContent = nil
	m.layoutValid = false
	m.updateViewport()
	m.clampScroll()
}

// SetSelectionSpans sets selection spans for specific rows.
func (m *Model) SetSelectionSpans(spans map[int]SelectionSpan) {
	m.selectionSpans = spans
	m.updateViewport()
	m.clampScroll()
}

// SetScrollbarHeader sets scrollbar header lines (one per header line).
func (m *Model) SetScrollbarHeader(lines []string) {
	m.scrollbarHeader = lines
}

// SetScrollbar overrides the scrollbar total and offset values.
func (m *Model) SetScrollbar(total, offset int) {
	m.scrollbarOverride = true
	m.scrollbarTotal = total
	m.scrollbarOffset = offset
}

// ClearScrollbar clears the scrollbar override, using row-based values instead.
func (m *Model) ClearScrollbar() {
	m.scrollbarOverride = false
}

// SetCursor sets the cursor position in the table.
func (m *Model) SetCursor(n int) {
	m.cursor = mathutil.Clamp(n, 0, len(m.rows)-1)
	m.ensureSelectedVisible()
	m.updateViewport()
}

// SetYOffset sets the vertical scroll offset.
func (m *Model) SetYOffset(offset int) {
	maxY := max(len(m.rows)-m.viewportHeight, 0)
	m.yOffset = mathutil.Clamp(offset, 0, maxY)
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

// ViewportHeight returns the visible row count.
func (m Model) ViewportHeight() int {
	return m.viewportHeight
}

// YOffset returns the top visible row offset.
func (m Model) YOffset() int {
	return m.yOffset
}

// RowCount returns the number of rows.
func (m Model) RowCount() int {
	return len(m.rows)
}

// Update handles key messages for navigation and scrolling.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
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
	headerLines := strings.Split(m.header, "\n")
	bodyLines := m.getVisibleContent()

	if m.scrollbarWidth() > 0 {
		headerLines = appendScrollbar(headerLines, m.headerScrollbarLines(len(headerLines)))
		bodyLines = appendScrollbar(bodyLines, m.scrollbarLines())
	}

	headerLines = append(headerLines, bodyLines...)
	return strings.Join(headerLines, "\n")
}

// MoveUp moves the selection up by n rows.
func (m *Model) MoveUp(n int) {
	m.moveCursor(-n)
}

// MoveDown moves the selection down by n rows.
func (m *Model) MoveDown(n int) {
	m.moveCursor(n)
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
	m.scrollX(-4)
}

// ScrollRight scrolls content right.
func (m *Model) ScrollRight() {
	m.scrollX(4)
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
	return max(m.maxRowWidth-m.contentWidth(), 0)
}

// clampScroll ensures scroll offsets are within valid bounds.
func (m *Model) clampScroll() {
	maxX := m.maxScrollOffset()
	m.xOffset = mathutil.Clamp(m.xOffset, 0, maxX)

	maxY := max(len(m.rows)-m.viewportHeight, 0)
	m.yOffset = mathutil.Clamp(m.yOffset, 0, maxY)
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

func (m *Model) moveCursor(delta int) {
	m.cursor = mathutil.Clamp(m.cursor+delta, 0, len(m.rows)-1)
	m.ensureSelectedVisible()
	m.updateViewport()
}

func (m *Model) scrollX(delta int) {
	m.xOffset = mathutil.Clamp(m.xOffset+delta, 0, m.maxScrollOffset())
	m.updateViewport()
}

// updateViewport measures changed data and renders only the visible body rows.
func (m *Model) updateViewport() {
	layoutChanged := !m.layoutValid
	if layoutChanged {
		m.measureRows()
		m.layoutValid = true
	}
	m.clampScroll()
	layoutChanged = layoutChanged || m.rowContentX != m.xOffset || m.rowContentWidth != m.contentWidth()
	visibleCount := min(m.viewportHeight, len(m.rows)-m.yOffset)
	if layoutChanged || m.rowContentY != m.yOffset || len(m.rowContent) != visibleCount {
		m.rowContent = m.renderRowContent(!layoutChanged)
		m.rowContentX = m.xOffset
		m.rowContentY = m.yOffset
		m.rowContentWidth = m.contentWidth()
	}
	m.content = m.renderBody()
	if layoutChanged || m.header == "" {
		m.header = m.renderHeader()
	}
}

func (m *Model) updateScrollbar() {
	m.scrollbar.SetStyles(scrollbar.Styles{
		Track: m.styles.ScrollbarTrack,
		Thumb: m.styles.ScrollbarThumb,
	})
	m.scrollbar.SetSize(scrollbar.DefaultWidth, m.viewportHeight)
}

// renderHeader renders the table header with separator.
func (m Model) renderHeader() string {
	line := viewportLine{offset: m.xOffset, width: m.contentWidth()}
	lastCol := len(m.columns) - 1
	for i, col := range m.columns {
		// Use dynamic column width if available, otherwise use defined width
		width := col.Width
		if m.colWidths != nil && i < len(m.colWidths) {
			width = m.colWidths[i]
		}

		if i == lastCol && m.lastColWidth > 0 {
			width = m.lastColWidth
		}
		if i > 0 {
			line.writePadding(1)
		}
		line.writeCell(col.Title, width, col.Align)
	}

	// Use maxRowWidth for consistent scrolling with body
	totalWidth := m.maxRowWidth
	if totalWidth == 0 {
		for _, col := range m.columns {
			totalWidth += col.Width + 1
		}
	}
	totalWidth = max(totalWidth, line.position)
	styledHeader := m.styles.Header.Render(line.String())

	// Materialize only the separator segment intersecting the viewport.
	separatorWidth := mathutil.Clamp(totalWidth-m.xOffset, 0, m.contentWidth())
	separator := strings.Repeat("─", separatorWidth) + strings.Repeat(" ", m.contentWidth()-separatorWidth)

	return styledHeader + "\n" + m.styles.Separator.Render(separator)
}

// renderBody styles cached visible rows without processing offscreen rows.
func (m Model) renderBody() []string {
	if len(m.columns) == 0 || len(m.rows) == 0 {
		empty := m.emptyMessage
		if len(m.columns) > 0 && m.contentWidth() > 0 {
			empty = padCell(empty, m.contentWidth(), AlignLeft)
		}
		empty = m.styles.Muted.Render(empty)
		if empty == "" {
			return nil
		}
		return strings.Split(empty, "\n")
	}

	lines := make([]string, len(m.rowContent))
	for visibleIndex, row := range m.rowContent {
		i := m.yOffset + visibleIndex
		_, isFullRow := m.fullRows[i]
		span, hasSpan := m.selectionSpans[i]
		if i == m.cursor {
			if hasSpan {
				row = applySelection(row, span, m.maxRowWidth, m.xOffset, m.contentWidth(), m.styles.Selected)
			} else {
				row = m.styles.Selected.Render(ansi.Strip(row))
			}
		} else if !isFullRow {
			row = m.styles.Text.Render(row)
		}
		lines[visibleIndex] = row
	}
	return lines
}

// measureRows keeps width metadata independent of the viewport's rendered rows.
func (m *Model) measureRows() {
	if len(m.columns) == 0 {
		m.maxRowWidth = 0
		m.colWidths = nil
		m.lastColWidth = 0
		return
	}

	widths := make([]int, len(m.columns))
	copy(widths, m.colWidths)
	for i, col := range m.columns {
		widths[i] = max(widths[i], col.Width)
	}
	fullRowWidth := 0
	for i, row := range m.rows {
		if fullRow, ok := m.fullRows[i]; ok {
			fullRowWidth = max(fullRowWidth, lipgloss.Width(singleLine(fullRow)))
			continue
		}
		for i, cell := range row.Cells[:min(len(row.Cells), len(widths))] {
			// Byte length bounds the display width after single-line normalization.
			if len(cell) > widths[i] {
				widths[i] = max(widths[i], lipgloss.Width(singleLine(cell)))
			}
		}
	}
	m.colWidths = widths
	m.lastColWidth = m.computeLastColWidth(m.colWidths, m.contentWidth())
	columnWidth := m.columnRowWidth(m.lastColWidth)
	m.maxRowWidth = max(columnWidth, fullRowWidth)
	for i, row := range m.rows {
		if _, ok := m.fullRows[i]; ok || len(row.Cells) <= len(m.columns) {
			continue
		}
		width := columnWidth
		for _, cell := range row.Cells[len(m.columns):] {
			width += 1 + max(m.lastColWidth, lipgloss.Width(singleLine(cell)))
		}
		m.maxRowWidth = max(m.maxRowWidth, width)
	}
}

// renderRowContent clips only visible rows, reusing overlapping cached rows.
func (m Model) renderRowContent(reuse bool) []string {
	if len(m.columns) == 0 {
		return nil
	}
	lastCol := len(m.columns) - 1
	end := min(m.yOffset+m.viewportHeight, len(m.rows))

	// Keep padding in display coordinates until it intersects the viewport.
	// A wide cell must not allocate its column width for every other row.
	lines := make([]string, end-m.yOffset)
	for i := m.yOffset; i < end; i++ {
		if reuse && i >= m.rowContentY && i < m.rowContentY+len(m.rowContent) {
			lines[i-m.yOffset] = m.rowContent[i-m.rowContentY]
			continue
		}
		line := viewportLine{offset: m.xOffset, width: m.contentWidth()}
		if fullRow, ok := m.fullRows[i]; ok {
			line.writeCell(fullRow, 0, AlignLeft)
		} else {
			for column, cell := range m.rows[i].Cells {
				if column > 0 {
					line.writePadding(1)
				}
				align := AlignLeft
				if column < len(m.columns) {
					align = m.columns[column].Align
				}
				width := m.lastColWidth
				if column < lastCol {
					width = m.colWidths[column]
				}
				line.writeCell(cell, width, align)
			}
		}
		lines[i-m.yOffset] = line.String()
	}
	return lines
}

// viewportLine builds only the visible part of a virtually padded line.
type viewportLine struct {
	content  strings.Builder
	position int
	offset   int
	width    int
}

func (l *viewportLine) String() string {
	// Fill the remaining viewport, including a partial glyph at the right edge.
	return applyHorizontalScroll(l.content.String(), 0, l.width)
}

func (l *viewportLine) writePadding(width int) {
	start := max(l.position, l.offset)
	end := min(l.position+width, l.offset+l.width)
	if end > start {
		l.content.WriteString(strings.Repeat(" ", end-start))
	}
	l.position += width
}

func (l *viewportLine) writeCell(s string, width int, align Alignment) {
	s = singleLine(s)
	contentWidth := lipgloss.Width(s)
	padding := max(width-contentWidth, 0)
	leftPad := 0
	switch align {
	case AlignRight:
		leftPad = padding
	case AlignCenter:
		leftPad = padding / 2
	case AlignLeft:
	}
	l.writePadding(leftPad)
	l.writeText(s, contentWidth)
	l.writePadding(padding - leftPad)
}

func (l *viewportLine) writeText(s string, width int) {
	start := max(l.offset-l.position, 0)
	end := min(l.offset+l.width-l.position, width)
	if end > start {
		// ansi.Cut includes a whole wide glyph when the left edge bisects it.
		// Replace that partial glyph with spaces so later cells keep their
		// display coordinates and the final metadata character stays visible.
		cutStart := start
		if start > 0 {
			for cutStart < end && lipgloss.Width(ansi.Cut(s, 0, cutStart)) < cutStart {
				cutStart++
			}
		}
		l.content.WriteString(strings.Repeat(" ", cutStart-start))
		l.content.WriteString(ansi.Cut(s, cutStart, end))
	} else if strings.Contains(s, "\x1b") {
		// Offscreen cells can still change ANSI state for subsequent cells.
		l.content.WriteString(ansi.Truncate(s, 0, ""))
	}
	l.position += width
}

// getVisibleContent copies the visible rows so scrollbar rendering cannot change
// the cached body, then fills unused viewport lines.
func (m Model) getVisibleContent() []string {
	visible := blankScrollbar(m.viewportHeight, m.contentWidth())
	copy(visible, m.content)
	return visible
}

// applyHorizontalScroll applies horizontal scroll offset to a plain text line.
func applyHorizontalScroll(line string, offset, visibleWidth int) string {
	return display.HorizontalScroll(line, offset, visibleWidth)
}

func applySelection(line string, span SelectionSpan, maxWidth, xOffset, visibleWidth int, style lipgloss.Style) string {
	lineWidth := lipgloss.Width(line)
	if lineWidth == 0 {
		return line
	}

	start := span.Start
	end := span.End
	start = mathutil.Clamp(start, 0, maxWidth)
	if end < 0 {
		end = maxWidth
	}
	end = mathutil.Clamp(end, 0, maxWidth)
	start -= xOffset
	end -= xOffset
	start = mathutil.Clamp(start, 0, visibleWidth)
	end = mathutil.Clamp(end, 0, visibleWidth)
	if start >= end || start >= lineWidth {
		return line
	}

	prefix := ansi.Cut(line, 0, start)
	mid := ansi.Cut(line, start, end)
	suffix := ansi.Cut(line, end, lineWidth)

	mid = ansi.Strip(mid)

	return prefix + style.Render(mid) + suffix
}

func (m Model) computeLastColWidth(colWidths []int, contentWidth int) int {
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

	if contentWidth > 0 {
		remaining := contentWidth - fixedWidth
		if remaining > lastWidth {
			lastWidth = remaining
		}
	}

	return lastWidth
}

func (m Model) columnRowWidth(lastColWidth int) int {
	if len(m.columns) == 0 {
		return 0
	}
	total := 0
	lastCol := len(m.columns) - 1
	for i := range m.columns {
		width := m.columns[i].Width
		if i == lastCol {
			width = lastColWidth
		} else if m.colWidths != nil && i < len(m.colWidths) && m.colWidths[i] > width {
			width = m.colWidths[i]
		}
		total += width
	}
	if len(m.columns) > 1 {
		total += lastCol // spaces between columns
	}
	return total
}

func (m Model) contentWidth() int {
	return max(m.width-m.scrollbarWidth(), 0)
}

func (m Model) scrollbarWidth() int {
	if m.width <= 0 {
		return 0
	}
	return scrollbar.DefaultWidth
}

func (m Model) scrollbarLines() []string {
	if m.viewportHeight <= 0 || m.scrollbarWidth() == 0 {
		return nil
	}

	total := len(m.rows)
	offset := m.yOffset
	if m.scrollbarOverride {
		total = m.scrollbarTotal
		offset = m.scrollbarOffset
	}
	total = mathutil.Clamp(total, 0, total)
	offset = mathutil.Clamp(offset, 0, offset)

	sb := m.scrollbar
	sb.SetRange(total, m.viewportHeight, offset)
	sb.SetSize(m.scrollbarWidth(), m.viewportHeight)
	lines := strings.Split(sb.View(), "\n")
	if len(lines) < m.viewportHeight {
		lines = append(lines, blankScrollbar(m.viewportHeight-len(lines), m.scrollbarWidth())...)
	}
	return lines
}

func (m Model) headerScrollbarLines(height int) []string {
	if height <= 0 {
		return nil
	}
	if m.scrollbarHeader == nil {
		return blankScrollbar(height, m.scrollbarWidth())
	}
	return normalizeScrollbarLines(m.scrollbarHeader, height, m.scrollbarWidth())
}

func appendScrollbar(lines, scrollbarLines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	width := 0
	if len(scrollbarLines) > 0 {
		width = lipgloss.Width(scrollbarLines[0])
	}
	for i, line := range lines {
		sbLine := strings.Repeat(" ", width)
		if i < len(scrollbarLines) {
			sbLine = scrollbarLines[i]
		}
		lines[i] = line + sbLine
	}
	return lines
}

func blankScrollbar(height, width int) []string {
	if height <= 0 {
		return nil
	}
	line := strings.Repeat(" ", max(width, 0))
	lines := make([]string, height)
	for i := range height {
		lines[i] = line
	}
	return lines
}

func normalizeScrollbarLines(lines []string, height, width int) []string {
	out := make([]string, height)
	for i := range height {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		if width > 0 {
			line = ansi.Cut(line, 0, width)
			lineWidth := lipgloss.Width(line)
			if lineWidth < width {
				line += strings.Repeat(" ", width-lineWidth)
			}
		}
		out[i] = line
	}
	return out
}

var rowWhitespace = strings.NewReplacer(
	"\r\n", " ",
	"\n", " ",
	"\r", " ",
	"\t", " ",
	"\v", " ",
	"\f", " ",
)

// singleLine keeps row offsets and widths consistent without changing stored data
// or ANSI styles. In particular, tabs must be replaced before clipping, since
// Lip Gloss otherwise expands them during styling.
func singleLine(s string) string {
	return rowWhitespace.Replace(s)
}

// padCell renders a single-line cell and pads it with the given alignment.
func padCell(s string, width int, align Alignment) string {
	s = singleLine(s)
	if width <= 0 {
		return s
	}
	contentWidth := lipgloss.Width(s)
	if contentWidth >= width {
		return s
	}

	padding := width - contentWidth

	switch align {
	case AlignRight:
		return strings.Repeat(" ", padding) + s
	case AlignCenter:
		leftPad := padding / 2
		rightPad := padding - leftPad
		return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
	case AlignLeft:
		return s + strings.Repeat(" ", padding)
	default:
		// Default to left alignment for any future alignment types
		return s + strings.Repeat(" ", padding)
	}
}

func indexRowByID(rows []Row, id string) int {
	return slices.IndexFunc(rows, func(row Row) bool {
		return row.ID == id
	})
}
