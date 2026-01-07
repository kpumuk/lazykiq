// Package contextbar renders a fixed-height contextual header with key hints.
package contextbar

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Item renders a line in the context bar.
type Item interface {
	Render(styles Styles) string
}

// HintKind describes the visual treatment for a hint.
type HintKind int

const (
	// HintNormal renders with standard hint styles.
	HintNormal HintKind = iota
	// HintDanger highlights mutational operations.
	HintDanger
)

// Hint describes a keybinding to render in the context bar.
type Hint struct {
	Binding key.Binding
	Kind    HintKind
}

type hintItem struct {
	displayKey string
	keyWidth   int
	desc       string
}

// KeyValueItem renders a label/value pair.
type KeyValueItem struct {
	Label string
	Value string
}

// Render renders the key/value item.
func (i KeyValueItem) Render(styles Styles) string {
	label := strings.TrimSpace(i.Label)
	value := strings.TrimSpace(i.Value)
	if label == "" && value == "" {
		return ""
	}
	if value == "" {
		value = "—"
	}

	valueStyle := styles.Value.Bold(true)
	if label == "" {
		return valueStyle.Render(value)
	}

	labelText := label + ":"
	labelStyle := styles.Label
	if labelStyle.GetWidth() > 0 {
		return labelStyle.Render(labelText) + valueStyle.Render(value)
	}
	return labelStyle.Render(labelText) + " " + valueStyle.Render(value)
}

// FormattedItem renders a pre-formatted line.
type FormattedItem struct {
	Line string
}

// Render returns the pre-formatted line.
func (i FormattedItem) Render(_ Styles) string {
	return i.Line
}

// Styles holds the styles for rendering the context bar.
type Styles struct {
	Bar        lipgloss.Style
	Label      lipgloss.Style
	Value      lipgloss.Style
	Key        lipgloss.Style
	Desc       lipgloss.Style
	DangerKey  lipgloss.Style
	DangerDesc lipgloss.Style
}

// DefaultStyles returns default styles for the context bar.
func DefaultStyles() Styles {
	return Styles{
		Bar:        lipgloss.NewStyle().Padding(0, 1),
		Label:      lipgloss.NewStyle(),
		Value:      lipgloss.NewStyle(),
		Key:        lipgloss.NewStyle(),
		Desc:       lipgloss.NewStyle(),
		DangerKey:  lipgloss.NewStyle(),
		DangerDesc: lipgloss.NewStyle(),
	}
}

// Model defines state for the context bar component.
type Model struct {
	styles Styles
	items  []Item
	hints  []Hint
	width  int
	height int
	gap    int
}

// Option configures the context bar.
type Option func(*Model)

// New creates a new context bar model.
func New(opts ...Option) Model {
	m := Model{
		styles: DefaultStyles(),
		height: 5,
		gap:    2,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}

// WithItems sets the contextual items.
func WithItems(items []Item) Option {
	return func(m *Model) { m.items = items }
}

// WithHints sets the key hints.
func WithHints(hints []Hint) Option {
	return func(m *Model) { m.hints = hints }
}

// WithSize sets width and height.
func WithSize(width, height int) Option {
	return func(m *Model) {
		m.width = width
		m.height = height
	}
}

// WithHeight sets the fixed height.
func WithHeight(height int) Option {
	return func(m *Model) { m.height = height }
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) { m.styles = s }

// SetItems updates the contextual items.
func (m *Model) SetItems(items []Item) { m.items = items }

// SetHints updates the key hints.
func (m *Model) SetHints(hints []Hint) { m.hints = hints }

// SetWidth updates the width.
func (m *Model) SetWidth(width int) { m.width = width }

// SetHeight updates the height.
func (m *Model) SetHeight(height int) { m.height = height }

// Width returns the current width.
func (m Model) Width() int { return m.width }

// Height returns the current height.
func (m Model) Height() int { return m.height }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(_ tea.Msg) (Model, tea.Cmd) { return m, nil }

// View renders the context bar.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	barStyle := m.styles.Bar.Width(m.width)
	_, rightPad, _, leftPad := barStyle.GetPadding()
	innerWidth := m.width - leftPad - rightPad
	if innerWidth <= 0 {
		return strings.TrimRight(strings.Repeat(barStyle.Render("")+"\n", m.height), "\n")
	}

	labelWidth := maxLabelWidth(m.items)
	leftLines, maxLeftWidth := m.buildItemLines(innerWidth, labelWidth)
	leftWidth := min(maxLeftWidth, innerWidth)
	if leftWidth > 0 {
		leftWidth = min(leftWidth, max(innerWidth-2, 0))
	}

	availableForRight := max(innerWidth-leftWidth-2, 0)
	rightLines := m.buildHintLines(availableForRight)
	maxRightWidth := 0
	for _, line := range rightLines {
		maxRightWidth = max(maxRightWidth, ansi.StringWidth(line))
	}
	rightWidth := min(maxRightWidth, availableForRight)
	if maxRightWidth > availableForRight {
		rightLines = m.buildHintLines(availableForRight)
		maxRightWidth = 0
		for _, line := range rightLines {
			maxRightWidth = max(maxRightWidth, ansi.StringWidth(line))
		}
		rightWidth = min(maxRightWidth, availableForRight)
	}

	lines := make([]string, 0, m.height)
	for i := range m.height {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}

		var line string
		switch {
		case leftWidth > 0 && rightWidth > 0:
			left = padRight(left, leftWidth)
			right = padLeft(right, rightWidth)
			spaceWidth := max(innerWidth-leftWidth-rightWidth, 2)
			line = left + strings.Repeat(" ", spaceWidth) + right
		case leftWidth > 0:
			line = padRight(left, leftWidth)
		case rightWidth > 0:
			line = padLeft(right, rightWidth)
		default:
			line = ""
		}

		lines = append(lines, barStyle.Render(line))
	}

	return strings.Join(lines, "\n")
}

func (m Model) buildItemLines(width int, labelWidth int) ([]string, int) {
	if width <= 0 || len(m.items) == 0 {
		return nil, 0
	}

	styles := m.styles
	if labelWidth > 0 {
		styles.Label = styles.Label.Width(labelWidth + 1)
	}

	lines := make([]string, 0, min(len(m.items), m.height))
	maxLineWidth := 0
	for i := 0; i < len(m.items) && len(lines) < m.height; i++ {
		line := m.items[i].Render(styles)
		line = ansi.Truncate(line, width, "")
		maxLineWidth = max(maxLineWidth, ansi.StringWidth(line))
		lines = append(lines, line)
	}
	return lines, maxLineWidth
}

func maxLabelWidth(items []Item) int {
	labelWidth := 0
	for _, item := range items {
		label := ""
		switch v := item.(type) {
		case KeyValueItem:
			label = v.Label
		case *KeyValueItem:
			label = v.Label
		}
		if label == "" {
			continue
		}
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		w := ansi.StringWidth(label + ":")
		if w > labelWidth {
			labelWidth = w
		}
	}
	return labelWidth
}

func (m Model) buildHintLines(width int) []string {
	if width <= 0 || len(m.hints) == 0 {
		return nil
	}
	normalItems := make([]hintItem, 0, len(m.hints))
	dangerItems := make([]hintItem, 0, len(m.hints))
	for _, hint := range m.hints {
		if !hint.Binding.Enabled() {
			continue
		}
		help := hint.Binding.Help()
		keyText := strings.TrimSpace(help.Key)
		descText := strings.TrimSpace(help.Desc)
		if keyText == "" {
			continue
		}
		displayKey := keyText
		item := hintItem{displayKey: displayKey, keyWidth: ansi.StringWidth(displayKey), desc: descText}
		if hint.Kind == HintDanger {
			dangerItems = append(dangerItems, item)
		} else {
			normalItems = append(normalItems, item)
		}
	}
	if len(normalItems) == 0 && len(dangerItems) == 0 {
		return nil
	}

	if len(dangerItems) == 0 {
		lines, _ := m.buildHintColumn(normalItems, width, m.styles.Key, m.styles.Desc)
		return lines
	}
	if len(normalItems) == 0 {
		lines, _ := m.buildHintColumn(dangerItems, width, m.styles.DangerKey, m.styles.DangerDesc)
		return lines
	}

	gap := max(m.gap, 1)
	normalLines, normalWidth := m.buildHintColumn(normalItems, width, m.styles.Key, m.styles.Desc)
	dangerLines, dangerWidth := m.buildHintColumn(dangerItems, width, m.styles.DangerKey, m.styles.DangerDesc)
	if normalWidth+gap+dangerWidth > width {
		normalWidth = min(normalWidth, max(width-gap, 0))
		dangerWidth = min(dangerWidth, max(width-normalWidth-gap, 0))
		normalLines, _ = m.buildHintColumn(normalItems, normalWidth, m.styles.Key, m.styles.Desc)
		dangerLines, _ = m.buildHintColumn(dangerItems, dangerWidth, m.styles.DangerKey, m.styles.DangerDesc)
	}

	rows := max(m.height, 1)
	rows = min(rows, max(len(normalLines), len(dangerLines)))
	lines := make([]string, 0, rows)
	maxLineWidth := 0
	for i := range rows {
		left := ""
		right := ""
		if i < len(normalLines) {
			left = padRight(normalLines[i], normalWidth)
		}
		if i < len(dangerLines) {
			right = padRight(dangerLines[i], dangerWidth)
		}
		line := strings.TrimRight(left, " ")
		if line != "" {
			line = padRight(line, normalWidth)
		}
		if right != "" {
			if line != "" {
				line += strings.Repeat(" ", gap) + right
			} else {
				line = right
			}
		}
		lines = append(lines, line)
		maxLineWidth = max(maxLineWidth, ansi.StringWidth(line))
	}
	if maxLineWidth > width {
		maxLineWidth = width
	}
	for i := range lines {
		line := lines[i]
		if ansi.StringWidth(line) > maxLineWidth {
			line = ansi.Truncate(line, maxLineWidth, "")
		}
		lines[i] = padRight(line, maxLineWidth)
	}
	return lines
}

func (m Model) buildHintColumn(items []hintItem, width int, keyStyle, descStyle lipgloss.Style) ([]string, int) {
	if width <= 0 || len(items) == 0 {
		return nil, 0
	}
	maxKeyWidth := 0
	for _, item := range items {
		maxKeyWidth = max(maxKeyWidth, item.keyWidth)
	}

	rows := max(m.height, 1)
	rows = min(rows, len(items))
	lines := make([]string, 0, rows)
	maxLineWidth := 0
	for i := range rows {
		item := items[i]
		keyCell := keyStyle.Render(item.displayKey) + strings.Repeat(" ", maxKeyWidth-item.keyWidth)
		line := keyCell
		if item.desc != "" {
			line += " " + descStyle.Render(item.desc)
		}
		lines = append(lines, line)
		maxLineWidth = max(maxLineWidth, ansi.StringWidth(line))
	}
	if maxLineWidth > width {
		maxLineWidth = width
	}
	for i := range lines {
		line := lines[i]
		if ansi.StringWidth(line) > maxLineWidth {
			line = ansi.Truncate(line, maxLineWidth, "")
		}
		lines[i] = padRight(line, maxLineWidth)
	}
	return lines, maxLineWidth
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	truncated := ansi.Truncate(value, width, "")
	if ansi.StringWidth(truncated) >= width {
		return truncated
	}
	return truncated + strings.Repeat(" ", width-ansi.StringWidth(truncated))
}

func padLeft(value string, width int) string {
	if width <= 0 {
		return ""
	}
	truncated := ansi.Truncate(value, width, "")
	if ansi.StringWidth(truncated) >= width {
		return truncated
	}
	return strings.Repeat(" ", width-ansi.StringWidth(truncated)) + truncated
}
