package table

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Column defines a table column
type Column struct {
	Title string
	Width int
}

// Styles holds the styles needed by the table
type Styles struct {
	Text      lipgloss.Style
	Muted     lipgloss.Style
	Header    lipgloss.Style
	Selected  lipgloss.Style
	Separator lipgloss.Style
}

// Table is a scrollable table component with selection support
type Table struct {
	columns        []Column
	rows           [][]string
	styles         Styles
	width          int
	height         int
	selectedRow    int
	yOffset        int
	xOffset        int
	maxRowWidth    int
	colWidths      []int  // dynamic column widths (max of defined and actual)
	emptyMessage   string
	content        string // pre-rendered body content
	viewportHeight int
}

// New creates a new Table component
func New(columns []Column) *Table {
	return &Table{
		columns:      columns,
		emptyMessage: "No data",
	}
}

// SetEmptyMessage sets the message shown when there are no rows
func (t *Table) SetEmptyMessage(msg string) *Table {
	t.emptyMessage = msg
	return t
}

// SetStyles updates the table styles
func (t *Table) SetStyles(styles Styles) {
	t.styles = styles
	t.rebuildContent()
}

// SetSize sets the table dimensions
func (t *Table) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.viewportHeight = height - 2 // minus header and separator
	if t.viewportHeight < 1 {
		t.viewportHeight = 1
	}
	t.rebuildContent()
	t.clampScroll()
}

// SetRows updates the table data
func (t *Table) SetRows(rows [][]string) {
	t.rows = rows
	// Keep selection in bounds
	if t.selectedRow >= len(t.rows) {
		t.selectedRow = len(t.rows) - 1
	}
	if t.selectedRow < 0 {
		t.selectedRow = 0
	}
	t.rebuildContent()
	t.clampScroll()
}

// SelectedRow returns the currently selected row index
func (t *Table) SelectedRow() int {
	return t.selectedRow
}

// RowCount returns the number of rows
func (t *Table) RowCount() int {
	return len(t.rows)
}

// Update handles key messages for navigation and scrolling
func (t *Table) Update(msg tea.Msg) (*Table, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		maxRow := len(t.rows) - 1
		if maxRow < 0 {
			maxRow = 0
		}

		switch msg.Type {
		case tea.KeyUp:
			t.moveSelectionUp(1)
		case tea.KeyDown:
			t.moveSelectionDown(1, maxRow)
		case tea.KeyLeft:
			t.scrollLeft()
		case tea.KeyRight:
			t.scrollRight()
		case tea.KeyPgUp:
			t.moveSelectionUp(10)
		case tea.KeyPgDown:
			t.moveSelectionDown(10, maxRow)
		case tea.KeyHome:
			t.selectedRow = 0
			t.yOffset = 0
			t.xOffset = 0
		case tea.KeyEnd:
			t.selectedRow = maxRow
			t.gotoBottom()
		default:
			switch msg.String() {
			case "k":
				t.moveSelectionUp(1)
			case "j":
				t.moveSelectionDown(1, maxRow)
			case "h":
				t.scrollLeft()
			case "l":
				t.scrollRight()
			case "g":
				t.selectedRow = 0
				t.yOffset = 0
				t.xOffset = 0
			case "G":
				t.selectedRow = maxRow
				t.gotoBottom()
			case "0":
				t.xOffset = 0
			case "$":
				t.xOffset = t.maxScrollOffset()
			}
		}
		t.rebuildContent()
	}
	return t, nil
}

// View renders the table (header + visible rows)
func (t *Table) View() string {
	header := t.renderHeader()
	body := t.getVisibleContent()
	return header + "\n" + body
}

// moveSelectionUp moves selection up by n rows
func (t *Table) moveSelectionUp(n int) {
	t.selectedRow -= n
	if t.selectedRow < 0 {
		t.selectedRow = 0
	}
	t.ensureSelectedVisible()
}

// moveSelectionDown moves selection down by n rows
func (t *Table) moveSelectionDown(n, maxRow int) {
	t.selectedRow += n
	if t.selectedRow > maxRow {
		t.selectedRow = maxRow
	}
	t.ensureSelectedVisible()
}

// scrollLeft scrolls content left
func (t *Table) scrollLeft() {
	t.xOffset -= 4
	if t.xOffset < 0 {
		t.xOffset = 0
	}
}

// scrollRight scrolls content right
func (t *Table) scrollRight() {
	maxScroll := t.maxScrollOffset()
	t.xOffset += 4
	if t.xOffset > maxScroll {
		t.xOffset = maxScroll
	}
}

// maxScrollOffset returns the maximum horizontal scroll offset
func (t *Table) maxScrollOffset() int {
	maxScroll := t.maxRowWidth - t.width
	if maxScroll < 0 {
		return 0
	}
	return maxScroll
}

// clampScroll ensures scroll offsets are within valid bounds
func (t *Table) clampScroll() {
	maxX := t.maxScrollOffset()
	if t.xOffset > maxX {
		t.xOffset = maxX
	}
	if t.xOffset < 0 {
		t.xOffset = 0
	}

	maxY := len(t.rows) - t.viewportHeight
	if maxY < 0 {
		maxY = 0
	}
	if t.yOffset > maxY {
		t.yOffset = maxY
	}
	if t.yOffset < 0 {
		t.yOffset = 0
	}
}

// ensureSelectedVisible scrolls to keep selected row visible
func (t *Table) ensureSelectedVisible() {
	if t.selectedRow < t.yOffset {
		t.yOffset = t.selectedRow
	} else if t.selectedRow >= t.yOffset+t.viewportHeight {
		t.yOffset = t.selectedRow - t.viewportHeight + 1
	}
}

// gotoBottom scrolls to show the last rows
func (t *Table) gotoBottom() {
	maxOffset := len(t.rows) - t.viewportHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	t.yOffset = maxOffset
}

// rebuildContent rebuilds the pre-rendered body content
func (t *Table) rebuildContent() {
	t.content = t.renderBody()
}

// renderHeader renders the table header with separator
func (t *Table) renderHeader() string {
	var cols []string
	lastCol := len(t.columns) - 1
	for i, col := range t.columns {
		// Use dynamic column width if available, otherwise use defined width
		width := col.Width
		if t.colWidths != nil && i < len(t.colWidths) {
			width = t.colWidths[i]
		}

		if i < lastCol {
			cols = append(cols, padRight(col.Title, width))
		} else {
			// Last column: no width constraint
			cols = append(cols, col.Title)
		}
	}
	header := strings.Join(cols, " ")

	// Use maxRowWidth for consistent scrolling with body
	totalWidth := t.maxRowWidth
	if totalWidth == 0 {
		for _, col := range t.columns {
			totalWidth += col.Width + 1
		}
	}

	// Pad header to match body width
	if len(header) < totalWidth {
		header = header + strings.Repeat(" ", totalWidth-len(header))
	}

	// Apply horizontal scroll to header
	header = applyHorizontalScroll(header, t.xOffset, t.width)
	styledHeader := t.styles.Header.Render(header)

	// Separator line (also scrolled)
	separator := strings.Repeat("─", totalWidth)
	separator = applyHorizontalScroll(separator, t.xOffset, t.width)

	return styledHeader + "\n" + t.styles.Separator.Render(separator)
}

// renderBody renders all table rows (for scrolling)
func (t *Table) renderBody() string {
	if len(t.rows) == 0 {
		t.maxRowWidth = 0
		t.colWidths = nil
		return t.styles.Muted.Render(t.emptyMessage)
	}

	// First pass: find max width for each column (at least the defined width)
	t.colWidths = make([]int, len(t.columns))
	for i, col := range t.columns {
		t.colWidths[i] = col.Width
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(t.colWidths) && len(cell) > t.colWidths[i] {
				t.colWidths[i] = len(cell)
			}
		}
	}

	// Second pass: build all rows using actual column widths (no truncation)
	var rawRows []string
	maxWidth := 0
	lastCol := len(t.columns) - 1
	for _, row := range t.rows {
		var cols []string
		for i, cell := range row {
			if i < lastCol {
				cols = append(cols, padRight(cell, t.colWidths[i]))
			} else {
				// Last column: no padding (variable width)
				cols = append(cols, cell)
			}
		}
		rowStr := strings.Join(cols, " ")
		rawRows = append(rawRows, rowStr)

		if len(rowStr) > maxWidth {
			maxWidth = len(rowStr)
		}
	}
	t.maxRowWidth = maxWidth

	// Second pass: apply scroll and styling
	var lines []string
	for i, row := range rawRows {
		// Pad row to max width for consistent selection highlight
		if len(row) < maxWidth {
			row = row + strings.Repeat(" ", maxWidth-len(row))
		}

		// Apply horizontal scroll offset (before styling)
		row = applyHorizontalScroll(row, t.xOffset, t.width)

		// Apply selection highlight
		if i == t.selectedRow {
			row = t.styles.Selected.Render(row)
		} else {
			row = t.styles.Text.Render(row)
		}

		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

// getVisibleContent returns the visible portion of content based on yOffset
func (t *Table) getVisibleContent() string {
	if t.content == "" {
		return ""
	}

	lines := strings.Split(t.content, "\n")

	// Clamp yOffset to valid range
	if t.yOffset < 0 {
		t.yOffset = 0
	}
	if t.yOffset >= len(lines) {
		t.yOffset = len(lines) - 1
		if t.yOffset < 0 {
			t.yOffset = 0
		}
	}

	// Get visible slice
	end := t.yOffset + t.viewportHeight
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[t.yOffset:end], "\n")
}

// applyHorizontalScroll applies horizontal scroll offset to a plain text line
func applyHorizontalScroll(line string, offset, visibleWidth int) string {
	runes := []rune(line)

	// Apply offset
	if offset >= len(runes) {
		return strings.Repeat(" ", visibleWidth)
	}
	runes = runes[offset:]

	// Pad or truncate to visible width
	if len(runes) < visibleWidth {
		return string(runes) + strings.Repeat(" ", visibleWidth-len(runes))
	}
	return string(runes[:visibleWidth])
}

// padRight pads a string to the specified width (no truncation)
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
