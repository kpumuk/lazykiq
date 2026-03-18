// Package jsonview renders syntax-highlighted JSON content.
package jsonview

import (
	"encoding/json"
	"strings"

	"charm.land/lipgloss/v2"
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
	tokens   [][]token
	maxWidth int
}

type tokenKind uint8

const (
	tokenText tokenKind = iota
	tokenKey
	tokenString
	tokenNumber
	tokenBool
	tokenNull
	tokenPunctuation
)

type token struct {
	kind  tokenKind
	value string
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

func (m Model) renderTokens(tokens []token, offset, width int) string {
	if width <= 0 {
		return ""
	}
	offset = max(offset, 0)

	end := offset + width
	var builder strings.Builder
	col := 0

	for _, token := range tokens {
		tokenWidth := lipgloss.Width(token.value)
		if tokenWidth == 0 {
			continue
		}

		tokenStart := col
		tokenEnd := col + tokenWidth

		if tokenEnd > offset && tokenStart < end {
			start := mathutil.Clamp(offset-tokenStart, 0, tokenWidth)
			stop := mathutil.Clamp(end-tokenStart, 0, tokenWidth)
			segment := ansi.Cut(token.value, start, stop)
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

func (m Model) styleForToken(token token) lipgloss.Style {
	switch token.kind {
	case tokenText:
		return m.styles.Text
	case tokenKey:
		return m.styles.Key
	case tokenString:
		return m.styles.String
	case tokenNumber:
		return m.styles.Number
	case tokenBool:
		return m.styles.Bool
	case tokenNull:
		return m.styles.Null
	case tokenPunctuation:
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

func tokenizeJSONLines(jsonText string) [][]token {
	if jsonText == "" {
		return nil
	}

	lines := [][]token{{}}
	appendToken := func(kind tokenKind, value string) {
		if value == "" {
			return
		}
		lines[len(lines)-1] = append(lines[len(lines)-1], token{kind: kind, value: value})
	}

	for i := 0; i < len(jsonText); {
		switch jsonText[i] {
		case '\n':
			lines = append(lines, []token{})
			i++
		case ' ', '\t', '\r':
			start := i
			for i < len(jsonText) && jsonText[i] != '\n' && isInlineWhitespace(jsonText[i]) {
				i++
			}
			appendToken(tokenText, jsonText[start:i])
		case '{', '}', '[', ']', ':', ',':
			appendToken(tokenPunctuation, jsonText[i:i+1])
			i++
		case '"':
			start := i
			i = parseJSONString(jsonText, i)
			appendToken(classifyStringToken(jsonText, i), jsonText[start:i])
		case 't':
			if strings.HasPrefix(jsonText[i:], "true") {
				appendToken(tokenBool, "true")
				i += len("true")
				continue
			}
			appendToken(tokenText, jsonText[i:i+1])
			i++
		case 'f':
			if strings.HasPrefix(jsonText[i:], "false") {
				appendToken(tokenBool, "false")
				i += len("false")
				continue
			}
			appendToken(tokenText, jsonText[i:i+1])
			i++
		case 'n':
			if strings.HasPrefix(jsonText[i:], "null") {
				appendToken(tokenNull, "null")
				i += len("null")
				continue
			}
			appendToken(tokenText, jsonText[i:i+1])
			i++
		default:
			if isNumberStart(jsonText[i]) {
				start := i
				i = parseJSONNumber(jsonText, i)
				appendToken(tokenNumber, jsonText[start:i])
				continue
			}
			appendToken(tokenText, jsonText[i:i+1])
			i++
		}
	}

	return lines
}

func classifyStringToken(jsonText string, end int) tokenKind {
	for i := end; i < len(jsonText); i++ {
		switch jsonText[i] {
		case ' ', '\t', '\r':
			continue
		case ':':
			return tokenKey
		default:
			return tokenString
		}
	}
	return tokenString
}

func parseJSONString(jsonText string, start int) int {
	for i := start + 1; i < len(jsonText); i++ {
		switch jsonText[i] {
		case '\\':
			if i+1 < len(jsonText) {
				i++
			}
		case '"':
			return i + 1
		}
	}
	return len(jsonText)
}

func parseJSONNumber(jsonText string, start int) int {
	i := start
	if jsonText[i] == '-' {
		i++
	}

	if i < len(jsonText) && jsonText[i] == '0' {
		i++
	} else {
		for i < len(jsonText) && isDigit(jsonText[i]) {
			i++
		}
	}

	if i < len(jsonText) && jsonText[i] == '.' {
		i++
		for i < len(jsonText) && isDigit(jsonText[i]) {
			i++
		}
	}

	if i < len(jsonText) && (jsonText[i] == 'e' || jsonText[i] == 'E') {
		i++
		if i < len(jsonText) && (jsonText[i] == '+' || jsonText[i] == '-') {
			i++
		}
		for i < len(jsonText) && isDigit(jsonText[i]) {
			i++
		}
	}

	return i
}

func isInlineWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r'
}

func isNumberStart(b byte) bool {
	return b == '-' || isDigit(b)
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
