// Package contextbar renders a fixed-height contextual header with key hints.
package contextbar

import (
	"fmt"
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

// KeyValueItem renders a label/value pair.
type KeyValueItem struct {
	Label string
	Value string
}

// Render renders the key/value item.
func (i KeyValueItem) Render(styles Styles) string {
	return renderKeyValueLine(styles, i.Label, i.Value, 0)
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
	Bar       lipgloss.Style
	Label     lipgloss.Style
	Value     lipgloss.Style
	Muted     lipgloss.Style
	Key       lipgloss.Style
	Desc      lipgloss.Style
	Separator lipgloss.Style
}

// DefaultStyles returns default styles for the context bar.
func DefaultStyles() Styles {
	return Styles{
		Bar:       lipgloss.NewStyle().Padding(0, 1),
		Label:     lipgloss.NewStyle(),
		Value:     lipgloss.NewStyle(),
		Muted:     lipgloss.NewStyle(),
		Key:       lipgloss.NewStyle(),
		Desc:      lipgloss.NewStyle(),
		Separator: lipgloss.NewStyle(),
	}
}

// Model defines state for the context bar component.
type Model struct {
	styles Styles
	items  []Item
	hints  []key.Binding
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
func WithHints(hints []key.Binding) Option {
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
func (m *Model) SetHints(hints []key.Binding) { m.hints = hints }

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

	leftLines := m.buildItemLines(innerWidth)
	maxLeftWidth := 0
	for _, line := range leftLines {
		maxLeftWidth = max(maxLeftWidth, ansi.StringWidth(line))
	}
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

func (m Model) buildItemLines(width int) []string {
	if width <= 0 || len(m.items) == 0 {
		return nil
	}
	labelWidth := 0
	for _, item := range m.items {
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

	lines := make([]string, 0, min(len(m.items), m.height))
	for i := 0; i < len(m.items) && len(lines) < m.height; i++ {
		item := m.items[i]
		line := ""
		switch v := item.(type) {
		case KeyValueItem:
			line = renderKeyValueLine(m.styles, v.Label, v.Value, labelWidth)
		case *KeyValueItem:
			line = renderKeyValueLine(m.styles, v.Label, v.Value, labelWidth)
		default:
			line = item.Render(m.styles)
		}
		line = ansi.Truncate(line, width, "")
		lines = append(lines, line)
	}
	return lines
}

func renderKeyValueLine(styles Styles, label, value string, labelWidth int) string {
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if label == "" && value == "" {
		return ""
	}
	if value == "" {
		value = "—"
	}

	valueStyle := styles.Value.Bold(true)
	mutedStyle := styles.Muted.Bold(true)

	if label == "" {
		if value == "—" {
			return mutedStyle.Render(value)
		}
		return valueStyle.Render(value)
	}

	labelText := label + ":"
	if labelWidth > 0 {
		labelText = fmt.Sprintf("%-*s", labelWidth, labelText)
	}
	if value == "—" {
		return styles.Label.Render(labelText) + " " + mutedStyle.Render(value)
	}
	return styles.Label.Render(labelText) + " " + valueStyle.Render(value)
}

func (m Model) buildHintLines(width int) []string {
	if width <= 0 || len(m.hints) == 0 {
		return nil
	}
	type hintItem struct {
		keyStyled string
		keyWidth  int
		desc      string
	}

	items := make([]hintItem, 0, len(m.hints))
	maxKeyWidth := 0
	keyStyle := m.styles.Key.Padding(0, 0)
	for _, hint := range m.hints {
		if !hint.Enabled() {
			continue
		}
		help := hint.Help()
		keyText := strings.TrimSpace(help.Key)
		descText := strings.TrimSpace(help.Desc)
		if keyText == "" {
			continue
		}
		displayKey := " " + keyText + " "
		keyStyled := keyStyle.Render(displayKey)
		keyWidth := ansi.StringWidth(displayKey)
		maxKeyWidth = max(maxKeyWidth, keyWidth)
		items = append(items, hintItem{keyStyled: keyStyled, keyWidth: keyWidth, desc: descText})
	}
	if len(items) == 0 {
		return nil
	}
	rows := max(m.height, 1)
	rows = min(rows, len(items))
	lines := make([]string, 0, rows)
	maxLineWidth := 0
	for i := range rows {
		item := items[i]
		keyCell := item.keyStyled + strings.Repeat(" ", maxKeyWidth-item.keyWidth)
		line := keyCell
		if item.desc != "" {
			line += " " + m.styles.Desc.Render(item.desc)
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
