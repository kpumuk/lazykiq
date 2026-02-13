// Package stats renders the top metrics bar.
package stats

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/ui/display"
)

// Data holds the Sidekiq metrics values.
type Data struct {
	Processed int64
	Failed    int64
	Busy      int64
	Enqueued  int64
	Retries   int64
	Scheduled int64
	Dead      int64
	UpdatedAt time.Time
}

// UpdateMsg is sent when metrics should be updated.
type UpdateMsg struct {
	Data Data
}

// Styles holds the styles needed by the metrics bar.
type Styles struct {
	Bar   lipgloss.Style
	Fill  lipgloss.Style
	Label lipgloss.Style
	Value lipgloss.Style
}

// DefaultStyles returns default styles for the metrics bar.
func DefaultStyles() Styles {
	return Styles{
		Bar:   lipgloss.NewStyle().Padding(0, 1),
		Fill:  lipgloss.NewStyle(),
		Label: lipgloss.NewStyle().Faint(true),
		Value: lipgloss.NewStyle().Bold(true),
	}
}

// Model defines state for the metrics bar component.
type Model struct {
	styles Styles
	data   Data
	width  int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new metrics bar model.
func New(opts ...Option) Model {
	m := Model{
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

// WithWidth sets the width.
func WithWidth(w int) Option {
	return func(m *Model) {
		m.width = w
	}
}

// WithData sets the initial data.
func WithData(d Data) Option {
	return func(m *Model) {
		m.data = d
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetWidth sets the width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// SetData sets the metrics data.
func (m *Model) SetData(d Data) {
	m.data = d
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the height of the metrics bar (always 1).
func (m Model) Height() int {
	return 1
}

// Data returns the current metrics data.
func (m Model) Data() Data {
	return m.data
}

// Init returns an initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case UpdateMsg:
		m.data = msg.Data
	}
	return m, nil
}

// View renders the metrics bar.
func (m Model) View() string {
	barStyle := m.styles.Bar.Width(m.width)

	baseMetrics := []string{
		m.styles.Label.Render("Processed: ") + m.styles.Value.Render(display.ShortNumber(m.data.Processed)),
		m.styles.Label.Render("Failed: ") + m.styles.Value.Render(display.ShortNumber(m.data.Failed)),
		m.styles.Label.Render("Busy: ") + m.styles.Value.Render(display.ShortNumber(m.data.Busy)),
		m.styles.Label.Render("Enqueued: ") + m.styles.Value.Render(display.ShortNumber(m.data.Enqueued)),
		m.styles.Label.Render("Retries: ") + m.styles.Value.Render(display.ShortNumber(m.data.Retries)),
		m.styles.Label.Render("Scheduled: ") + m.styles.Value.Render(display.ShortNumber(m.data.Scheduled)),
		m.styles.Label.Render("Dead: ") + m.styles.Value.Render(display.ShortNumber(m.data.Dead)),
	}

	if m.width <= 0 || len(baseMetrics) == 0 {
		return barStyle.Render("")
	}

	contentWidth := max(m.width-barStyle.GetHorizontalPadding(), 0)

	baseItems, baseWidths, maxWidth := buildBaseItems(baseMetrics, m.styles)
	sep := " "
	sepWidth := lipgloss.Width(sep)

	targetWidths := layoutTargetWidths(baseWidths, maxWidth, contentWidth, sepWidth)
	if targetWidths == nil {
		content := strings.Join(baseItems, sep)
		content = ansi.Truncate(content, contentWidth, "")
		return barStyle.Render(content)
	}

	metricsItems := applyWidths(baseItems, targetWidths, m.styles.Fill)
	return barStyle.Render(strings.Join(metricsItems, sep))
}

func buildBaseItems(metrics []string, styles Styles) ([]string, []int, int) {
	pad := styles.Fill.Render(" ")
	items := make([]string, len(metrics))
	widths := make([]int, len(metrics))
	maxWidth := 0

	for i, metric := range metrics {
		item := pad + metric + pad
		width := lipgloss.Width(item)
		items[i] = item
		widths[i] = width
		if width > maxWidth {
			maxWidth = width
		}
	}

	return items, widths, maxWidth
}

func layoutTargetWidths(baseWidths []int, maxWidth, contentWidth, sepWidth int) []int {
	if len(baseWidths) == 0 {
		return []int{}
	}

	minTotal := 0
	for _, width := range baseWidths {
		minTotal += width
	}
	minTotal += sepWidth * (len(baseWidths) - 1)

	if contentWidth < minTotal {
		return nil
	}

	targetWidths := make([]int, len(baseWidths))
	for i := range targetWidths {
		targetWidths[i] = maxWidth
	}

	totalEqual := maxWidth*len(baseWidths) + sepWidth*(len(baseWidths)-1)
	if contentWidth >= totalEqual {
		extra := contentWidth - totalEqual
		for extra > 0 {
			for i := range targetWidths {
				targetWidths[i]++
				extra--
				if extra == 0 {
					break
				}
			}
		}
		return targetWidths
	}

	overflow := totalEqual - contentWidth
	for overflow > 0 {
		trimmed := false
		for i := range targetWidths {
			if targetWidths[i] > baseWidths[i] {
				targetWidths[i]--
				overflow--
				trimmed = true
				if overflow == 0 {
					break
				}
			}
		}
		if !trimmed {
			return nil
		}
	}

	return targetWidths
}

func applyWidths(items []string, targetWidths []int, fillStyle lipgloss.Style) []string {
	applied := make([]string, len(items))
	for i, item := range items {
		width := lipgloss.Width(item)
		if targetWidths[i] > width {
			item += fillStyle.Render(strings.Repeat(" ", targetWidths[i]-width))
		}
		applied[i] = item
	}
	return applied
}
