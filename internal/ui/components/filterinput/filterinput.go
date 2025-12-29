// Package filterinput provides a reusable filter input component.
package filterinput

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Action describes filter input intents.
type Action int

const (
	// ActionNone indicates no action.
	ActionNone Action = iota
	// ActionApply indicates the current query should be applied.
	ActionApply
	// ActionClear indicates the current query should be cleared.
	ActionClear
)

// ActionMsg reports a filter action and query.
type ActionMsg struct {
	Action Action
	Query  string
}

// Styles holds the styles used by the filter input.
type Styles struct {
	Prompt      lipgloss.Style
	Text        lipgloss.Style
	Placeholder lipgloss.Style
	Cursor      lipgloss.Style
}

// DefaultStyles returns zero-value styles.
func DefaultStyles() Styles {
	return Styles{}
}

// Model defines state for the filter input component.
type Model struct {
	styles            Styles
	input             textinput.Model
	width             int
	query             string
	prompt            string
	placeholderIdle   string
	placeholderActive string
}

// Option configures the filter input.
type Option func(*Model)

// New creates a new filter input model.
func New(opts ...Option) Model {
	m := Model{
		styles:            DefaultStyles(),
		input:             textinput.New(),
		prompt:            "FILTER: ",
		placeholderIdle:   "press / to filter",
		placeholderActive: "type to filter",
	}

	m.input.Prompt = m.prompt
	m.input.Blur()

	for _, opt := range opts {
		opt(&m)
	}

	m.applyStyles()
	m.applyWidth()
	m.syncPlaceholder()

	return m
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) {
		m.styles = s
		m.applyStyles()
	}
}

// WithWidth sets the available width.
func WithWidth(width int) Option {
	return func(m *Model) {
		m.width = width
		m.applyWidth()
	}
}

// WithQuery sets the initial query.
func WithQuery(query string) Option {
	return func(m *Model) {
		m.query = query
		m.input.SetValue(query)
	}
}

// WithPrompt sets the prompt text.
func WithPrompt(prompt string) Option {
	return func(m *Model) {
		m.prompt = prompt
		m.input.Prompt = prompt
		m.applyWidth()
	}
}

// WithPlaceholders sets the idle and active placeholders.
func WithPlaceholders(idle, active string) Option {
	return func(m *Model) {
		m.placeholderIdle = idle
		m.placeholderActive = active
		m.syncPlaceholder()
	}
}

// SetStyles updates styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.applyStyles()
}

// SetWidth updates available width.
func (m *Model) SetWidth(width int) {
	m.width = width
	m.applyWidth()
}

// Query returns the current query.
func (m Model) Query() string {
	return m.query
}

// Focused reports whether the input is focused.
func (m Model) Focused() bool {
	return m.input.Focused()
}

// Init resets focus and placeholder.
func (m *Model) Init() {
	m.input.SetValue(m.query)
	m.input.Blur()
	m.syncPlaceholder()
}

// Update handles key messages and returns optional action messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.input.Focused() {
			switch msg.String() {
			case "enter":
				next := strings.TrimSpace(m.input.Value())
				changed := next != m.query
				m.query = next
				m.blur()
				if changed {
					return m, func() tea.Msg {
						return ActionMsg{Action: ActionApply, Query: m.query}
					}
				}
				return m, nil
			case "esc":
				if m.query != "" {
					m.query = ""
					m.input.SetValue("")
					m.blur()
					return m, func() tea.Msg {
						return ActionMsg{Action: ActionClear, Query: ""}
					}
				}
				m.blur()
				return m, nil
			}

			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "/":
			cmd := m.focus()
			return m, cmd
		case "esc", "ctrl+u":
			if m.query != "" {
				m.query = ""
				m.input.SetValue("")
				return m, func() tea.Msg {
					return ActionMsg{Action: ActionClear, Query: ""}
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// View renders the filter input padded to width.
func (m Model) View() string {
	line := m.input.View()
	if m.width < 1 {
		return line
	}
	var style lipgloss.Style
	return style.Width(m.width).MaxWidth(m.width).Render(line)
}

func (m *Model) applyStyles() {
	styles := m.input.Styles()
	styles.Focused.Prompt = m.styles.Prompt
	styles.Focused.Text = m.styles.Text
	styles.Focused.Placeholder = m.styles.Placeholder
	styles.Blurred.Prompt = m.styles.Prompt
	styles.Blurred.Text = m.styles.Text
	styles.Blurred.Placeholder = m.styles.Placeholder
	if cursorColor := m.styles.Cursor.GetForeground(); cursorColor != nil {
		styles.Cursor.Color = cursorColor
	}
	m.input.SetStyles(styles)
}

func (m *Model) applyWidth() {
	if m.width < 1 {
		return
	}
	promptWidth := lipgloss.Width(m.input.Prompt)
	width := m.width - promptWidth
	if width < 1 {
		width = 1
	}
	m.input.SetWidth(width)
}

func (m *Model) syncPlaceholder() {
	if m.input.Focused() {
		m.input.Placeholder = m.placeholderActive
	} else {
		m.input.Placeholder = m.placeholderIdle
	}
}

func (m *Model) focus() tea.Cmd {
	m.input.SetValue(m.query)
	m.input.CursorEnd()
	m.syncPlaceholder()
	return m.input.Focus()
}

func (m *Model) blur() {
	m.input.SetValue(m.query)
	m.input.Blur()
	m.syncPlaceholder()
}
