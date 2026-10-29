package jsonview

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestTokenCutMatchesANSI(t *testing.T) {
	for name, text := range map[string]string{
		"ascii":             strings.Repeat("abcdef", 400),
		"unicode":           strings.Repeat("界e\u0301👩🏽‍💻🇨🇦", 100),
		"ascii combining":   strings.Repeat("1️⃣#️⃣e\u0301", 200),
		"zero width":        strings.Repeat("\u200b\u0301\u0085x", 300),
		"long grapheme":     "e" + strings.Repeat("\u0301", 2048) + "界tail",
		"regional sequence": strings.Repeat("🇨🇦🇺🇸🇬", 150),
		"escaped":           strings.Repeat("\"\\\n\t\x1b[31m", 200),
		"delete control":    strings.Repeat("a\x7fb", 800),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(text)
			if err != nil {
				t.Fatal(err)
			}
			tok := newToken(tokenString, string(data))
			columns := ansi.StringWidth(tok.value)
			if tok.width() != columns {
				t.Fatalf("cached width %d, want %d", tok.width(), columns)
			}
			for _, left := range []int{0, 1, 2, 3, 31, 255, 256, 1023, 1024, 1025, columns / 2, columns - 1} {
				if left >= columns {
					continue
				}
				for _, width := range []int{1, 2, 5, 80, 320, columns} {
					right := min(left+width, columns)
					if got, want := tok.cut(left, right), ansi.Cut(tok.value, left, right); got != want {
						t.Fatalf("cut(%d, %d):\ngot  %q\nwant %q", left, right, got, want)
					}
				}
			}
		})
	}
}

func FuzzTokenCut(f *testing.F) {
	for _, text := range []string{"plain", "界e\u0301👩🏽‍💻🇨🇦", "1️⃣#️⃣", "\x7f\u200b\u0301", "\"\\\n\x1b[31m"} {
		f.Add(text, uint16(1), uint16(3))
	}
	f.Fuzz(func(t *testing.T, text string, offset, width uint16) {
		if len(text) > 8192 {
			t.Skip()
		}
		data, err := json.Marshal(text)
		if err != nil {
			t.Fatal(err)
		}
		tok := newToken(tokenString, string(data))
		left := int(offset) % tok.width()
		right := min(left+int(width)+1, tok.width())
		if got, want := tok.cut(left, right), ansi.Cut(tok.value, left, right); got != want {
			t.Fatalf("cut(%d, %d):\ngot  %q\nwant %q", left, right, got, want)
		}
	})
}

func TestRenderLineMatchesUncached(t *testing.T) {
	styles := DefaultStyles()
	styles.Key = styles.Key.Bold(true)
	styles.String = styles.String.Underline(true)
	styles.Number = styles.Number.Italic(true)
	for name, value := range map[string]any{
		"mixed": map[string]any{
			"escaped": "quote: \" slash: \\ escape: \x1b[31m",
			"unicode": strings.Repeat("界e\u0301👩🏽‍💻🇨🇦", 120),
			"number":  -12.5e20,
			"bool":    true,
			"nil":     nil,
		},
		"wide key":  map[string]any{strings.Repeat("👩🏽‍💻", 100): "value"},
		"wide text": map[string]any{"args": strings.Repeat("x", 1048576)},
	} {
		t.Run(name, func(t *testing.T) {
			m := New(WithStyles(styles))
			m.SetValue(value)
			for line := range m.LineCount() {
				columns := ansi.StringWidth(m.lines[line])
				for _, offset := range []int{-3, 0, 1, 10, columns / 2, columns - 8, columns + 1} {
					for _, width := range []int{1, 7, 80} {
						got := m.RenderLine(line, offset, width)
						want := renderUncached(m, line, offset, width)
						if got != want {
							t.Fatalf("line=%d offset=%d width=%d:\ngot  %q\nwant %q", line, offset, width, got, want)
						}
					}
				}
			}
		})
	}
}

// Keep the previous renderer as an output oracle, including token-local wide
// character cuts and ANSI styling, rather than only comparing stripped text.
func renderUncached(m Model, index, offset, width int) string {
	offset = max(offset, 0)
	end := offset + width
	var builder strings.Builder
	column := 0
	for _, tok := range m.tokens[index] {
		tokenWidth := lipgloss.Width(tok.value)
		if tokenWidth == 0 {
			continue
		}
		tokenEnd := column + tokenWidth
		if tokenEnd > offset && column < end {
			start := min(max(offset-column, 0), tokenWidth)
			stop := min(max(end-column, 0), tokenWidth)
			if segment := ansi.Cut(tok.value, start, stop); segment != "" {
				builder.WriteString(m.styleForToken(tok).Render(segment))
			}
		}
		column = tokenEnd
		if column >= end {
			break
		}
	}
	rendered := builder.String()
	if renderedWidth := lipgloss.Width(rendered); renderedWidth < width {
		rendered += strings.Repeat(" ", width-renderedWidth)
	}
	return rendered
}

func TestTokenIndexIsSparse(t *testing.T) {
	ascii := newToken(tokenString, strings.Repeat("x", 1048576))
	if ascii.layout != nil {
		t.Fatal("ASCII tokens should use byte offsets without an index")
	}
	unicode := newToken(tokenString, strings.Repeat("界e\u0301👩🏽‍💻🇨🇦", 32768))
	checkpoints := unicode.layout.checkpoints
	if len(checkpoints) == 0 || len(checkpoints) > len(unicode.value)/checkpointBytes {
		t.Fatalf("unexpected checkpoint count: %d for %d bytes", len(checkpoints), len(unicode.value))
	}
	last := tokenPosition{}
	for _, position := range checkpoints {
		if position.byteOffset-last.byteOffset < checkpointBytes || position.column < last.column {
			t.Fatalf("invalid checkpoint: %+v after %+v", position, last)
		}
		last = position
	}
}

func TestSetValueReplacesIndex(t *testing.T) {
	m := New()
	m.SetValue(strings.Repeat("界", 2048))
	old := m
	oldLayout := old.tokens[0][0].layout
	want := old.RenderLine(0, 2048, 20)
	for name, value := range map[string]any{
		"ascii":   "short",
		"unicode": strings.Repeat("👩🏽‍💻1️⃣", 200),
		"nil":     nil,
		"error":   make(chan int),
	} {
		t.Run(name, func(t *testing.T) {
			m.SetValue(value)
			for _, line := range m.tokens {
				for _, tok := range line {
					if tok.layout == oldLayout {
						t.Fatal("old Unicode index survived replacement")
					}
				}
			}
			fresh := New()
			fresh.SetValue(value)
			if m.LineCount() != fresh.LineCount() || m.MaxWidth() != fresh.MaxWidth() || renderAll(m, 0, 20) != renderAll(fresh, 0, 20) {
				t.Fatal("replacement differs from a fresh model")
			}
			if got := old.RenderLine(0, 2048, 20); got != want {
				t.Fatal("replacement mutated the previous model's immutable tokens")
			}
		})
	}
}

func TestSetStylesUsesCurrentStyles(t *testing.T) {
	m := New()
	m.SetValue(strings.Repeat("界", 2048))
	before := m.RenderLine(0, 2048, 20)
	styles := DefaultStyles()
	styles.String = styles.String.Bold(true)
	m.SetStyles(styles)
	after := m.RenderLine(0, 2048, 20)
	if before == after || ansi.Strip(before) != ansi.Strip(after) {
		t.Fatal("style replacement should update ANSI styling and preserve clipped text")
	}
	if want := renderUncached(m, 0, 2048, 20); after != want {
		t.Fatalf("style replacement: got %q, want %q", after, want)
	}
}

func TestGoldenJSONViewUnicodeScroll(t *testing.T) {
	m := New()
	m.SetValue(map[string]any{
		"args": strings.Repeat("界e\u0301👩🏽‍💻🇨🇦", 60),
		"text": "quote: \" slash: \\ newline: \n escape: \x1b[31m",
	})
	var output strings.Builder
	for _, tc := range []struct{ offset, width int }{{0, 24}, {10, 13}, {221, 17}, {428, 24}, {900, 8}} {
		fmt.Fprintf(&output, "offset=%d width=%d\n", tc.offset, tc.width)
		for line := range strings.SplitSeq(ansi.Strip(renderAll(m, tc.offset, tc.width)), "\n") {
			fmt.Fprintf(&output, "%q\n", line)
		}
	}
	golden.RequireEqual(t, []byte(output.String()))
}

func TestMaxWidthTerminalCells(t *testing.T) {
	for name, value := range map[string]any{
		"ascii":          strings.Repeat("x", 80),
		"multibyte":      strings.Repeat("é", 40),
		"wide":           strings.Repeat("界", 40),
		"combining":      strings.Repeat("e\u0301", 40),
		"emoji":          strings.Repeat("👩🏽‍💻🇨🇦1️⃣", 40),
		"zero width":     strings.Repeat("\u200b\u0301", 40),
		"escaped":        strings.Repeat("\"\\\n\x1b[31m", 40),
		"unicode keys":   map[string]any{strings.Repeat("界", 40): "é"},
		"multiple lines": map[string]any{"ascii": "short", "args": []any{strings.Repeat("界e\u0301👩🏽‍💻", 40)}},
	} {
		t.Run(name, func(t *testing.T) {
			m := New()
			m.SetValue(value)
			data, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			want := lipgloss.Width(string(data))
			if m.MaxWidth() != want {
				t.Fatalf("MaxWidth = %d; want %d terminal cells", m.MaxWidth(), want)
			}
			m.SetValue("short")
			if m.MaxWidth() != 7 {
				t.Fatalf("replacement width = %d; want 7", m.MaxWidth())
			}
			m.SetValue(nil)
			if m.MaxWidth() != 0 {
				t.Fatalf("cleared width = %d; want 0", m.MaxWidth())
			}
		})
	}
}

func TestGoldenJSONViewEnd(t *testing.T) {
	m := New()
	m.SetValue(map[string]any{"args": strings.Repeat("界e\u0301👩🏽‍💻", 40) + "TAIL"})
	var output strings.Builder
	for _, width := range []int{10, 20, 40} {
		offset := max(m.MaxWidth()-width, 0)
		fmt.Fprintf(&output, "width=%d offset=%d\n", width, offset)
		for line := range strings.SplitSeq(ansi.Strip(renderAll(m, offset, width)), "\n") {
			fmt.Fprintf(&output, "%q\n", line)
		}
	}
	golden.RequireEqual(t, []byte(output.String()))
}
