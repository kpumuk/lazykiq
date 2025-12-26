package metrics

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kpumuk/lazykiq/internal/ui/format"
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
}

// UpdateMsg is sent when metrics should be updated.
type UpdateMsg struct {
	Data Data
}

// Styles holds the styles needed by the metrics bar.
type Styles struct {
	Bar       lipgloss.Style
	Label     lipgloss.Style
	Value     lipgloss.Style
	Separator lipgloss.Style
}

// DefaultStyles returns default styles for the metrics bar.
func DefaultStyles() Styles {
	return Styles{
		Bar:       lipgloss.NewStyle().Padding(0, 1),
		Label:    lipgloss.NewStyle().Faint(true),
		Value:     lipgloss.NewStyle().Bold(true),
		Separator: lipgloss.NewStyle().Faint(true),
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

	sep := m.styles.Separator.Render(" │ ")

	metricsItems := []string{
		m.styles.Label.Render("Processed: ") + m.styles.Value.Render(format.Number(m.data.Processed)),
		m.styles.Label.Render("Failed: ") + m.styles.Value.Render(format.Number(m.data.Failed)),
		m.styles.Label.Render("Busy: ") + m.styles.Value.Render(format.Number(m.data.Busy)),
		m.styles.Label.Render("Enqueued: ") + m.styles.Value.Render(format.Number(m.data.Enqueued)),
		m.styles.Label.Render("Retries: ") + m.styles.Value.Render(format.Number(m.data.Retries)),
		m.styles.Label.Render("Scheduled: ") + m.styles.Value.Render(format.Number(m.data.Scheduled)),
		m.styles.Label.Render("Dead: ") + m.styles.Value.Render(format.Number(m.data.Dead)),
	}

	content := ""
	for i, metric := range metricsItems {
		if i > 0 {
			content += sep
		}
		content += metric
	}

	return barStyle.Render(content)
}
