// Package scrollbar renders a vertical scrollbar.
package scrollbar

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	// DefaultWidth is the default scrollbar width.
	DefaultWidth = 1

	scrollbarThumb = "█"
	scrollbarTrack = "░"
)

// Styles holds the styles needed for the scrollbar.
type Styles struct {
	Track lipgloss.Style
	Thumb lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for this scrollbar.
func DefaultStyles() Styles {
	return Styles{
		Track: lipgloss.NewStyle(),
		Thumb: lipgloss.NewStyle(),
	}
}

// Model is a vertical scrollbar component.
type Model struct {
	styles  Styles
	width   int
	height  int
	total   int
	visible int
	offset  int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new scrollbar model.
func New(opts ...Option) Model {
	m := Model{
		styles: DefaultStyles(),
		width:  DefaultWidth,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// WithStyles sets the scrollbar styles.
func WithStyles(s Styles) Option {
	return func(m *Model) {
		m.styles = s
	}
}

// WithSize sets the scrollbar width and height.
func WithSize(width, height int) Option {
	return func(m *Model) {
		m.width = width
		m.height = height
	}
}

// WithRange sets total, visible, and offset values.
func WithRange(total, visible, offset int) Option {
	return func(m *Model) {
		m.total = total
		m.visible = visible
		m.offset = offset
	}
}

// SetStyles updates the scrollbar styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize updates the scrollbar width and height.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetRange updates total, visible, and offset values.
func (m *Model) SetRange(total, visible, offset int) {
	m.total = total
	m.visible = visible
	m.offset = offset
}

// SetTotal updates the total items count.
func (m *Model) SetTotal(total int) {
	m.total = total
}

// SetVisible updates the visible items count.
func (m *Model) SetVisible(visible int) {
	m.visible = visible
}

// SetOffset updates the top offset.
func (m *Model) SetOffset(offset int) {
	m.offset = offset
}

// Width returns the scrollbar width.
func (m Model) Width() int {
	return m.width
}

// Height returns the scrollbar height.
func (m Model) Height() int {
	return m.height
}

// View renders the scrollbar.
func (m Model) View() string {
	if m.height <= 0 || m.width <= 0 {
		return ""
	}

	blank := strings.Repeat(" ", m.width)
	if m.total <= m.visible || m.total <= 0 || m.visible <= 0 {
		return strings.TrimRight(strings.Repeat(blank+"\n", m.height), "\n")
	}

	ratio := float64(m.height) / float64(m.total)
	thumbHeight := maxInt(1, int(math.Round(float64(m.visible)*ratio)))
	maxOffset := maxInt(m.height-thumbHeight, 0)
	thumbOffset := clamp(int(math.Round(float64(m.offset)*ratio)), 0, maxOffset)

	track := m.styles.Track.Render(strings.Repeat(scrollbarTrack, m.width))
	thumb := m.styles.Thumb.Render(strings.Repeat(scrollbarThumb, m.width))

	var b strings.Builder
	for i := range m.height {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i >= thumbOffset && i < thumbOffset+thumbHeight {
			b.WriteString(thumb)
		} else {
			b.WriteString(track)
		}
	}
	return b.String()
}

func clamp(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
