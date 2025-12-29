// Package jobdetail renders job detail panels.
package jobdetail

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/format"
)

// KeyMap defines keybindings for the job detail view.
type KeyMap struct {
	SwitchPanel key.Binding
	LineUp      key.Binding
	LineDown    key.Binding
	ScrollLeft  key.Binding
	ScrollRight key.Binding
	GotoTop     key.Binding
	GotoBottom  key.Binding
	Home        key.Binding
	End         key.Binding
}

// DefaultKeyMap returns default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		SwitchPanel: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch panel"),
		),
		LineUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("j/k", "scroll"),
		),
		LineDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/k", "scroll"),
		),
		ScrollLeft: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("h/l", "scroll left/right"),
		),
		ScrollRight: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("h/l", "scroll left/right"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "go to top"),
		),
		GotoBottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to bottom"),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "0"),
			key.WithHelp("0", "scroll to start"),
		),
		End: key.NewBinding(
			key.WithKeys("end", "$"),
			key.WithHelp("$", "scroll to end"),
		),
	}
}

// Styles holds styles for the job detail component.
type Styles struct {
	Title       lipgloss.Style
	Label       lipgloss.Style
	Value       lipgloss.Style
	JSON        lipgloss.Style
	Border      lipgloss.Style
	PanelTitle  lipgloss.Style
	FocusBorder lipgloss.Style
	Muted       lipgloss.Style
}

// DefaultStyles returns default styles.
func DefaultStyles() Styles {
	return Styles{
		Title:       lipgloss.NewStyle().Bold(true),
		Label:       lipgloss.NewStyle().Faint(true),
		Value:       lipgloss.NewStyle(),
		JSON:        lipgloss.NewStyle(),
		Border:      lipgloss.NewStyle(),
		PanelTitle:  lipgloss.NewStyle().Bold(true),
		FocusBorder: lipgloss.NewStyle(),
		Muted:       lipgloss.NewStyle().Faint(true),
	}
}

// PropertyRow represents a key-value pair for display.
type PropertyRow struct {
	Label string
	Value string
}

// Model is the job detail component state.
type Model struct {
	KeyMap KeyMap
	styles Styles
	width  int
	height int

	// Job data
	job        *sidekiq.JobRecord
	properties []PropertyRow
	jsonLines  []string

	// Scroll state
	leftYOffset  int
	rightYOffset int
	rightXOffset int

	// Focus state (false = left panel, true = right panel)
	focusRight bool

	// Calculated dimensions
	leftWidth    int
	rightWidth   int
	panelHeight  int
	maxJSONWidth int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new job detail model.
func New(opts ...Option) Model {
	m := Model{
		KeyMap: DefaultKeyMap(),
		styles: DefaultStyles(),
	}

	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// WithStyles sets the styles.
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

// WithSize sets the dimensions.
func WithSize(width, height int) Option {
	return func(m *Model) {
		m.width = width
		m.height = height
		m.updateDimensions()
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize sets the dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.updateDimensions()
	m.clampScroll()
}

// SetJob sets the job to display.
func (m *Model) SetJob(job *sidekiq.JobRecord) {
	m.job = job
	m.leftYOffset = 0
	m.rightYOffset = 0
	m.rightXOffset = 0
	m.focusRight = false

	m.extractProperties()
	m.formatJSON()
}

// Update handles key messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.SwitchPanel):
			m.focusRight = !m.focusRight

		case key.Matches(msg, m.KeyMap.LineUp):
			if m.focusRight {
				m.rightYOffset--
				if m.rightYOffset < 0 {
					m.rightYOffset = 0
				}
			} else {
				m.leftYOffset--
				if m.leftYOffset < 0 {
					m.leftYOffset = 0
				}
			}

		case key.Matches(msg, m.KeyMap.LineDown):
			if m.focusRight {
				maxY := len(m.jsonLines) - m.panelHeight
				if maxY < 0 {
					maxY = 0
				}
				m.rightYOffset++
				if m.rightYOffset > maxY {
					m.rightYOffset = maxY
				}
			} else {
				maxY := m.countLeftPanelLines() - m.panelHeight
				if maxY < 0 {
					maxY = 0
				}
				m.leftYOffset++
				if m.leftYOffset > maxY {
					m.leftYOffset = maxY
				}
			}

		case key.Matches(msg, m.KeyMap.ScrollLeft):
			if m.focusRight {
				m.rightXOffset -= 4
				if m.rightXOffset < 0 {
					m.rightXOffset = 0
				}
			}

		case key.Matches(msg, m.KeyMap.ScrollRight):
			if m.focusRight {
				maxX := m.maxJSONWidth - m.rightWidth + 2
				if maxX < 0 {
					maxX = 0
				}
				m.rightXOffset += 4
				if m.rightXOffset > maxX {
					m.rightXOffset = maxX
				}
			}

		case key.Matches(msg, m.KeyMap.GotoTop):
			if m.focusRight {
				m.rightYOffset = 0
			} else {
				m.leftYOffset = 0
			}

		case key.Matches(msg, m.KeyMap.GotoBottom):
			if m.focusRight {
				maxY := len(m.jsonLines) - m.panelHeight
				if maxY < 0 {
					maxY = 0
				}
				m.rightYOffset = maxY
			} else {
				maxY := m.countLeftPanelLines() - m.panelHeight
				if maxY < 0 {
					maxY = 0
				}
				m.leftYOffset = maxY
			}

		case key.Matches(msg, m.KeyMap.Home):
			if m.focusRight {
				m.rightXOffset = 0
			}

		case key.Matches(msg, m.KeyMap.End):
			if m.focusRight {
				maxX := m.maxJSONWidth - m.rightWidth + 2
				if maxX < 0 {
					maxX = 0
				}
				m.rightXOffset = maxX
			}
		}
	}

	return m, nil
}

// View renders the job detail view.
func (m Model) View() string {
	if m.job == nil {
		return ""
	}

	leftPanel := m.renderLeftPanel()
	rightPanel := m.renderRightPanel()

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// updateDimensions recalculates panel dimensions.
func (m *Model) updateDimensions() {
	// Split width: 40% left, 60% right (with 1 char gap)
	m.leftWidth = (m.width * 40) / 100
	if m.leftWidth < 30 {
		m.leftWidth = 30
	}
	m.rightWidth = m.width - m.leftWidth

	// Height minus border (2 lines: top and bottom)
	// Note: title is part of the top border, not a separate line
	m.panelHeight = m.height - 2
	if m.panelHeight < 1 {
		m.panelHeight = 1
	}
}

// clampScroll ensures scroll offsets are in valid range.
func (m *Model) clampScroll() {
	// Left panel - count actual display lines (with wrapping)
	leftLineCount := m.countLeftPanelLines()
	maxLeftY := leftLineCount - m.panelHeight
	if maxLeftY < 0 {
		maxLeftY = 0
	}
	if m.leftYOffset > maxLeftY {
		m.leftYOffset = maxLeftY
	}

	// Right panel
	maxRightY := len(m.jsonLines) - m.panelHeight
	if maxRightY < 0 {
		maxRightY = 0
	}
	if m.rightYOffset > maxRightY {
		m.rightYOffset = maxRightY
	}

	maxRightX := m.maxJSONWidth - m.rightWidth + 2
	if maxRightX < 0 {
		maxRightX = 0
	}
	if m.rightXOffset > maxRightX {
		m.rightXOffset = maxRightX
	}
}

// countLeftPanelLines counts total display lines in left panel (with wrapping).
func (m Model) countLeftPanelLines() int {
	if len(m.properties) == 0 {
		return 0
	}

	// Calculate value width (same as in renderLeftPanel)
	innerWidth := m.leftWidth - 2
	valueIndent := 2
	valueWidth := innerWidth - 1 - valueIndent - 1
	if valueWidth < 10 {
		valueWidth = 10
	}

	count := 0
	for _, prop := range m.properties {
		count++ // label line
		lines := wrapText(prop.Value, valueWidth)
		if len(lines) == 0 {
			count++ // empty value line
		} else {
			count += len(lines)
		}
	}
	return count
}

// extractProperties builds the properties list from job data.
func (m *Model) extractProperties() {
	m.properties = nil
	if m.job == nil {
		return
	}

	// Basic properties
	m.properties = append(m.properties, PropertyRow{Label: "JID", Value: m.job.JID()})
	if bid := m.job.Bid(); bid != "" {
		m.properties = append(m.properties, PropertyRow{Label: "BID", Value: bid})
	}
	m.properties = append(m.properties, PropertyRow{Label: "Queue", Value: m.job.Queue()})
	m.properties = append(m.properties, PropertyRow{Label: "Class", Value: m.job.DisplayClass()})

	// Timestamps
	if enqueuedAt := m.job.EnqueuedAt(); enqueuedAt > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Enqueued At",
			Value: formatTimestamp(enqueuedAt),
		})
	}
	if createdAt := m.job.CreatedAt(); createdAt > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Created At",
			Value: formatTimestamp(createdAt),
		})
	}
	if latency := m.job.Latency(); latency > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Latency",
			Value: format.Duration(int64(math.Round(latency))),
		})
	}
	if tags := m.job.Tags(); len(tags) > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Tags",
			Value: strings.Join(tags, ", "),
		})
	}

	// Error info (for retry/dead jobs)
	if m.job.HasError() {
		m.properties = append(m.properties, PropertyRow{Label: "Error Class", Value: m.job.ErrorClass()})
		m.properties = append(m.properties, PropertyRow{Label: "Error", Value: m.job.ErrorMessage()})
	}
	if retryCount := m.job.RetryCount(); retryCount > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Retry Count",
			Value: fmt.Sprintf("%d", retryCount),
		})
	}
	if failedAt := m.job.FailedAt(); failedAt > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Failed At",
			Value: formatTimestamp(failedAt),
		})
	}
	if retriedAt := m.job.RetriedAt(); retriedAt > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Retried At",
			Value: formatTimestamp(retriedAt),
		})
	}
	if backtrace := m.job.ErrorBacktrace(); len(backtrace) > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Backtrace",
			Value: strings.Join(backtrace, " | "),
		})
	}

	// Arguments summary
	displayArgs := m.job.DisplayArgs()
	if len(displayArgs) > 0 {
		m.properties = append(m.properties, PropertyRow{
			Label: "Args",
			Value: format.Args(displayArgs),
		})
	}
}

// formatJSON creates pretty-printed JSON lines.
func (m *Model) formatJSON() {
	m.jsonLines = nil
	m.maxJSONWidth = 0

	if m.job == nil {
		return
	}

	b, err := json.MarshalIndent(m.job.Item(), "", "  ")
	if err != nil {
		m.jsonLines = []string{"{}", "  Error formatting JSON"}
		return
	}

	m.jsonLines = strings.Split(string(b), "\n")

	// Calculate max width for horizontal scroll
	for _, line := range m.jsonLines {
		if len(line) > m.maxJSONWidth {
			m.maxJSONWidth = len(line)
		}
	}
}

// renderLeftPanel renders the properties panel.
func (m Model) renderLeftPanel() string {
	border := lipgloss.RoundedBorder()

	// Choose border style based on focus
	borderStyle := m.styles.Border
	if !m.focusRight {
		borderStyle = m.styles.FocusBorder
	}

	// Panel title
	title := " Job Details "
	titleWidth := lipgloss.Width(title)

	// Build borders
	hBar := borderStyle.Render(string(border.Top))
	innerWidth := m.leftWidth - 2 // minus left and right border

	// Top border with title
	titlePad := innerWidth - titleWidth - 1
	if titlePad < 0 {
		titlePad = 0
	}
	topBorder := borderStyle.Render(string(border.TopLeft)) +
		hBar +
		m.styles.PanelTitle.Render(title) +
		strings.Repeat(hBar, titlePad) +
		borderStyle.Render(string(border.TopRight))

	// Calculate available width for values (with 2-space indent)
	valueIndent := "  "
	valueWidth := innerWidth - 1 - len(valueIndent) - 1 // left padding, indent, right padding
	if valueWidth < 10 {
		valueWidth = 10
	}

	// Build all display lines (label on own row, value indented below)
	var allLines []string
	for _, prop := range m.properties {
		// Label row
		label := m.styles.Label.Render(prop.Label + ":")
		allLines = append(allLines, " "+label)
		// Value rows (indented, wrapped if needed)
		valueLines := wrapText(prop.Value, valueWidth)
		if len(valueLines) == 0 {
			valueLines = []string{""}
		}
		for _, vl := range valueLines {
			allLines = append(allLines, " "+valueIndent+m.styles.Value.Render(vl))
		}
	}

	// Apply vertical scroll
	var contentLines []string
	endY := m.leftYOffset + m.panelHeight
	if endY > len(allLines) {
		endY = len(allLines)
	}
	if m.leftYOffset < len(allLines) {
		contentLines = allLines[m.leftYOffset:endY]
	}

	// Pad to panel height
	for len(contentLines) < m.panelHeight {
		contentLines = append(contentLines, "")
	}

	// Add borders to content
	vBar := borderStyle.Render(string(border.Left))
	vBarRight := borderStyle.Render(string(border.Right))
	var middleLines []string
	for _, line := range contentLines {
		lineWidth := lipgloss.Width(line)
		padding := innerWidth - lineWidth
		if padding < 0 {
			padding = 0
		}
		middleLines = append(middleLines, vBar+line+strings.Repeat(" ", padding)+vBarRight)
	}

	// Bottom border
	bottomBorder := borderStyle.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		borderStyle.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}

// renderRightPanel renders the JSON panel.
func (m Model) renderRightPanel() string {
	border := lipgloss.RoundedBorder()

	// Choose border style based on focus
	borderStyle := m.styles.Border
	if m.focusRight {
		borderStyle = m.styles.FocusBorder
	}

	// Panel title and hint
	title := " Job Data (JSON) "
	hint := " Esc to close "
	titleWidth := lipgloss.Width(title)
	hintWidth := lipgloss.Width(hint)

	// Build borders
	hBar := borderStyle.Render(string(border.Top))
	innerWidth := m.rightWidth - 2 // minus left and right border

	// Top border with title on left and hint on right
	middlePad := innerWidth - titleWidth - hintWidth - 2 // -2 for the two hBar segments
	if middlePad < 0 {
		middlePad = 0
	}
	topBorder := borderStyle.Render(string(border.TopLeft)) +
		hBar +
		m.styles.PanelTitle.Render(title) +
		strings.Repeat(hBar, middlePad) +
		m.styles.Muted.Render(hint) +
		hBar +
		borderStyle.Render(string(border.TopRight))

	// Content lines with horizontal scroll
	var contentLines []string
	endY := m.rightYOffset + m.panelHeight
	if endY > len(m.jsonLines) {
		endY = len(m.jsonLines)
	}

	contentWidth := innerWidth - 2 // padding on each side

	for i := m.rightYOffset; i < endY; i++ {
		line := m.jsonLines[i]
		// Apply horizontal scroll BEFORE styling
		line = applyHorizontalScroll(line, m.rightXOffset, contentWidth)
		line = " " + m.styles.JSON.Render(line) + " "
		contentLines = append(contentLines, line)
	}

	// Pad to panel height
	for len(contentLines) < m.panelHeight {
		contentLines = append(contentLines, strings.Repeat(" ", innerWidth))
	}

	// Add borders to content
	vBar := borderStyle.Render(string(border.Left))
	vBarRight := borderStyle.Render(string(border.Right))
	var middleLines []string
	for _, line := range contentLines {
		lineWidth := lipgloss.Width(line)
		padding := innerWidth - lineWidth
		if padding < 0 {
			padding = 0
		}
		middleLines = append(middleLines, vBar+line+strings.Repeat(" ", padding)+vBarRight)
	}

	// Bottom border
	bottomBorder := borderStyle.Render(string(border.BottomLeft)) +
		strings.Repeat(hBar, innerWidth) +
		borderStyle.Render(string(border.BottomRight))

	return topBorder + "\n" + strings.Join(middleLines, "\n") + "\n" + bottomBorder
}

// formatTimestamp formats a Unix timestamp.
func formatTimestamp(ts float64) string {
	// Handle both seconds and milliseconds
	var t time.Time
	if ts > 1e12 {
		t = time.UnixMilli(int64(ts))
	} else {
		t = time.Unix(int64(ts), 0)
	}
	return t.Format("2006-01-02 15:04:05")
}

// applyHorizontalScroll applies horizontal scroll offset.
func applyHorizontalScroll(line string, offset, visibleWidth int) string {
	runes := []rune(line)

	if offset >= len(runes) {
		return strings.Repeat(" ", visibleWidth)
	}
	runes = runes[offset:]

	if len(runes) < visibleWidth {
		return string(runes) + strings.Repeat(" ", visibleWidth-len(runes))
	}
	return string(runes[:visibleWidth])
}

// wrapText wraps text to fit within the specified width.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}

	runes := []rune(s)
	if len(runes) <= width {
		return []string{s}
	}

	var lines []string
	for len(runes) > 0 {
		if len(runes) <= width {
			lines = append(lines, string(runes))
			break
		}
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	return lines
}
