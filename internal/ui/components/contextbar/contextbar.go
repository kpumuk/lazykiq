// Package contextbar renders a fixed-height contextual header with key hints.
package contextbar

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
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
	key  string
	desc string
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

	layout := m.calculateLayout(innerWidth)
	return m.renderLines(layout, innerWidth, barStyle)
}

type layoutResult struct {
	leftLines  []string
	rightLines []string
	leftWidth  int
	rightWidth int
	gap        int
}

func (m Model) calculateLayout(innerWidth int) layoutResult {
	labelWidth := maxLabelWidth(m.items)
	leftLinesNatural := m.buildItemLines(innerWidth, labelWidth)
	leftWidthNatural := maxLineWidth(leftLinesNatural)

	rightLines := m.buildHintLines(innerWidth)
	rightWidthNatural := maxLineWidth(rightLines)

	minGap := max(m.gap, 1)
	totalNatural := leftWidthNatural + rightWidthNatural

	switch {
	case totalNatural+1 <= innerWidth:
		// Both fit: calculate gap (at least 1, maximize if possible)
		gap := innerWidth - totalNatural
		if gap < minGap && totalNatural+minGap <= innerWidth {
			gap = minGap
		}
		return layoutResult{leftLinesNatural, rightLines, leftWidthNatural, rightWidthNatural, gap}
	case leftWidthNatural+1 < innerWidth:
		// Truncate shortcuts, keep context full
		rightWidth := innerWidth - leftWidthNatural - 1
		if rightWidth > 0 {
			rightLines = m.buildHintLines(rightWidth)
			return layoutResult{leftLinesNatural, rightLines, leftWidthNatural, rightWidth, 1}
		}
		return layoutResult{leftLinesNatural, nil, leftWidthNatural, 0, 0}
	default:
		// Truncate context, no shortcuts
		leftLines := m.buildItemLines(innerWidth, labelWidth)
		return layoutResult{leftLines, nil, innerWidth, 0, 0}
	}
}

func (m Model) renderLines(layout layoutResult, innerWidth int, barStyle lipgloss.Style) string {
	lines := make([]string, 0, m.height)
	for i := range m.height {
		var line string
		if layout.rightWidth > 0 {
			left := getLine(layout.leftLines, i)
			right := strings.TrimLeft(getLine(layout.rightLines, i), " ")
			line = padRight(left, layout.leftWidth) + strings.Repeat(" ", layout.gap) + right
			if w := layout.leftWidth + layout.gap + ansi.StringWidth(right); w < innerWidth {
				line = padRight(line, innerWidth)
			}
		} else if layout.leftWidth > 0 {
			line = padRight(getLine(layout.leftLines, i), innerWidth)
		}
		lines = append(lines, barStyle.Render(line))
	}
	return strings.Join(lines, "\n")
}

func getLine(lines []string, index int) string {
	if index < len(lines) {
		return lines[index]
	}
	return ""
}

func maxLineWidth(lines []string) int {
	width := 0
	for _, line := range lines {
		width = max(width, ansi.StringWidth(line))
	}
	return width
}

func (m Model) buildItemLines(width, labelWidth int) []string {
	if width <= 0 || len(m.items) == 0 {
		return nil
	}

	styles := m.styles
	if labelWidth > 0 {
		styles.Label = styles.Label.Width(labelWidth + 1)
	}

	lines := make([]string, 0, min(len(m.items), m.height))
	for i := 0; i < len(m.items) && len(lines) < m.height; i++ {
		lines = append(lines, ansi.Truncate(m.items[i].Render(styles), width, ""))
	}
	return lines
}

func maxLabelWidth(items []Item) int {
	labelWidth := 0
	for _, item := range items {
		var label string
		switch v := item.(type) {
		case KeyValueItem:
			label = strings.TrimSpace(v.Label)
		case *KeyValueItem:
			label = strings.TrimSpace(v.Label)
		}
		if label != "" {
			labelWidth = max(labelWidth, ansi.StringWidth(label+":"))
		}
	}
	return labelWidth
}

func (m Model) buildHintLines(width int) []string {
	if width <= 0 || len(m.hints) == 0 {
		return nil
	}

	normalItems, dangerItems := m.separateHints()
	if len(normalItems)+len(dangerItems) == 0 {
		return nil
	}

	tbl := table.New().
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).BorderRow(false)

	if len(normalItems) > 0 && len(dangerItems) > 0 {
		m.buildTwoColumnTable(tbl, normalItems, dangerItems, max(len(normalItems), len(dangerItems)))
	} else {
		m.buildSingleColumnTable(tbl, normalItems, dangerItems)
	}

	lines := strings.Split(tbl.String(), "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return lines
}

func (m Model) separateHints() (normal, danger []hintItem) {
	rows := max(m.height, 1)
	for _, hint := range m.hints {
		if !hint.Binding.Enabled() {
			continue
		}
		help := hint.Binding.Help()
		if key := strings.TrimSpace(help.Key); key != "" {
			item := hintItem{key: key, desc: strings.TrimSpace(help.Desc)}
			if hint.Kind == HintDanger {
				if len(danger) < rows {
					danger = append(danger, item)
				}
			} else if len(normal) < rows {
				normal = append(normal, item)
			}
		}
	}
	return
}

func (m Model) buildTwoColumnTable(tbl *table.Table, normalItems, dangerItems []hintItem, maxRows int) {
	normalKeyWidth := maxWidth(normalItems, func(h hintItem) int { return ansi.StringWidth(h.key) })
	dangerKeyWidth := maxWidth(dangerItems, func(h hintItem) int { return ansi.StringWidth(h.key) })

	for i := range maxRows {
		normalCell := m.formatCell(normalItems, i, normalKeyWidth, m.styles.Key, m.styles.Desc)
		dangerCell := m.formatCell(dangerItems, i, dangerKeyWidth, m.styles.DangerKey, m.styles.DangerDesc)
		tbl.Row(normalCell, dangerCell)
	}
	tbl.StyleFunc(func(_, col int) lipgloss.Style {
		if col == 0 {
			return lipgloss.NewStyle().PaddingRight(m.gap)
		}
		return lipgloss.NewStyle()
	})
}

func (m Model) buildSingleColumnTable(tbl *table.Table, normalItems, dangerItems []hintItem) {
	items, keyStyle, descStyle := normalItems, m.styles.Key, m.styles.Desc
	if len(normalItems) == 0 {
		items, keyStyle, descStyle = dangerItems, m.styles.DangerKey, m.styles.DangerDesc
	}
	keyWidth := maxWidth(items, func(h hintItem) int { return ansi.StringWidth(h.key) })
	for _, item := range items {
		tbl.Row(m.formatHintItem(item, keyWidth, keyStyle, descStyle))
	}
}

func (m Model) formatCell(items []hintItem, index, keyWidth int, keyStyle, descStyle lipgloss.Style) string {
	if index < len(items) {
		return m.formatHintItem(items[index], keyWidth, keyStyle, descStyle)
	}
	return ""
}

func (m Model) formatHintItem(item hintItem, keyWidth int, keyStyle, descStyle lipgloss.Style) string {
	key := keyStyle.Render(item.key)
	if item.desc == "" {
		return key
	}
	return key + strings.Repeat(" ", max(0, keyWidth-ansi.StringWidth(item.key))) + " " + descStyle.Render(item.desc)
}

func maxWidth[T any](items []T, widthFunc func(T) int) int {
	width := 0
	for _, item := range items {
		width = max(width, widthFunc(item))
	}
	return width
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	truncated := ansi.Truncate(value, width, "")
	if w := ansi.StringWidth(truncated); w < width {
		return truncated + strings.Repeat(" ", width-w)
	}
	return truncated
}
