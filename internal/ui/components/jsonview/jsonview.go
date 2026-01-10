// Package jsonview renders syntax-highlighted JSON content.
package jsonview

import (
	"encoding/json"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/mathutil"
)

// Styles holds styles for JSON tokens.
type Styles struct {
	Text        lipgloss.Style
	Key         lipgloss.Style
	String      lipgloss.Style
	Number      lipgloss.Style
	Bool        lipgloss.Style
	Null        lipgloss.Style
	Punctuation lipgloss.Style
	Muted       lipgloss.Style
}

// DefaultStyles returns default styles.
func DefaultStyles() Styles {
	return Styles{
		Text:        lipgloss.NewStyle(),
		Key:         lipgloss.NewStyle(),
		String:      lipgloss.NewStyle(),
		Number:      lipgloss.NewStyle(),
		Bool:        lipgloss.NewStyle(),
		Null:        lipgloss.NewStyle(),
		Punctuation: lipgloss.NewStyle(),
		Muted:       lipgloss.NewStyle(),
	}
}

// Model is the JSON view component state.
type Model struct {
	styles Styles
	width  int
	height int

	lines    []string
	tokens   [][]chroma.Token
	maxWidth int
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new JSON view model.
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

// WithSize sets the dimensions.
func WithSize(width, height int) Option {
	return func(m *Model) {
		m.width = width
		m.height = height
	}
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
}

// SetSize sets the dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Width returns the width.
func (m Model) Width() int {
	return m.width
}

// Height returns the height.
func (m Model) Height() int {
	return m.height
}

// LineCount returns the number of lines.
func (m Model) LineCount() int {
	return len(m.lines)
}

// MaxWidth returns the maximum line width.
func (m Model) MaxWidth() int {
	return m.maxWidth
}

// SetValue formats and tokenizes a JSON-serializable value.
func (m *Model) SetValue(value any) {
	m.lines = nil
	m.tokens = nil
	m.maxWidth = 0

	if value == nil {
		return
	}

	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		m.lines = []string{"{}", "  Error formatting JSON"}
		return
	}

	jsonText := string(b)
	m.lines = strings.Split(jsonText, "\n")
	m.tokens = tokenizeJSONLines(jsonText)
	if len(m.tokens) != len(m.lines) {
		m.tokens = nil
	}

	for _, line := range m.lines {
		if len(line) > m.maxWidth {
			m.maxWidth = len(line)
		}
	}
}

// RenderLine renders a single line with horizontal scroll and syntax highlighting.
func (m Model) RenderLine(index, offset, width int) string {
	if width <= 0 {
		return ""
	}
	if index < 0 || index >= len(m.lines) {
		return ""
	}
	if len(m.tokens) == len(m.lines) {
		return m.renderTokens(m.tokens[index], offset, width)
	}

	line := applyHorizontalScroll(m.lines[index], offset, width)
	return m.styles.Text.Render(line)
}

func (m Model) renderTokens(tokens []chroma.Token, offset, width int) string {
	if width <= 0 {
		return ""
	}
	offset = max(offset, 0)

	end := offset + width
	var builder strings.Builder
	col := 0

	for _, token := range tokens {
		if token.Type == chroma.EOFType {
			break
		}

		tokenWidth := lipgloss.Width(token.Value)
		if tokenWidth == 0 {
			continue
		}

		tokenStart := col
		tokenEnd := col + tokenWidth

		if tokenEnd > offset && tokenStart < end {
			start := mathutil.Clamp(offset-tokenStart, 0, tokenWidth)
			stop := mathutil.Clamp(end-tokenStart, 0, tokenWidth)
			segment := ansi.Cut(token.Value, start, stop)
			if segment != "" {
				builder.WriteString(m.styleForToken(token).Render(segment))
			}
		}

		col = tokenEnd
		if col >= end {
			break
		}
	}

	rendered := builder.String()
	if renderedWidth := lipgloss.Width(rendered); renderedWidth < width {
		rendered += strings.Repeat(" ", width-renderedWidth)
	}
	return rendered
}

func (m Model) styleForToken(token chroma.Token) lipgloss.Style {
	switch {
	case token.Type == chroma.NameTag:
		return m.styles.Key
	case token.Type.InSubCategory(chroma.LiteralString):
		return m.styles.String
	case token.Type.InSubCategory(chroma.LiteralNumber):
		return m.styles.Number
	case token.Type.InCategory(chroma.Keyword):
		if token.Value == "null" {
			return m.styles.Null
		}
		return m.styles.Bool
	case token.Type.InCategory(chroma.Comment):
		return m.styles.Muted
	case token.Type == chroma.Punctuation:
		return m.styles.Punctuation
	default:
		return m.styles.Text
	}
}

func applyHorizontalScroll(line string, offset, visibleWidth int) string {
	if visibleWidth <= 0 {
		return ""
	}
	offset = max(offset, 0)

	cut := ansi.Cut(line, offset, offset+visibleWidth)
	cutWidth := lipgloss.Width(cut)
	if cutWidth < visibleWidth {
		cut += strings.Repeat(" ", visibleWidth-cutWidth)
	}
	return cut
}

func tokenizeJSONLines(jsonText string) [][]chroma.Token {
	if jsonLexer == nil {
		return nil
	}

	iterator, err := jsonLexer.Tokenise(nil, jsonText)
	if err != nil {
		return nil
	}

	lines := [][]chroma.Token{{}}
	for _, token := range iterator.Tokens() {
		if token.Type == chroma.EOFType {
			break
		}
		if token.Value == "" {
			continue
		}

		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				lines = append(lines, []chroma.Token{})
			}
			if part == "" {
				continue
			}
			lines[len(lines)-1] = append(lines[len(lines)-1], chroma.Token{Type: token.Type, Value: part})
		}
	}

	return lines
}

var jsonLexer = func() chroma.Lexer {
	lexer := lexers.Get("json")
	if lexer == nil {
		return nil
	}
	return chroma.Coalesce(lexer)
}()
