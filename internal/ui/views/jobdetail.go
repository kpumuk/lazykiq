package views

import (
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/jsonview"
	"github.com/kpumuk/lazykiq/internal/ui/components/messagebox"
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

type jobDetailStyles struct {
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
	FilterFocused   lipgloss.Style
	FilterBlurred   lipgloss.Style
}

// PropertyRow represents a key-value pair for display.
type PropertyRow struct {
	Label string
	Value string
}

// JobDetail shows a full job detail panel.
type JobDetail struct {
	KeyMap KeyMap
	styles jobDetailStyles
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

// NewJobDetail creates a new job detail view.
func NewJobDetail() *JobDetail {
	return &JobDetail{
		KeyMap:   DefaultKeyMap(),
		jsonView: jsonview.New(),
	}
}

// Init implements View.
func (j *JobDetail) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (j *JobDetail) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, j.KeyMap.SwitchPanel):
			j.focusRight = !j.focusRight

		case key.Matches(msg, j.KeyMap.LineUp):
			if j.focusRight {
				j.rightYOffset = clampZeroMax(j.rightYOffset-1, j.maxRightYOffset())
			} else {
				j.leftYOffset = clampZeroMax(j.leftYOffset-1, j.maxLeftYOffset())
			}

		case key.Matches(msg, j.KeyMap.LineDown):
			if j.focusRight {
				j.rightYOffset = clampZeroMax(j.rightYOffset+1, j.maxRightYOffset())
			} else {
				j.leftYOffset = clampZeroMax(j.leftYOffset+1, j.maxLeftYOffset())
			}

		case key.Matches(msg, j.KeyMap.ScrollLeft):
			if j.focusRight {
				j.rightXOffset = clampZeroMax(j.rightXOffset-4, j.maxRightXOffset())
			}

		case key.Matches(msg, j.KeyMap.ScrollRight):
			if j.focusRight {
				j.rightXOffset = clampZeroMax(j.rightXOffset+4, j.maxRightXOffset())
			}

		case key.Matches(msg, j.KeyMap.GotoTop):
			if j.focusRight {
				j.rightYOffset = 0
			} else {
				j.leftYOffset = 0
			}

		case key.Matches(msg, j.KeyMap.GotoBottom):
			if j.focusRight {
				j.rightYOffset = j.maxRightYOffset()
			} else {
				j.leftYOffset = j.maxLeftYOffset()
			}

		case key.Matches(msg, j.KeyMap.Home):
			if j.focusRight {
				j.rightXOffset = 0
			}

		case key.Matches(msg, j.KeyMap.End):
			if j.focusRight {
				j.rightXOffset = j.maxRightXOffset()
			}
		}
	}

	return j, nil
}

// View implements View.
func (j *JobDetail) View() string {
	if j.job == nil {
		return messagebox.Render(messagebox.Styles{
			Title:  j.styles.Title,
			Muted:  j.styles.Muted,
			Border: j.styles.FocusBorder,
		}, "Job Detail", "No job selected", j.width, j.height)
	}

	leftPanel := j.renderLeftPanel()
	rightPanel := j.renderRightPanel()

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// Name implements View.
func (j *JobDetail) Name() string {
	if j.job != nil {
		return "Job: " + j.job.JID()
	}
	return "Job Detail"
}

// ShortHelp implements View.
func (j *JobDetail) ShortHelp() []key.Binding {
	return nil
}

// ContextItems implements ContextProvider.
func (j *JobDetail) ContextItems() []ContextItem {
	items := []ContextItem{}
	if j.job != nil {
		items = append(items, ContextItem{Label: "Job", Value: j.job.JID()})
	}
	panel := "Details"
	if j.focusRight {
		panel = "JSON"
	}
	items = append(items, ContextItem{Label: "Panel", Value: panel})
	return items
}

// HintBindings implements HintProvider.
func (j *JobDetail) HintBindings() []key.Binding {
	return []key.Binding{
		helpBinding([]string{"tab"}, "tab", "switch panel"),
		helpBinding([]string{"j"}, "j/k", "scroll"),
		helpBinding([]string{"h"}, "h/l", "scroll left/right"),
	}
}

// HelpSections implements HelpProvider.
func (j *JobDetail) HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Job Detail",
			Bindings: []key.Binding{
				j.KeyMap.SwitchPanel,
				j.KeyMap.LineUp,
				j.KeyMap.LineDown,
				j.KeyMap.ScrollLeft,
				j.KeyMap.ScrollRight,
				j.KeyMap.GotoTop,
				j.KeyMap.GotoBottom,
				j.KeyMap.Home,
				j.KeyMap.End,
			},
		},
	}
}

// SetSize implements View.
func (j *JobDetail) SetSize(width, height int) View {
	j.width = width
	j.height = height
	j.updateDimensions()
	j.clampScroll()
	j.jsonView.SetSize(width, height)
	return j
}

// SetStyles implements View.
func (j *JobDetail) SetStyles(styles Styles) View {
	j.styles = jobDetailStyles{
		Title:           styles.Title,
		Label:           styles.Muted,
		Value:           styles.Text,
		JSON:            styles.Text,
		JSONKey:         styles.JSONKey,
		JSONString:      styles.JSONString,
		JSONNumber:      styles.JSONNumber,
		JSONBool:        styles.JSONBool,
		JSONNull:        styles.JSONNull,
		JSONPunctuation: styles.JSONPunctuation,
		Border:          styles.BorderStyle,
		PanelTitle:      styles.Title,
		FocusBorder:     styles.FocusBorder,
		Muted:           styles.Muted,
		FilterFocused:   styles.FilterFocused,
		FilterBlurred:   styles.FilterBlurred,
	}
	j.jsonView.SetStyles(jsonview.Styles{
		Text:        j.styles.JSON,
		Key:         j.styles.JSONKey,
		String:      j.styles.JSONString,
		Number:      j.styles.JSONNumber,
		Bool:        j.styles.JSONBool,
		Null:        j.styles.JSONNull,
		Punctuation: j.styles.JSONPunctuation,
		Muted:       j.styles.Muted,
	})
	return j
}

// SetJob sets the job to display.
func (j *JobDetail) SetJob(job *sidekiq.JobRecord) {
	j.job = job
	j.leftYOffset = 0
	j.rightYOffset = 0
	j.rightXOffset = 0
	j.focusRight = false

	j.extractProperties()
	j.formatJSON()
}

// Dispose clears cached data when the view is removed from the stack.
func (j *JobDetail) Dispose() {
	j.SetJob(nil)
}

// updateDimensions recalculates panel dimensions.
func (j *JobDetail) updateDimensions() {
	// Split width: 40% left, 60% right (with 1 char gap)
	j.leftWidth = max((j.width*40)/100, 30)
	j.rightWidth = j.width - j.leftWidth

	// Height minus border (2 lines: top and bottom)
	// Note: title is part of the top border, not a separate line
	j.panelHeight = max(j.height-2, 1)
}

func (j *JobDetail) maxLeftYOffset() int {
	maxY := j.countLeftPanelLines() - j.panelHeight
	if maxY < 0 {
		return 0
	}
	return maxY
}

func (j *JobDetail) maxRightYOffset() int {
	maxY := j.jsonView.LineCount() - j.panelHeight
	if maxY < 0 {
		return 0
	}
	return maxY
}

func (j *JobDetail) maxRightXOffset() int {
	contentWidth := max(j.rightWidth-2-2*jobDetailPanelPadding, 0)
	maxX := j.jsonView.MaxWidth() - contentWidth
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
func (j *JobDetail) clampScroll() {
	// Left panel - count actual display lines (with wrapping)
	maxLeftY := j.maxLeftYOffset()
	if j.leftYOffset > maxLeftY {
		j.leftYOffset = maxLeftY
	}

	// Right panel
	maxRightY := j.maxRightYOffset()
	if j.rightYOffset > maxRightY {
		j.rightYOffset = maxRightY
	}

	maxRightX := j.maxRightXOffset()
	if j.rightXOffset > maxRightX {
		j.rightXOffset = maxRightX
	}
}

// countLeftPanelLines counts total display lines in left panel (with wrapping).
func (j *JobDetail) countLeftPanelLines() int {
	if len(j.properties) == 0 {
		return 0
	}

	// Calculate value width (same as in renderLeftPanel)
	innerWidth := j.leftWidth - 2
	contentWidth := max(innerWidth-2*jobDetailPanelPadding, 0)
	valueWidth := max(contentWidth-jobDetailValueIndent, 10)

	count := 0
	for _, prop := range j.properties {
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
func (j *JobDetail) extractProperties() {
	j.properties = nil
	if j.job == nil {
		return
	}

	// Basic properties
	j.properties = append(j.properties, PropertyRow{Label: "JID", Value: j.job.JID()})
	if bid := j.job.Bid(); bid != "" {
		j.properties = append(j.properties, PropertyRow{Label: "BID", Value: bid})
	}
	j.properties = append(j.properties, PropertyRow{Label: "Queue", Value: j.job.Queue()})
	j.properties = append(j.properties, PropertyRow{Label: "Class", Value: j.job.DisplayClass()})

	// Timestamps
	if enqueuedAt := j.job.EnqueuedAt(); enqueuedAt > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Enqueued At",
			Value: formatTimestamp(enqueuedAt),
		})
	}
	if createdAt := j.job.CreatedAt(); createdAt > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Created At",
			Value: formatTimestamp(createdAt),
		})
	}
	if latency := j.job.Latency(); latency > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Latency",
			Value: format.Duration(int64(math.Round(latency))),
		})
	}
	if tags := j.job.Tags(); len(tags) > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Tags",
			Value: strings.Join(tags, ", "),
		})
	}

	// Error info (for retry/dead jobs)
	if j.job.HasError() {
		j.properties = append(j.properties, PropertyRow{Label: "Error Class", Value: j.job.ErrorClass()})
		j.properties = append(j.properties, PropertyRow{Label: "Error", Value: j.job.ErrorMessage()})
	}
	if retryCount := j.job.RetryCount(); retryCount > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Retry Count",
			Value: strconv.Itoa(retryCount),
		})
	}
	if failedAt := j.job.FailedAt(); failedAt > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Failed At",
			Value: formatTimestamp(failedAt),
		})
	}
	if retriedAt := j.job.RetriedAt(); retriedAt > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Retried At",
			Value: formatTimestamp(retriedAt),
		})
	}
	if backtrace := j.job.ErrorBacktrace(); len(backtrace) > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Backtrace",
			Value: strings.Join(backtrace, " | "),
		})
	}

	// Arguments summary
	displayArgs := j.job.DisplayArgs()
	if len(displayArgs) > 0 {
		j.properties = append(j.properties, PropertyRow{
			Label: "Args",
			Value: format.Args(displayArgs),
		})
	}
}

// formatJSON creates pretty-printed JSON lines.
func (j *JobDetail) formatJSON() {
	if j.job == nil {
		j.jsonView.SetValue(nil)
		return
	}
	j.jsonView.SetValue(j.job.Item())
}

// renderLeftPanel renders the properties panel.
func (j *JobDetail) renderLeftPanel() string {
	innerWidth := j.leftWidth - 2 // minus left and right border

	// Calculate available width for values (with 2-space indent)
	contentWidth := max(innerWidth-2*jobDetailPanelPadding, 0)
	valueIndent := strings.Repeat(" ", jobDetailValueIndent)
	valueWidth := max(contentWidth-jobDetailValueIndent, 10)

	// Build all display lines (label on own row, value indented below)
	allLines := make([]string, 0, len(j.properties)*2)
	for _, prop := range j.properties {
		// Label row
		label := j.styles.Label.Render(prop.Label + ":")
		allLines = append(allLines, label)
		// Value rows (indented, wrapped if needed)
		valueLines := wrapText(prop.Value, valueWidth)
		if len(valueLines) == 0 {
			valueLines = []string{""}
		}
		for _, vl := range valueLines {
			allLines = append(allLines, valueIndent+j.styles.Value.Render(vl))
		}
	}

	// Apply vertical scroll
	var contentLines []string
	endY := min(j.leftYOffset+j.panelHeight, len(allLines))
	if j.leftYOffset < len(allLines) {
		contentLines = allLines[j.leftYOffset:endY]
	}

	// Pad to panel height
	for len(contentLines) < j.panelHeight {
		contentLines = append(contentLines, "")
	}
	return frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  j.styles.PanelTitle,
				Muted:  j.styles.Muted,
				Filter: j.styles.FilterFocused,
				Border: j.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  j.styles.PanelTitle,
				Muted:  j.styles.Muted,
				Filter: j.styles.FilterBlurred,
				Border: j.styles.Border,
			},
		}),
		frame.WithTitle("Job Details"),
		frame.WithTitlePadding(0),
		frame.WithContent(strings.Join(contentLines, "\n")),
		frame.WithPadding(jobDetailPanelPadding),
		frame.WithSize(j.leftWidth, j.height),
		frame.WithFocused(!j.focusRight),
	).View()
}

// renderRightPanel renders the JSON panel.
func (j *JobDetail) renderRightPanel() string {
	innerWidth := j.rightWidth - 2 // minus left and right border
	contentWidth := max(innerWidth-2*jobDetailPanelPadding, 0)

	// Content lines with horizontal scroll
	endY := min(j.rightYOffset+j.panelHeight, j.jsonView.LineCount())
	contentCap := 0
	if endY > j.rightYOffset {
		contentCap = endY - j.rightYOffset
	}
	contentLines := make([]string, 0, contentCap)

	for i := j.rightYOffset; i < endY; i++ {
		contentLines = append(contentLines, j.jsonView.RenderLine(i, j.rightXOffset, contentWidth))
	}

	// Pad to panel height
	for len(contentLines) < j.panelHeight {
		contentLines = append(contentLines, "")
	}
	return frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  j.styles.PanelTitle,
				Muted:  j.styles.Muted,
				Filter: j.styles.FilterFocused,
				Border: j.styles.FocusBorder,
			},
			Blurred: frame.StyleState{
				Title:  j.styles.PanelTitle,
				Muted:  j.styles.Muted,
				Filter: j.styles.FilterBlurred,
				Border: j.styles.Border,
			},
		}),
		frame.WithTitle("Job Data (JSON)"),
		frame.WithTitlePadding(0),
		frame.WithMeta(j.styles.Muted.Render("Esc to close")),
		frame.WithMetaPadding(0),
		frame.WithContent(strings.Join(contentLines, "\n")),
		frame.WithPadding(jobDetailPanelPadding),
		frame.WithSize(j.rightWidth, j.height),
		frame.WithFocused(j.focusRight),
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
