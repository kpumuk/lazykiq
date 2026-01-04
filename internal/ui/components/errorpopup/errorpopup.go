// Package errorpopup renders connection error overlays.
package errorpopup

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
)

// Styles holds the styles needed by the error popup.
type Styles struct {
	Title   lipgloss.Style
	Message lipgloss.Style
	Border  lipgloss.Style
}

// DefaultStyles returns default styles for the error popup.
func DefaultStyles() Styles {
	errorColor := theme.DefaultTheme.Error
	return Styles{
		Title:   lipgloss.NewStyle().Foreground(errorColor).Bold(true),
		Message: lipgloss.NewStyle().Faint(true),
		Border:  lipgloss.NewStyle().Foreground(errorColor),
	}
}

// Model defines state for the error popup component.
type Model struct {
	styles  Styles
	message string
	width   int
	height  int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new error popup model.
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

// WithSize sets the width and height.
func WithSize(w, h int) Option {
	return func(m *Model) {
		m.width = w
		m.height = h
	}
}

// WithMessage sets the error message.
func WithMessage(msg string) Option {
	return func(m *Model) {
		m.message = msg
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize sets the width and height.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetMessage sets the error message to display.
func (m *Model) SetMessage(msg string) {
	m.message = msg
}

// Width returns the current width.
func (m Model) Width() int {
	return m.width
}

// Height returns the current height.
func (m Model) Height() int {
	return m.height
}

// Message returns the current error message.
func (m Model) Message() string {
	return m.message
}

// HasError returns true if there is an error message to display.
func (m Model) HasError() bool {
	return m.message != ""
}

// View renders the error popup overlaid on the background content.
func (m Model) View() string {
	if m.message == "" {
		return ""
	}

	panelWidth := min(m.width, 60)
	if panelWidth < 2 {
		return ""
	}
	contentWidth := max(panelWidth-2-2, 0) // borders + padding

	// Error message content
	messageStyle := m.styles.Message.Width(contentWidth)
	errorMessage := messageStyle.Render(m.message) + "\n\n" +
		messageStyle.Render("Retrying every 5 seconds...")

	// Create error panel with title on border
	messageLines := strings.Split(errorMessage, "\n")
	panelHeight := min(len(messageLines)+2, m.height)
	errorPanel := frame.New(
		frame.WithStyles(frame.Styles{
			Focused: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Message,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
			Blurred: frame.StyleState{
				Title:  m.styles.Title,
				Muted:  m.styles.Message,
				Filter: m.styles.Title,
				Border: m.styles.Border,
			},
		}),
		frame.WithTitle("Connection Error"),
		frame.WithTitlePadding(0),
		frame.WithContent(errorMessage),
		frame.WithSize(panelWidth, panelHeight),
		frame.WithPadding(1),
		frame.WithFocused(true),
	).View()

	return errorPanel
}
