// Package frame renders a titled bordered box with optional meta content.
package frame

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// StyleState holds styles for a focus state.
type StyleState struct {
	Title  lipgloss.Style
	Border lipgloss.Style
}

// Styles holds focus-aware styles for a frame.
type Styles struct {
	Focused StyleState
	Blurred StyleState
}

// DefaultStyles returns default styles for a frame.
func DefaultStyles() Styles {
	state := StyleState{
		Title:  lipgloss.NewStyle().Bold(true),
		Border: lipgloss.NewStyle(),
	}
	return Styles{
		Focused: state,
		Blurred: state,
	}
}

// Model defines state for the frame component.
type Model struct {
	styles       Styles
	title        string
	meta         string
	content      string
	width        int
	height       int
	minHeight    int
	padding      int
	titlePadding int
	metaPadding  int
	focused      bool
	border       lipgloss.Border
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new frame model.
func New(opts ...Option) Model {
	m := Model{
		styles:       DefaultStyles(),
		titlePadding: 1,
		border:       lipgloss.RoundedBorder(),
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

// WithTitle sets the title.
func WithTitle(title string) Option {
	return func(m *Model) {
		m.title = title
	}
}

// WithMeta sets the meta content.
func WithMeta(meta string) Option {
	return func(m *Model) {
		m.meta = meta
	}
}

// WithContent sets the content.
func WithContent(content string) Option {
	return func(m *Model) {
		m.content = content
	}
}

// WithSize sets width and height.
func WithSize(width, height int) Option {
	return func(m *Model) {
		m.width = width
		m.height = height
	}
}

// WithMinHeight sets the minimum height.
func WithMinHeight(height int) Option {
	return func(m *Model) {
		m.minHeight = height
	}
}

// WithPadding sets horizontal padding inside the frame.
func WithPadding(padding int) Option {
	return func(m *Model) {
		m.padding = padding
	}
}

// WithTitlePadding sets the title padding.
func WithTitlePadding(padding int) Option {
	return func(m *Model) {
		m.titlePadding = padding
	}
}

// WithMetaPadding sets the meta padding.
func WithMetaPadding(padding int) Option {
	return func(m *Model) {
		m.metaPadding = padding
	}
}

// WithFocused sets the focus state.
func WithFocused(focused bool) Option {
	return func(m *Model) {
		m.focused = focused
	}
}

// WithBorder sets the border characters.
func WithBorder(border lipgloss.Border) Option {
	return func(m *Model) {
		m.border = border
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetTitle sets the title.
func (m *Model) SetTitle(title string) {
	m.title = title
}

// SetMeta sets the meta content.
func (m *Model) SetMeta(meta string) {
	m.meta = meta
}

// SetContent sets the content.
func (m *Model) SetContent(content string) {
	m.content = content
}

// SetSize sets the width and height.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetMinHeight sets the minimum height.
func (m *Model) SetMinHeight(height int) {
	m.minHeight = height
}

// SetPadding sets horizontal padding inside the frame.
func (m *Model) SetPadding(padding int) {
	m.padding = padding
}

// SetTitlePadding sets the title padding.
func (m *Model) SetTitlePadding(padding int) {
	m.titlePadding = padding
}

// SetMetaPadding sets the meta padding.
func (m *Model) SetMetaPadding(padding int) {
	m.metaPadding = padding
}

// Focused returns the focus state.
func (m Model) Focused() bool {
	return m.focused
}

// Focus focuses the frame.
func (m *Model) Focus() {
	m.focused = true
}

// Blur blurs the frame.
func (m *Model) Blur() {
	m.focused = false
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the current height.
func (m Model) Height() int {
	return m.height
}

// MinHeight returns the minimum height.
func (m Model) MinHeight() int {
	return m.minHeight
}

// View renders the frame with the current content.
func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}

	effectiveHeight := m.height
	if m.minHeight > 0 {
		effectiveHeight = max(effectiveHeight, m.minHeight)
	}
	if effectiveHeight <= 0 {
		return ""
	}
	if effectiveHeight < 2 {
		return ""
	}

	state := m.styles.Blurred
	if m.focused {
		state = m.styles.Focused
	}

	innerWidth := max(m.width-2, 0)
	contentHeight := max(effectiveHeight-2, 0)

	top := m.renderTopBorder(state, innerWidth)
	body := m.renderBody(state, innerWidth, contentHeight)
	bottom := m.renderBottomBorder(state, innerWidth)

	if contentHeight == 0 {
		return top + "\n" + bottom
	}

	return top + "\n" + strings.Join(body, "\n") + "\n" + bottom
}

func (m Model) renderTopBorder(state StyleState, innerWidth int) string {
	topLeft := state.Border.Render(m.border.TopLeft)
	topRight := state.Border.Render(m.border.TopRight)
	hBar := state.Border.Render(m.border.Top)

	leftPad := 1
	rightPad := 1
	available := max(innerWidth-leftPad-rightPad, 0)

	title := padLabel(m.title, m.titlePadding)
	styledTitle := state.Title.Render(title)
	titleWidth := lipgloss.Width(styledTitle)

	meta := padLabel(m.meta, m.metaPadding)
	if meta != "" {
		meta = state.Border.Render("╖") + meta + state.Border.Render("╓")
	}
	metaWidth := lipgloss.Width(meta)

	if titleWidth+metaWidth > available {
		excess := titleWidth + metaWidth - available
		if metaWidth > 0 {
			reduce := min(excess, metaWidth)
			meta = ""
			metaWidth = 0
			excess -= reduce
		}
		if excess > 0 && titleWidth > 0 {
			target := max(titleWidth-excess, 0)
			title = lipgloss.NewStyle().Width(target).MaxWidth(target).Render(title)
			styledTitle = state.Title.Render(title)
			titleWidth = lipgloss.Width(styledTitle)
		}
	}

	remaining := max(available-titleWidth-metaWidth, 0)

	return topLeft +
		strings.Repeat(hBar, leftPad) +
		styledTitle +
		strings.Repeat(hBar, remaining) +
		meta +
		strings.Repeat(hBar, rightPad) +
		topRight
}

func (m Model) renderBottomBorder(state StyleState, innerWidth int) string {
	bottomLeft := state.Border.Render(m.border.BottomLeft)
	bottomRight := state.Border.Render(m.border.BottomRight)
	hBar := state.Border.Render(m.border.Bottom)
	return bottomLeft + strings.Repeat(hBar, innerWidth) + bottomRight
}

func (m Model) renderBody(state StyleState, innerWidth, contentHeight int) []string {
	if contentHeight <= 0 {
		return nil
	}

	lines := strings.Split(m.content, "\n")
	body := make([]string, 0, contentHeight)

	vBar := state.Border.Render(m.border.Left)
	vBarRight := state.Border.Render(m.border.Right)

	for i := range contentHeight {
		var line string
		if i < len(lines) {
			line = lines[i]
		}

		line = padLine(line, innerWidth, m.padding)
		body = append(body, vBar+line+vBarRight)
	}

	return body
}

func padLine(line string, width, padding int) string {
	if width <= 0 {
		return ""
	}

	if padding > 0 {
		spaces := strings.Repeat(" ", padding)
		line = spaces + line + spaces
	}

	lineWidth := lipgloss.Width(line)
	if lineWidth < width {
		line += strings.Repeat(" ", width-lineWidth)
	}
	return line
}

func padLabel(label string, padding int) string {
	if label == "" || padding <= 0 {
		return label
	}
	spaces := strings.Repeat(" ", padding)
	return spaces + label + spaces
}
