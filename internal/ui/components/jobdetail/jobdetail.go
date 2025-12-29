// Package jobdetail renders job detail panels.
package jobdetail

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/jsonview"
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
	Title           lipgloss.Style
	Label           lipgloss.Style
	Value           lipgloss.Style
	JSON            lipgloss.Style
	JSONKey         lipgloss.Style
	JSONString      lipgloss.Style
	JSONNumber      lipgloss.Style
	JSONBool        lipgloss.Style
	JSONNull        lipgloss.Style
	JSONPunctuation lipgloss.Style
	Border          lipgloss.Style
	PanelTitle      lipgloss.Style
	FocusBorder     lipgloss.Style
	Muted           lipgloss.Style
}

// DefaultStyles returns default styles.
func DefaultStyles() Styles {
	return Styles{
		Title:           lipgloss.NewStyle().Bold(true),
		Label:           lipgloss.NewStyle().Faint(true),
		Value:           lipgloss.NewStyle(),
		JSON:            lipgloss.NewStyle(),
		JSONKey:         lipgloss.NewStyle(),
		JSONString:      lipgloss.NewStyle(),
		JSONNumber:      lipgloss.NewStyle(),
		JSONBool:        lipgloss.NewStyle(),
		JSONNull:        lipgloss.NewStyle(),
		JSONPunctuation: lipgloss.NewStyle(),
		Border:          lipgloss.NewStyle(),
		PanelTitle:      lipgloss.NewStyle().Bold(true),
		FocusBorder:     lipgloss.NewStyle(),
		Muted:           lipgloss.NewStyle().Faint(true),
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
	jsonView   jsonview.Model

	// Scroll state
	leftYOffset  int
	rightYOffset int
	rightXOffset int

	// Focus state (false = left panel, true = right panel)
	focusRight bool

	// Calculated dimensions
	leftWidth   int
	rightWidth  int
	panelHeight int
}

const (
	jobDetailPanelPadding = 1
	jobDetailValueIndent  = 2
)

// Option is used to set options in New.
type Option func(*Model)

// New creates a new job detail model.
func New(opts ...Option) Model {
	m := Model{
		KeyMap:   DefaultKeyMap(),
		styles:   DefaultStyles(),
		jsonView: jsonview.New(),
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
		m.jsonView.SetStyles(jsonview.Styles{
			Text:        s.JSON,
			Key:         s.JSONKey,
			String:      s.JSONString,
			Number:      s.JSONNumber,
			Bool:        s.JSONBool,
			Null:        s.JSONNull,
			Punctuation: s.JSONPunctuation,
			Muted:       s.Muted,
		})
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
		m.jsonView.SetSize(width, height)
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.jsonView.SetStyles(jsonview.Styles{
		Text:        s.JSON,
		Key:         s.JSONKey,
		String:      s.JSONString,
		Number:      s.JSONNumber,
		Bool:        s.JSONBool,
		Null:        s.JSONNull,
		Punctuation: s.JSONPunctuation,
		Muted:       s.Muted,
	})
}

// SetSize sets the dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.updateDimensions()
	m.clampScroll()
	m.jsonView.SetSize(width, height)
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
				m.rightYOffset = clampZeroMax(m.rightYOffset-1, m.maxRightYOffset())
			} else {
				m.leftYOffset = clampZeroMax(m.leftYOffset-1, m.maxLeftYOffset())
			}

		case key.Matches(msg, m.KeyMap.LineDown):
			if m.focusRight {
				m.rightYOffset = clampZeroMax(m.rightYOffset+1, m.maxRightYOffset())
			} else {
				m.leftYOffset = clampZeroMax(m.leftYOffset+1, m.maxLeftYOffset())
			}

		case key.Matches(msg, m.KeyMap.ScrollLeft):
			if m.focusRight {
				m.rightXOffset = clampZeroMax(m.rightXOffset-4, m.maxRightXOffset())
			}

		case key.Matches(msg, m.KeyMap.ScrollRight):
			if m.focusRight {
				m.rightXOffset = clampZeroMax(m.rightXOffset+4, m.maxRightXOffset())
			}

		case key.Matches(msg, m.KeyMap.GotoTop):
			if m.focusRight {
				m.rightYOffset = 0
			} else {
				m.leftYOffset = 0
			}

		case key.Matches(msg, m.KeyMap.GotoBottom):
			if m.focusRight {
				m.rightYOffset = m.maxRightYOffset()
			} else {
				m.leftYOffset = m.maxLeftYOffset()
			}

		case key.Matches(msg, m.KeyMap.Home):
			if m.focusRight {
				m.rightXOffset = 0
			}

		case key.Matches(msg, m.KeyMap.End):
			if m.focusRight {
				m.rightXOffset = m.maxRightXOffset()
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
	m.leftWidth = max((m.width*40)/100, 30)
	m.rightWidth = m.width - m.leftWidth

	// Height minus border (2 lines: top and bottom)
	// Note: title is part of the top border, not a separate line
	m.panelHeight = max(m.height-2, 1)
}

func (m Model) maxLeftYOffset() int {
	maxY := m.countLeftPanelLines() - m.panelHeight
	if maxY < 0 {
		return 0
	}
	return maxY
}

func (m Model) maxRightYOffset() int {
	maxY := m.jsonView.LineCount() - m.panelHeight
	if maxY < 0 {
		return 0
	}
	return maxY
}

func (m Model) maxRightXOffset() int {
	contentWidth := max(m.rightWidth-2-2*jobDetailPanelPadding, 0)
	maxX := m.jsonView.MaxWidth() - contentWidth
	if maxX < 0 {
		return 0
	}
	return maxX
}

func clampZeroMax(value, maxValue int) int {
	if value < 0 {
		return 0
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

// clampScroll ensures scroll offsets are in valid range.
func (m *Model) clampScroll() {
	// Left panel - count actual display lines (with wrapping)
	maxLeftY := m.maxLeftYOffset()
	if m.leftYOffset > maxLeftY {
		m.leftYOffset = maxLeftY
	}

	// Right panel
	maxRightY := m.maxRightYOffset()
	if m.rightYOffset > maxRightY {
		m.rightYOffset = maxRightY
	}

	maxRightX := m.maxRightXOffset()
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
	contentWidth := max(innerWidth-2*jobDetailPanelPadding, 0)
	valueWidth := max(contentWidth-jobDetailValueIndent, 10)

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
	if m.job == nil {
		m.jsonView.SetValue(nil)
		return
	}
	m.jsonView.SetValue(m.job.Item())
}

// renderLeftPanel renders the properties panel.
func (m Model) renderLeftPanel() string {
	innerWidth := m.leftWidth - 2 // minus left and right border

	// Calculate available width for values (with 2-space indent)
	contentWidth := max(innerWidth-2*jobDetailPanelPadding, 0)
	valueIndent := strings.Repeat(" ", jobDetailValueIndent)
	valueWidth := max(contentWidth-jobDetailValueIndent, 10)

	// Build all display lines (label on own row, value indented below)
	allLines := make([]string, 0, len(m.properties)*2)
	for _, prop := range m.properties {
		// Label row
		label := m.styles.Label.Render(prop.Label + ":")
		allLines = append(allLines, label)
		// Value rows (indented, wrapped if needed)
		valueLines := wrapText(prop.Value, valueWidth)
		if len(valueLines) == 0 {
			valueLines = []string{""}
		}
		for _, vl := range valueLines {
			allLines = append(allLines, valueIndent+m.styles.Value.Render(vl))
		}
	}

	// Apply vertical scroll
	var contentLines []string
	endY := min(m.leftYOffset+m.panelHeight, len(allLines))
	if m.leftYOffset < len(allLines) {
		contentLines = allLines[m.leftYOffset:endY]
	}

	// Pad to panel height
	for len(contentLines) < m.panelHeight {
		contentLines = append(contentLines, "")
	}
	return frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.PanelTitle,
				Border: m.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.PanelTitle,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle("Job Details"),
		frame.WithTitlePadding(0),
		frame.WithContent(strings.Join(contentLines, "\n")),
		frame.WithPadding(jobDetailPanelPadding),
		frame.WithSize(m.leftWidth, m.height),
		frame.WithFocused(!m.focusRight),
	).View()
}

// renderRightPanel renders the JSON panel.
func (m Model) renderRightPanel() string {
	innerWidth := m.rightWidth - 2 // minus left and right border
	contentWidth := max(innerWidth-2*jobDetailPanelPadding, 0)

	// Content lines with horizontal scroll
	endY := min(m.rightYOffset+m.panelHeight, m.jsonView.LineCount())
	contentCap := 0
	if endY > m.rightYOffset {
		contentCap = endY - m.rightYOffset
	}
	contentLines := make([]string, 0, contentCap)

	for i := m.rightYOffset; i < endY; i++ {
		contentLines = append(contentLines, m.jsonView.RenderLine(i, m.rightXOffset, contentWidth))
	}

	// Pad to panel height
	for len(contentLines) < m.panelHeight {
		contentLines = append(contentLines, "")
	}
	return frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.PanelTitle,
				Border: m.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.PanelTitle,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle("Job Data (JSON)"),
		frame.WithTitlePadding(0),
		frame.WithMeta(m.styles.Muted.Render("Esc to close")),
		frame.WithMetaPadding(0),
		frame.WithContent(strings.Join(contentLines, "\n")),
		frame.WithPadding(jobDetailPanelPadding),
		frame.WithSize(m.rightWidth, m.height),
		frame.WithFocused(m.focusRight),
	).View()
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

// wrapText wraps text to fit within the specified width.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}

	if lipgloss.Width(s) <= width {
		return []string{s}
	}

	var lines []string
	for lipgloss.Width(s) > width {
		lines = append(lines, ansi.Truncate(s, width, ""))
		s = ansi.Cut(s, width, lipgloss.Width(s))
	}
	if s != "" {
		lines = append(lines, s)
	}
	return lines
}
