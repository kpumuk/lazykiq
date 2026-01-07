// Package confirm provides a confirmation dialog component.
package confirm

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

// DialogID identifies the confirmation dialog.
const DialogID dialogs.DialogID = "confirm"

// Selection describes which button is selected.
type Selection int

const (
	// SelectionNone indicates no button is selected.
	SelectionNone Selection = iota
	// SelectionYes indicates the "Yes" button is selected.
	SelectionYes
	// SelectionNo indicates the "No" button is selected.
	SelectionNo
)

// ActionMsg reports a confirmation result.
type ActionMsg struct {
	Confirmed bool
	Target    string
}

// Styles holds the styles used by the confirmation dialog.
type Styles struct {
	Title           lipgloss.Style
	Border          lipgloss.Style
	Text            lipgloss.Style
	Muted           lipgloss.Style
	Button          lipgloss.Style
	ButtonYesActive lipgloss.Style
	ButtonNoActive  lipgloss.Style
}

// DefaultStyles returns zero-value styles.
func DefaultStyles() Styles {
	return Styles{}
}

// Model defines state for the confirmation dialog component.
type Model struct {
	styles       Styles
	title        string
	message      string
	target       string
	yesLabel     string
	noLabel      string
	selection    Selection
	width        int
	height       int
	windowWidth  int
	windowHeight int
	row          int
	col          int
	padding      int
	minWidth     int
}

// Option configures the confirmation dialog.
type Option func(*Model)

// New creates a new confirmation dialog model.
func New(opts ...Option) *Model {
	m := &Model{
		styles:    DefaultStyles(),
		title:     "Confirm",
		yesLabel:  "Yes",
		noLabel:   "No",
		padding:   1,
		minWidth:  40,
		selection: SelectionNo,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) {
		m.styles = s
	}
}

// WithTitle sets the dialog title.
func WithTitle(title string) Option {
	return func(m *Model) {
		m.title = strings.TrimSpace(title)
	}
}

// WithMessage sets the dialog message.
func WithMessage(message string) Option {
	return func(m *Model) {
		m.message = strings.TrimSpace(message)
	}
}

// WithTarget sets the target value to return with the action.
func WithTarget(target string) Option {
	return func(m *Model) {
		m.target = target
	}
}

// WithLabels sets the yes/no labels.
func WithLabels(yesLabel, noLabel string) Option {
	return func(m *Model) {
		if strings.TrimSpace(yesLabel) != "" {
			m.yesLabel = strings.TrimSpace(yesLabel)
		}
		if strings.TrimSpace(noLabel) != "" {
			m.noLabel = strings.TrimSpace(noLabel)
		}
	}
}

// WithMinWidth sets the minimum dialog width.
func WithMinWidth(width int) Option {
	return func(m *Model) {
		m.minWidth = width
	}
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
		case "esc":
			return m, func() tea.Msg { return dialogs.CloseDialogMsg{} }
		case "y":
			return m, tea.Batch(
				func() tea.Msg { return ActionMsg{Confirmed: true, Target: m.target} },
				func() tea.Msg { return dialogs.CloseDialogMsg{} },
			)
		case "n":
			return m, tea.Batch(
				func() tea.Msg { return ActionMsg{Confirmed: false, Target: m.target} },
				func() tea.Msg { return dialogs.CloseDialogMsg{} },
			)
		case "left", "h", "shift+tab":
			m.moveSelection(-1)
			return m, nil
		case "right", "l", "tab":
			m.moveSelection(1)
			return m, nil
		case "enter":
			if m.selection == SelectionNone {
				m.selection = SelectionNo
			}
			confirmed := m.selection == SelectionYes
			return m, tea.Batch(
				func() tea.Msg { return ActionMsg{Confirmed: confirmed, Target: m.target} },
				func() tea.Msg { return dialogs.CloseDialogMsg{} },
			)
		}
	}

	return m, nil
}

// View renders the confirmation dialog.
func (m *Model) View() string {
	m.applySize()
	contentWidth := max(m.width-2-(m.padding*2), 1)
	message := m.renderMessage(contentWidth)
	buttons := m.renderButtons(contentWidth)

	contentLines := []string{}
	if message != "" {
		contentLines = append(contentLines, message, "")
	}

	contentLines = append(contentLines, buttons)

	content := strings.Join(contentLines, "\n")
	box := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Muted,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle(m.title),
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

func (m *Model) moveSelection(direction int) {
	if direction == 0 {
		return
	}
	switch m.selection {
	case SelectionNone:
		if direction < 0 {
			m.selection = SelectionYes
		} else {
			m.selection = SelectionNo
		}
	case SelectionYes:
		m.selection = SelectionNo
	case SelectionNo:
		m.selection = SelectionYes
	}
}

func (m *Model) renderMessage(width int) string {
	if m.message == "" {
		return ""
	}
	style := m.styles.Text.Width(width).MaxWidth(width).Align(lipgloss.Center)
	lines := strings.Split(m.message, "\n")
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			styled = append(styled, style.Render(line))
			continue
		}
		wrapped := lipgloss.Wrap(line, width, " ")
		for wrappedLine := range strings.SplitSeq(wrapped, "\n") {
			styled = append(styled, style.Render(wrappedLine))
		}
	}
	return strings.Join(styled, "\n")
}

func (m *Model) renderButtons(width int) string {
	yes := m.renderButton(m.yesLabel, SelectionYes, m.selection == SelectionYes)
	no := m.renderButton(m.noLabel, SelectionNo, m.selection == SelectionNo)
	buttons := yes + "  " + no
	return centerLine(buttons, width)
}

func (m *Model) renderButton(label string, kind Selection, selected bool) string {
	style := m.styles.Button
	if selected {
		switch kind {
		case SelectionNone:
			style = m.styles.ButtonNoActive
		case SelectionYes:
			style = m.styles.ButtonYesActive
		case SelectionNo:
			style = m.styles.ButtonNoActive
		}
		return style.Render(label)
	}
	runes := []rune(label)
	if len(runes) == 0 {
		return style.Render(label)
	}
	hotkeyStyle := style.Bold(true).Underline(true).Padding(0, 0)
	hotkey := hotkeyStyle.Render(string(runes[0]))
	content := hotkey + string(runes[1:])
	return style.Render(content)
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

	contentWidth := max(dialogWidth-2-(m.padding*2), 1)
	message := m.renderMessage(contentWidth)
	contentLines := 1
	if message != "" {
		contentLines += lipgloss.Height(message) + 1
	}

	dialogHeight := contentLines + 2
	dialogHeight = max(dialogHeight, 3)
	dialogHeight = min(dialogHeight, max(m.windowHeight-2, 3))

	m.width = dialogWidth
	m.height = dialogHeight
	m.row = max((m.windowHeight-dialogHeight)/2, 0)
	m.col = max((m.windowWidth-dialogWidth)/2, 0)
}

func centerLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	lineWidth := lipgloss.Width(line)
	if lineWidth >= width {
		return line
	}
	left := (width - lineWidth) / 2
	return strings.Repeat(" ", left) + line
}
