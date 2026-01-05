// Package help provides a keybindings help dialog.
package help

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

// DialogID identifies the help dialog.
const DialogID dialogs.DialogID = "help"

// Column describes which column to render a section into.
type Column int

const (
	// ColumnAuto lets the dialog decide placement.
	ColumnAuto Column = iota
	// ColumnLeft forces placement in the left column.
	ColumnLeft
	// ColumnRight forces placement in the right column.
	ColumnRight
)

// Section groups bindings or custom lines under a title.
type Section struct {
	Title    string
	Bindings []key.Binding
	Lines    []string
	Column   Column
}

// Styles holds the styles used by the help dialog.
type Styles struct {
	Title   lipgloss.Style
	Border  lipgloss.Style
	Section lipgloss.Style
	Key     lipgloss.Style
	Desc    lipgloss.Style
	Muted   lipgloss.Style
}

// DefaultStyles returns zero-value styles.
func DefaultStyles() Styles {
	return Styles{}
}

// Model defines state for the help dialog component.
type Model struct {
	styles       Styles
	sections     []Section
	width        int
	height       int
	windowWidth  int
	windowHeight int
	row          int
	col          int
	yOffset      int
	padding      int
	minWidth     int
	minHeight    int
	columnGap    int
}

// Option configures the help dialog.
type Option func(*Model)

// New creates a new help dialog model.
func New(opts ...Option) *Model {
	m := &Model{
		styles:    DefaultStyles(),
		padding:   1,
		minWidth:  64,
		minHeight: 12,
		columnGap: 4,
	}

	for _, opt := range opts {
		opt(m)
	}

	m.applySize()
	return m
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}

// WithSections sets the help sections.
func WithSections(sections []Section) Option {
	return func(m *Model) { m.sections = sections }
}

// Init implements dialogs.DialogModel.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles input and dialog lifecycle.
func (m *Model) Update(msg tea.Msg) (dialogs.DialogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.applySize()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "?", "esc":
			return m, func() tea.Msg { return dialogs.CloseDialogMsg{} }
		case "up", "k":
			m.scrollBy(-1)
			return m, nil
		case "down", "j":
			m.scrollBy(1)
			return m, nil
		case "pgup":
			m.scrollBy(-m.pageSize())
			return m, nil
		case "pgdown":
			m.scrollBy(m.pageSize())
			return m, nil
		case "home":
			m.scrollTo(0)
			return m, nil
		case "end":
			m.scrollTo(m.maxOffset())
			return m, nil
		}
	}

	return m, nil
}

// View renders the help dialog.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	contentWidth := m.contentWidth()
	contentHeight := m.contentHeight()
	lines := m.renderColumnLines(contentWidth)
	m.clampOffset(len(lines), contentHeight)
	if contentHeight > 0 && m.yOffset < len(lines) {
		end := min(m.yOffset+contentHeight, len(lines))
		lines = lines[m.yOffset:end]
	} else if contentHeight > 0 {
		lines = nil
	}
	content := strings.Join(lines, "\n")

	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Muted,
				Border: m.styles.Border,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Muted,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle("Help"),
		frame.WithTitlePadding(0),
		frame.WithContent(content),
		frame.WithPadding(m.padding),
		frame.WithSize(m.width, m.height),
		frame.WithMinHeight(5),
		frame.WithFocused(true),
	)
	return box.View()
}

// Position returns the dialog position.
func (m *Model) Position() (int, int) {
	return m.row, m.col
}

// ID returns the dialog ID.
func (m *Model) ID() dialogs.DialogID {
	return DialogID
}

func (m *Model) applySize() {
	if m.windowWidth == 0 || m.windowHeight == 0 {
		return
	}

	dialogWidth := max((m.windowWidth*2)/3, m.minWidth)
	dialogWidth = min(dialogWidth, m.windowWidth-4)
	if dialogWidth < 10 {
		dialogWidth = max(m.windowWidth-2, 10)
	}

	dialogHeight := max(m.windowHeight/2, m.minHeight)
	dialogHeight = min(dialogHeight, m.windowHeight-4)
	if dialogHeight < 5 {
		dialogHeight = max(m.windowHeight-2, 5)
	}

	m.width = dialogWidth
	m.height = dialogHeight
	m.row = max((m.windowHeight-dialogHeight)/2, 0)
	m.col = max((m.windowWidth-dialogWidth)/2, 0)
	m.scrollTo(m.yOffset)
}

func (m *Model) renderColumnLines(width int) []string {
	if width <= 0 || len(m.sections) == 0 {
		return nil
	}

	left, right := splitSections(m.sections)
	gap := m.columnGap
	if width <= gap+10 {
		gap = 2
	}
	columnWidth := max((width-gap)/2, 1)
	leftLines := renderSections(left, columnWidth, m.styles)
	rightLines := renderSections(right, columnWidth, m.styles)

	lines := make([]string, 0, max(len(leftLines), len(rightLines)))
	rows := max(len(leftLines), len(rightLines))
	for i := range rows {
		leftLine := ""
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		rightLine := ""
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}

		leftLine = padRight(leftLine, columnWidth)
		rightLine = padRight(rightLine, columnWidth)
		lines = append(lines, leftLine+strings.Repeat(" ", gap)+rightLine)
	}

	return lines
}

func (m *Model) contentWidth() int {
	return max(m.width-2-(m.padding*2), 1)
}

func (m *Model) contentHeight() int {
	effectiveHeight := m.height
	if m.minHeight > 0 {
		effectiveHeight = max(effectiveHeight, m.minHeight)
	}
	return max(effectiveHeight-2, 0)
}

func (m *Model) pageSize() int {
	height := m.contentHeight()
	if height <= 1 {
		return 1
	}
	return height - 1
}

func (m *Model) maxOffset() int {
	totalLines := len(m.renderColumnLines(m.contentWidth()))
	visible := m.contentHeight()
	if totalLines <= visible || visible <= 0 {
		return 0
	}
	return totalLines - visible
}

func (m *Model) clampOffset(totalLines, visible int) {
	if visible <= 0 || totalLines <= visible {
		m.yOffset = 0
		return
	}
	maxOffset := totalLines - visible
	if m.yOffset > maxOffset {
		m.yOffset = maxOffset
	}
	if m.yOffset < 0 {
		m.yOffset = 0
	}
}

func (m *Model) scrollBy(delta int) {
	if delta == 0 {
		return
	}
	maxOffset := m.maxOffset()
	if maxOffset == 0 {
		m.yOffset = 0
		return
	}
	m.scrollTo(m.yOffset + delta)
}

func (m *Model) scrollTo(offset int) {
	if offset < 0 {
		offset = 0
	}
	maxOffset := m.maxOffset()
	if offset > maxOffset {
		offset = maxOffset
	}
	m.yOffset = offset
}

func splitSections(sections []Section) ([]Section, []Section) {
	if len(sections) == 0 {
		return nil, nil
	}

	left := []Section{}
	right := []Section{}
	auto := []Section{}
	for _, section := range sections {
		switch section.Column {
		case ColumnAuto:
			auto = append(auto, section)
		case ColumnLeft:
			left = append(left, section)
		case ColumnRight:
			right = append(right, section)
		}
	}

	hasLeft := len(left) > 0
	hasRight := len(right) > 0
	for _, section := range auto {
		switch {
		case hasLeft && !hasRight:
			right = append(right, section)
		case hasRight && !hasLeft:
			left = append(left, section)
		default:
			if len(left) <= len(right) {
				left = append(left, section)
			} else {
				right = append(right, section)
			}
		}
	}

	return left, right
}

func renderSections(sections []Section, width int, styles Styles) []string {
	if len(sections) == 0 || width <= 0 {
		return nil
	}

	lines := make([]string, 0, len(sections)*4)
	keyPadWidth := max(ansi.StringWidth(styles.Key.Render("x"))-ansi.StringWidth("x"), 0)

	for i, section := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		title := strings.TrimSpace(section.Title)
		if title != "" {
			lines = append(lines, ansi.Truncate(styles.Section.Render(title), width, ""))
		}

		if len(section.Lines) > 0 {
			for _, line := range section.Lines {
				lines = append(lines, ansi.Truncate(line, width, ""))
			}
		}

		keys := make([]string, 0, len(section.Bindings))
		descs := make([]string, 0, len(section.Bindings))
		keyWidths := make([]int, 0, len(section.Bindings))
		maxKeyWidth := 0
		for _, binding := range section.Bindings {
			if !binding.Enabled() {
				continue
			}
			help := binding.Help()
			keyText := strings.TrimSpace(help.Key)
			if keyText == "" {
				continue
			}
			keyWidth := ansi.StringWidth(keyText)
			maxKeyWidth = max(maxKeyWidth, keyWidth)
			keys = append(keys, keyText)
			descs = append(descs, strings.TrimSpace(help.Desc))
			keyWidths = append(keyWidths, keyWidth)
		}
		if len(keys) == 0 {
			continue
		}

		if len(section.Lines) > 0 {
			lines = append(lines, "")
		}

		keyCellWidth := maxKeyWidth + keyPadWidth
		for i, keyText := range keys {
			keyRendered := styles.Key.Render(keyText)
			if keyCellWidth > keyWidths[i]+keyPadWidth {
				keyRendered += strings.Repeat(" ", keyCellWidth-(keyWidths[i]+keyPadWidth))
			}
			if descs[i] == "" {
				lines = append(lines, ansi.Truncate(keyRendered, width, ""))
				continue
			}
			line := keyRendered + " " + styles.Desc.Render(descs[i])
			lines = append(lines, ansi.Truncate(line, width, ""))
		}
	}
	return lines
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}

	stringWidth := ansi.StringWidth(value)
	if stringWidth == width {
		return value
	}
	if stringWidth > width {
		return ansi.Truncate(value, width, "")
	}
	return value + strings.Repeat(" ", width-stringWidth)
}
