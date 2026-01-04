// Package filter provides a filter dialog component.
package filter

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

// DialogID identifies the filter dialog.
const DialogID dialogs.DialogID = "filter"

// Action describes filter dialog intents.
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

// Styles holds the styles used by the filter dialog.
type Styles struct {
	Title       lipgloss.Style
	Border      lipgloss.Style
	Prompt      lipgloss.Style
	Text        lipgloss.Style
	Placeholder lipgloss.Style
	Cursor      lipgloss.Style
}

// DefaultStyles returns zero-value styles.
func DefaultStyles() Styles {
	return Styles{}
}

// Model defines state for the filter dialog component.
type Model struct {
	styles       Styles
	input        textinput.Model
	query        string
	width        int
	height       int
	windowWidth  int
	windowHeight int
	row          int
	col          int
	padding      int
	minWidth     int
}

// Option configures the filter dialog.
type Option func(*Model)

// New creates a new filter dialog model.
func New(opts ...Option) *Model {
	m := &Model{
		styles:   DefaultStyles(),
		input:    textinput.New(),
		padding:  1,
		minWidth: 38,
	}

	m.input.Prompt = ""
	m.input.Blur()

	for _, opt := range opts {
		opt(m)
	}

	m.applyStyles()
	m.applySize()
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

// WithQuery sets the initial query.
func WithQuery(query string) Option {
	return func(m *Model) {
		m.query = strings.TrimSpace(query)
		m.input.SetValue(m.query)
	}
}

// WithMinWidth sets the minimum dialog width.
func WithMinWidth(width int) Option {
	return func(m *Model) {
		m.minWidth = width
	}
}

// Init focuses the input.
func (m *Model) Init() tea.Cmd {
	m.input.SetValue(m.query)
	m.input.CursorEnd()
	m.syncPlaceholder()
	return m.input.Focus()
}

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
		case "enter":
			next := strings.TrimSpace(m.input.Value())
			if next == m.query {
				return m, func() tea.Msg { return dialogs.CloseDialogMsg{} }
			}
			action := ActionApply
			if next == "" {
				action = ActionClear
			}
			m.query = next
			return m, tea.Batch(
				func() tea.Msg { return ActionMsg{Action: action, Query: m.query} },
				func() tea.Msg { return dialogs.CloseDialogMsg{} },
			)
		case "esc":
			return m, func() tea.Msg { return dialogs.CloseDialogMsg{} }
		case "ctrl+u":
			m.input.SetValue("")
			m.input.CursorEnd()
			return m, nil
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the filter dialog.
func (m *Model) View() string {
	contentWidth := max(m.width-2-(m.padding*2), 1)
	content := lipgloss.NewStyle().Width(contentWidth).MaxWidth(contentWidth).Render(m.input.View())
	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Placeholder,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Placeholder,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle("Filter"),
		frame.WithTitlePadding(0),
		frame.WithContent(content),
		frame.WithPadding(m.padding),
		frame.WithSize(m.width, m.height),
		frame.WithMinHeight(3),
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

func (m *Model) applySize() {
	if m.windowWidth == 0 || m.windowHeight == 0 {
		return
	}

	dialogWidth := max(m.windowWidth/2, m.minWidth)
	dialogWidth = min(dialogWidth, m.windowWidth-4)
	if dialogWidth < 10 {
		dialogWidth = max(m.windowWidth-2, 10)
	}

	dialogHeight := 3
	if m.windowHeight < dialogHeight {
		dialogHeight = max(m.windowHeight, 3)
	}

	m.width = dialogWidth
	m.height = dialogHeight
	m.row = max((m.windowHeight-dialogHeight)/2, 0)
	m.col = max((m.windowWidth-dialogWidth)/2, 0)

	contentWidth := max(dialogWidth-2-(m.padding*2), 1)
	promptWidth := lipgloss.Width(m.input.Prompt)
	// textinput renders a virtual cursor that adds one extra column.
	m.input.SetWidth(max(contentWidth-promptWidth-1, 1))
}

func (m *Model) syncPlaceholder() {
	switch {
	case m.input.Focused():
		m.input.Placeholder = "type to filter"
	case m.query == "":
		m.input.Placeholder = "type to filter"
	default:
		m.input.Placeholder = ""
	}
}
