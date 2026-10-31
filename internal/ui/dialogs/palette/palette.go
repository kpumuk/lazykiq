// Package palette provides a small searchable navigation dialog.
package palette

import (
	"context"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/scrollbar"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
)

// DialogID identifies the navigation palette.
const DialogID dialogs.DialogID = "navigation"

// Item is a navigation destination with an opaque identifier.
type (
	Item struct {
		Label string
		ID    int
		Queue string
	}
	// ActionMsg selects a destination.
	ActionMsg struct {
		ID    int
		Queue string
	}
	// Styles holds palette styles supplied by the app theme.
	Styles struct {
		Title, Border, Text, Muted, Selected lipgloss.Style
		Scrollbar                            scrollbar.Styles
	}
)

// DefaultStyles returns usable uncolored styles.
func DefaultStyles() Styles {
	return Styles{Selected: lipgloss.NewStyle().Reverse(true), Scrollbar: scrollbar.DefaultStyles()}
}

// Loader fetches additional destinations once for a palette activation.
type Loader func(context.Context) ([]Item, error)

// Option configures a palette.
type Option func(*Model)

// WithLoader adds a cancellable background destination fetch.
func WithLoader(parent context.Context, loader Loader) Option {
	return func(m *Model) {
		if parent != nil {
			m.parent = parent
		}
		m.loader = loader
	}
}

type loadedMsg struct {
	owner *Model
	items []Item
	err   error
}

// DialogID keeps discovery results routed to the palette while it is covered.
func (loadedMsg) DialogID() dialogs.DialogID { return DialogID }

// Model owns search input and completion selection.
type Model struct {
	parent                        context.Context
	loader                        Loader
	cancel                        context.CancelFunc
	loading, failed               bool
	styles                        Styles
	input                         textinput.Model
	items, matches                []Item
	selected, offset              int
	width, height, row, col, rows int
}

// New creates a navigation palette.
func New(items []Item, opts ...Option) *Model {
	m := &Model{parent: context.Background(), styles: DefaultStyles(), input: textinput.New(), items: slices.Clone(items)}
	slices.SortStableFunc(m.items, func(a, b Item) int {
		return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	})
	for _, opt := range opts {
		opt(m)
	}
	m.input.Prompt = ""
	m.input.Placeholder = "type to navigate"
	m.filter()
	return m
}

// SetStyles updates colors while preserving input and selection.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	styles := m.input.Styles()
	styles.Focused.Text = s.Text
	styles.Focused.Prompt = s.Title
	styles.Focused.Placeholder = s.Muted
	styles.Blurred = styles.Focused
	m.input.SetStyles(styles)
}

// Init focuses the search prompt.
func (m *Model) Init() tea.Cmd {
	focus := m.input.Focus()
	if m.loader == nil {
		return focus
	}
	ctx, cancel := context.WithTimeout(m.parent, 5*time.Second)
	m.cancel = cancel
	m.loading = true
	loader, owner := m.loader, m
	return tea.Batch(focus, func() tea.Msg {
		defer cancel()
		items, err := loader(ctx)
		return loadedMsg{owner: owner, items: items, err: err}
	})
}

// Close cancels queue discovery when the palette is dismissed.
func (m *Model) Close() tea.Cmd {
	m.loading = false
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// ID returns the dialog ID.
func (m *Model) ID() dialogs.DialogID { return DialogID }

// Position returns the centered overlay origin.
func (m *Model) Position() (int, int) { return m.row, m.col }

// Update handles search, selection and cancellation.
func (m *Model) Update(msg tea.Msg) (dialogs.DialogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.owner != m || !m.loading {
			return m, nil
		}
		m.loading, m.failed = false, msg.err != nil
		if msg.err == nil {
			m.addItems(msg.items)
		}
		return m, nil
	case tea.WindowSizeMsg:
		rows := 5
		// Eight matches, one input line and two borders, with five rows on each side.
		if msg.Height >= 8+3+2*5 {
			rows = 8
		}
		m.width, m.height = min(56, max(0, msg.Width)), min(rows+3, max(0, msg.Height))
		m.row, m.col = max(0, (msg.Height-m.height)/2), max(0, (msg.Width-m.width)/2)
		m.rows = max(0, m.height-3)
		m.input.SetWidth(max(1, m.width-3))
		position := m.input.Position()
		// Rebuild textinput's scroll window for the new width.
		m.input.CursorEnd()
		m.input.SetCursor(position)
		m.clamp()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return dialogs.CloseDialogMsg{} }
		case "enter":
			if len(m.matches) == 0 {
				return m, nil
			}
			item := m.matches[m.selected]
			return m, func() tea.Msg { return dialogs.CloseDialogMsg{Action: ActionMsg{ID: item.ID, Queue: item.Queue}} }
		case "up":
			m.selected--
			m.clamp()
			return m, nil
		case "down":
			m.selected++
			m.clamp()
			return m, nil
		}
	}
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if before != m.input.Value() {
		m.filter()
	}
	return m, cmd
}

// View renders up to eight completion rows, adapting to shorter terminals.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < 6 || m.height < 4 {
		return ansi.Truncate(m.input.View(), m.width, "")
	}
	contentWidth := m.width - 2
	lines := []string{ansi.Truncate(m.input.View(), contentWidth, "")}
	bar := scrollbar.New(scrollbar.WithStyles(m.styles.Scrollbar), scrollbar.WithSize(1, m.rows), scrollbar.WithRange(len(m.matches), m.rows, m.offset))
	tracks := strings.Split(bar.View(), "\n")
	for i := range m.rows {
		label := ""
		style := m.styles.Text
		idx := m.offset + i
		if idx < len(m.matches) {
			label = "  " + m.matches[idx].Label
			if idx == m.selected {
				label = "> " + m.matches[idx].Label
				style = m.styles.Selected
			}
		} else if i == 0 && len(m.matches) == 0 {
			label = "No matches"
			style = m.styles.Muted
		}
		label = ansi.Truncate(label, contentWidth-1, "…")
		label += strings.Repeat(" ", max(0, contentWidth-1-ansi.StringWidth(label)))
		lines = append(lines, style.Render(label)+tracks[i])
	}
	state := frame.StyleState{Title: m.styles.Title, Border: m.styles.Border}
	meta := ""
	if m.loading {
		meta = "loading queues"
	} else if m.failed {
		meta = "queues unavailable"
	}
	box := frame.New(frame.WithMeta(meta), frame.WithStyles(frame.Styles{Focused: state, Blurred: state}), frame.WithTitle("Go to"), frame.WithTitlePadding(0), frame.WithContent(strings.Join(lines, "\n")), frame.WithSize(m.width, m.height), frame.WithFocused(true))
	return box.View()
}

func (m *Model) filter() {
	m.matches = nil
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.Label), strings.ToLower(m.input.Value())) {
			m.matches = append(m.matches, item)
		}
	}
	m.selected, m.offset = 0, 0
	m.clamp()
}

func (m *Model) clamp() {
	m.selected = mathutil.Clamp(m.selected, 0, max(0, len(m.matches)-1))
	m.offset = mathutil.Clamp(m.offset, max(0, m.selected-max(1, m.rows)+1), m.selected)
	m.offset = min(m.offset, max(0, len(m.matches)-m.rows))
}

func (m *Model) addItems(items []Item) {
	var selected Item
	hadSelection := len(m.matches) > 0
	if hadSelection {
		selected = m.matches[m.selected]
	}
	offset := m.offset
	m.items = append(m.items, items...)
	slices.SortStableFunc(m.items, func(a, b Item) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	m.filter()
	if hadSelection {
		for i, item := range m.matches {
			if item.ID == selected.ID && item.Queue == selected.Queue {
				m.selected, m.offset = i, offset
				break
			}
		}
	}
	m.clamp()
}
