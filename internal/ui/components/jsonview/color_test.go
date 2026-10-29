package jsonview

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/ui/theme"
)

func TestRenderLineSyntaxColors(t *testing.T) {
	for name, dark := range map[string]bool{"light": false, "dark": true} {
		t.Run(name, func(t *testing.T) {
			for _, profile := range []colorprofile.Profile{colorprofile.ANSI, colorprofile.ANSI256, colorprofile.TrueColor} {
				t.Run(profile.String(), func(t *testing.T) {
					styles := syntaxColorStylesForBackground(dark)
					for name, tc := range map[string]struct {
						value any
						style lipgloss.Style
					}{
						"string": {value: "界e\u0301👩🏽‍💻 \"\\\n\x1b[31m", style: styles.String},
						"number": {value: -12.5, style: styles.Number},
						"bool":   {value: true, style: styles.Bool},
						"null":   {value: nil, style: styles.Null},
					} {
						t.Run(name, func(t *testing.T) {
							literal, err := json.Marshal(tc.value)
							if err != nil {
								t.Fatal(err)
							}
							m := New(WithStyles(styles))
							m.SetValue(map[string]any{"value": tc.value})
							want := styles.Text.Render("  ") + styles.Key.Render(`"value"`) +
								styles.Punctuation.Render(":") + styles.Text.Render(" ") + tc.style.Render(string(literal))
							want += strings.Repeat(" ", 80-ansi.StringWidth(want))
							if got := downsample(t, profile, m.RenderLine(1, 0, 80)); got != downsample(t, profile, want) {
								t.Fatalf("colored tokens:\ngot  %q\nwant %q", got, want)
							}
							for _, rendered := range []string{styles.Key.Render(`"value"`), styles.Punctuation.Render(":"), tc.style.Render(string(literal))} {
								if !strings.Contains(rendered, "\x1b[") || rendered == ansi.Strip(rendered) {
									t.Fatalf("expected actual syntax color codes, got %q", rendered)
								}
							}
						})
					}
					m := New(WithStyles(styles))
					m.SetValue(map[string]any{
						"ascii":   strings.Repeat("x", 8192),
						"unicode": strings.Repeat("界e\u0301👩🏽‍💻🇨🇦", 300),
					})
					for line := range m.LineCount() {
						columns := ansi.StringWidth(m.lines[line])
						for _, offset := range []int{0, 7, 1023, 1024, columns / 2, columns - 3, columns + 1} {
							for _, width := range []int{1, 7, 80} {
								if got, want := m.RenderLine(line, offset, width), renderUncached(m, line, offset, width); got != want {
									t.Fatalf("line=%d offset=%d width=%d:\ngot  %q\nwant %q", line, offset, width, got, want)
								}
							}
						}
					}
					before := m.RenderLine(2, 1024, 80)
					replacement := styles
					replacement.String = styles.Key
					m.SetStyles(replacement)
					after := m.RenderLine(2, 1024, 80)
					if before == after || ansi.Strip(before) != ansi.Strip(after) || after != renderUncached(m, 2, 1024, 80) {
						t.Fatal("replacing syntax colors should update ANSI codes without changing clipped text")
					}
				})
			}
		})
	}
}

func syntaxColorStylesForBackground(dark bool) Styles {
	s := theme.NewStylesForBackground(dark)
	return Styles{
		Text:        s.ViewText,
		Key:         s.JSONKey,
		String:      s.JSONString,
		Number:      s.JSONNumber,
		Bool:        s.JSONBool,
		Null:        s.JSONNull,
		Punctuation: s.JSONPunctuation,
		Muted:       s.ViewMuted,
	}
}

func downsample(t *testing.T, profile colorprofile.Profile, text string) string {
	t.Helper()
	var out bytes.Buffer
	w := colorprofile.Writer{Forward: &out, Profile: profile}
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
