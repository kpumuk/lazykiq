package frame

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Title renders a frame title with an optional filter.
type Title struct {
	focused StyleState
	blurred StyleState
	text    string
	filter  string
}

// TitleOption configures a Title.
type TitleOption func(*Title)

// NewTitle creates a new title component.
func NewTitle(opts ...TitleOption) Title {
	t := Title{
		focused: StyleState{
			Title: lipgloss.NewStyle().Bold(true),
		},
		blurred: StyleState{
			Title: lipgloss.NewStyle().Bold(true),
		},
	}
	for _, opt := range opts {
		opt(&t)
	}
	return t
}

// WithTitleText sets the title text.
func WithTitleText(text string) TitleOption {
	return func(t *Title) { t.text = text }
}

// WithTitleFilter sets the filter text.
func WithTitleFilter(filter string) TitleOption {
	return func(t *Title) { t.filter = filter }
}

// WithTitleStyles sets the title styles for focused and blurred states.
func WithTitleStyles(focused, blurred StyleState) TitleOption {
	return func(t *Title) {
		t.focused = focused
		t.blurred = blurred
	}
}

// SetTitle updates the title text.
func (t *Title) SetTitle(text string) {
	t.text = text
}

// SetFilter updates the filter text.
func (t *Title) SetFilter(filter string) {
	t.filter = filter
}

// SetStyles updates the title styles.
func (t *Title) SetStyles(focused, blurred StyleState) {
	t.focused = focused
	t.blurred = blurred
}

// Render renders the title within the max width.
func (t Title) Render(focused bool, maxWidth, padding int) string {
	if maxWidth <= 0 || t.text == "" {
		return ""
	}
	contentWidth := maxWidth - (padding * 2)
	if contentWidth <= 0 {
		return ""
	}
	style := t.blurred
	if focused {
		style = t.focused
	}

	baseWidth := lipgloss.Width(t.text)
	if baseWidth > contentWidth {
		base := style.Title.Render(truncateWithEllipsis(t.text, contentWidth))
		return t.pad(style, base, padding)
	}

	if strings.TrimSpace(t.filter) == "" {
		base := style.Title.Render(t.text)
		return t.pad(style, base, padding)
	}

	bracketWidth := lipgloss.Width("[") + lipgloss.Width("]")
	available := contentWidth - baseWidth - bracketWidth
	if available <= 0 {
		base := style.Title.Render(truncateWithEllipsis(t.text, contentWidth))
		return t.pad(style, base, padding)
	}

	filter := truncateWithEllipsis(strings.TrimSpace(t.filter), available)
	rendered := style.Title.Render(t.text) +
		style.Muted.Render("[") +
		style.Filter.Render(filter) +
		style.Muted.Render("]")

	return t.pad(style, rendered, padding)
}

func (t Title) pad(style StyleState, rendered string, padding int) string {
	if padding <= 0 {
		return rendered
	}
	pad := style.Title.Render(strings.Repeat(" ", padding))
	return pad + rendered + pad
}

func truncateWithEllipsis(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= maxWidth {
		return text
	}
	ellipsis := "…"
	ellipsisWidth := lipgloss.Width(ellipsis)
	if maxWidth <= ellipsisWidth {
		return ellipsis
	}
	target := maxWidth - ellipsisWidth
	var b strings.Builder
	width := 0
	for _, r := range text {
		rw := lipgloss.Width(string(r))
		if width+rw > target {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String() + ellipsis
}
