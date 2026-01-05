// Package navbar renders the bottom navigation bar.
package navbar

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ViewInfo holds information about a view for display in the navbar.
type ViewInfo struct {
	Name string
}

// Styles holds the styles needed by the navbar.
type Styles struct {
	Bar   lipgloss.Style
	Key   lipgloss.Style
	Item  lipgloss.Style
	Quit  lipgloss.Style
	Brand lipgloss.Style
}

// DefaultStyles returns default styles for the navbar.
func DefaultStyles() Styles {
	return Styles{
		Bar:   lipgloss.NewStyle().Padding(0, 1),
		Key:   lipgloss.NewStyle().Padding(0, 1),
		Item:  lipgloss.NewStyle().PaddingRight(1),
		Quit:  lipgloss.NewStyle().PaddingRight(1),
		Brand: lipgloss.NewStyle(),
	}
}

// Model defines state for the navbar component.
type Model struct {
	styles Styles
	views  []ViewInfo
	brand  string
	width  int
	help   key.Binding
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new navbar model.
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

// WithViews sets the views to display.
func WithViews(views []ViewInfo) Option {
	return func(m *Model) {
		m.views = views
	}
}

// WithBrand sets the brand label shown on the right.
func WithBrand(brand string) Option {
	return func(m *Model) {
		m.brand = brand
	}
}

// WithHelp sets the help binding.
func WithHelp(help key.Binding) Option {
	return func(m *Model) {
		m.help = help
	}
}

// WithWidth sets the width.
func WithWidth(w int) Option {
	return func(m *Model) {
		m.width = w
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetViews sets the views to display.
func (m *Model) SetViews(views []ViewInfo) {
	m.views = views
}

// SetBrand sets the brand label shown on the right.
func (m *Model) SetBrand(brand string) {
	m.brand = brand
}

// SetHelp sets the help binding.
func (m *Model) SetHelp(help key.Binding) {
	m.help = help
}

// SetWidth sets the width.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the height of the navbar (always 1).
func (m Model) Height() int {
	return 1
}

// Init returns an initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(_ tea.Msg) (Model, tea.Cmd) {
	return m, nil
}

// View renders the navbar.
func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}

	barStyle := m.styles.Bar.Width(m.width)
	_, rightPad, _, leftPad := m.styles.Bar.GetPadding()
	innerWidth := m.width - leftPad - rightPad
	if innerWidth <= 0 {
		return barStyle.Render("")
	}

	var items strings.Builder
	for i, v := range m.views {
		key := m.styles.Key.Render(strconv.Itoa(i + 1))
		name := m.styles.Item.Render(v.Name)
		items.WriteString(key + name)
	}

	// Add quit hint
	items.WriteString(m.styles.Key.Render("q") + m.styles.Quit.Render("quit"))
	if m.help.Enabled() {
		items.WriteString(m.styles.Key.Render(m.help.Help().Key) + m.styles.Quit.Render(m.help.Help().Desc))
	}

	left := items.String()
	right := ""
	if m.brand != "" {
		right = m.styles.Brand.Render(m.brand)
	}

	leftWidth := lipgloss.Width(left)
	if right == "" || leftWidth >= innerWidth-1 {
		line := lipgloss.NewStyle().MaxWidth(innerWidth).Render(left)
		return barStyle.Render(line)
	}

	spaceForRight := innerWidth - leftWidth - 1
	right = lipgloss.NewStyle().MaxWidth(spaceForRight).Render(right)
	right = lipgloss.PlaceHorizontal(spaceForRight, lipgloss.Right, right)
	line := left + " " + right
	return barStyle.Render(line)
}
