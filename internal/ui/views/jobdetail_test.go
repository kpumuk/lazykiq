package views

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/display"
)

func TestWrapText(t *testing.T) {
	for name, tc := range map[string]struct {
		text  string
		width int
		want  []string
	}{
		"empty":             {"", 4, []string{""}},
		"zero width":        {"a\nb", 0, []string{"a\nb"}},
		"negative width":    {"abcd", -1, []string{"abcd"}},
		"fits":              {"abcd", 4, []string{"abcd"}},
		"exact multiple":    {"abcdefgh", 4, []string{"abcd", "efgh"}},
		"spaces":            {"ab  cd ", 3, []string{"ab ", " cd", " "}},
		"newlines":          {"a\n\nabcdef\n", 3, []string{"a", "", "abc", "def", ""}},
		"wide":              {"界界a", 3, []string{"界", "界a"}},
		"oversize grapheme": {"界a", 1, []string{"界", "a"}},
		"combining mark":    {"ae\u0301b", 2, []string{"ae\u0301", "b"}},
		"emoji":             {"a👩🏽‍💻b", 2, []string{"a", "👩🏽‍💻", "b"}},
		"flag":              {"ab🇨🇦z", 3, []string{"ab", "🇨🇦z"}},
		"keycap":            {"1\ufe0f\u20e3x", 2, []string{"1\ufe0f\u20e3", "x"}},
		"invalid UTF8":      {"a\xffb", 2, []string{"a\xff", "b"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := wrapText(tc.text, tc.width); !slices.Equal(got, tc.want) {
				t.Fatalf("wrapText(%q, %d) = %q; want %q", tc.text, tc.width, got, tc.want)
			}
		})
	}
}

func TestWrapTextANSI(t *testing.T) {
	for name, input := range map[string]string{
		"color":      "\x1b[31mabcdefgh\x1b[0m",
		"attributes": "\x1b[1;3mab\x1b[22mcd\x1b[4:3mef\x1b[0mgh",
		"truecolor":  "\x1b[38;2;12;34;56mabcdefgh\x1b[m",
		"hyperlink":  ansi.SetHyperlink("https://example.com", "id=test") + "abcdefgh" + ansi.SetHyperlink(""),
		"mixed":      "\x1b[32mab" + ansi.SetHyperlink("https://example.com") + "cd界e\u0301f" + ansi.SetHyperlink("") + "\x1b[m",
		"newlines":   "\x1b[31mab\n\ncdef\n\x1b[m",
	} {
		t.Run(name, func(t *testing.T) {
			var want uv.Line
			for _, line := range uv.NewStyledString(input).Lines(ansi.GraphemeWidth) {
				want = append(want, line...)
			}
			var got uv.Line
			for _, line := range wrapText(input, 3) {
				if ansi.StringWidth(line) > 3 || strings.Contains(line, "\n") {
					t.Fatalf("invalid wrapped line: %q", line)
				}
				// Each line starts from a fresh terminal state, as it does when
				// it becomes the first visible row after scrolling.
				cells := uv.NewStyledString(line + "!").Lines(ansi.GraphemeWidth)[0]
				last := cells[len(cells)-1]
				if !last.Style.IsZero() || !last.Link.IsZero() {
					t.Fatalf("ANSI state leaked beyond line: %q", line)
				}
				got = append(got, cells[:len(cells)-1]...)
			}
			if len(got) != len(want) {
				t.Fatalf("wrapped cells = %d; want %d", len(got), len(want))
			}
			for i := range want {
				if !got[i].Equal(&want[i]) {
					t.Fatalf("cell %d changed: %#v; want %#v", i, got[i], want[i])
				}
			}
		})
	}
}

func TestJobDetailLargeProperties(t *testing.T) {
	for _, size := range []int{1024, 4096, 16384, 65536} {
		payload := strings.Repeat("x", size-4) + "TAIL"
		for label, field := range map[string]string{
			"Args": "args", "Error": "error_message", "Backtrace": "error_backtrace",
		} {
			t.Run(fmt.Sprintf("%s/%d", label, size), func(t *testing.T) {
				item := map[string]any{"jid": "wide", "class": "TestJob", "queue": "default"}
				want := payload
				switch label {
				case "Args":
					item[field] = []any{payload}
					want = display.Args([]any{payload})
				case "Backtrace":
					item["error_class"] = "RuntimeError"
					item[field] = []string{payload}
				case "Error":
					item["error_class"] = "RuntimeError"
					item[field] = payload
				}
				raw, err := json.Marshal(item)
				if err != nil {
					t.Fatal(err)
				}
				view := NewJobDetail()
				view.SetJob(sidekiq.NewJobRecord(string(raw), "default"))
				// Copy uses this complete JSON, independently of the cached lines.
				var copied map[string]any
				if err := json.Unmarshal([]byte(view.jobJSON()), &copied); err != nil {
					t.Fatal(err)
				}
				if fmt.Sprint(copied[field]) != fmt.Sprint(item[field]) {
					t.Fatal("copy JSON lost property content")
				}
				for _, width := range []int{80, 160, 240} {
					view.SetSize(width, 12)
					valueWidth := view.leftWidth - 2 - 2*jobDetailPanelPadding - jobDetailValueIndent
					count := 0
					for _, property := range view.properties {
						count += 1 + max((len(property.Value)+valueWidth-1)/valueWidth, 1)
					}
					if view.countLeftPanelLines() != count {
						t.Fatalf("width %d: line count = %d; want %d", width, view.countLeftPanelLines(), count)
					}
					var parts []string
					collect := false
					for _, line := range view.leftLines {
						if line.kind == jobDetailLabelLine {
							collect = line.text == label+":"
						} else if collect {
							parts = append(parts, line.text)
						}
					}
					if strings.Join(parts, "") != want {
						t.Fatalf("width %d: wrapped property changed", width)
					}
					view.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
					if view.leftYOffset != count-view.panelHeight {
						t.Fatal("cannot reach the last property line")
					}
					output := ansi.Strip(view.renderLeftPanel())
					if !strings.Contains(output, "TAIL") || lipgloss.Height(output) != 12 || lipgloss.Width(output) != view.leftWidth {
						t.Fatalf("width %d: wrong scrolled panel dimensions or missing tail", width)
					}
					view.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
					if view.leftYOffset != view.maxLeftYOffset() {
						t.Fatal("scrolled beyond last line")
					}
				}
			})
		}
	}
}

func TestJobDetailCacheLifecycle(t *testing.T) {
	view := NewJobDetail()
	view.SetSize(80, 10)
	job := sidekiq.NewJobRecord(fmt.Sprintf(`{"jid":"same","class":"TestJob","args":[%q]}`, strings.Repeat("x", 4096)), "default")
	view.SetJob(job)
	initialCount := view.countLeftPanelLines()
	initial := &view.leftLines[0]
	view.SetSize(80, 16) // A height change does not rewrap.
	styled := 0
	view.SetStyles(Styles{Text: lipgloss.NewStyle().Transform(func(s string) string {
		styled++
		return s
	})})
	view.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	view.renderLeftPanel()
	if styled == 0 || styled > view.panelHeight {
		t.Fatalf("styled %d values for %d visible rows", styled, view.panelHeight)
	}
	if view.countLeftPanelLines() != initialCount || &view.leftLines[0] != initial {
		t.Fatal("height, styles, redraw or scrolling rebuilt prepared lines")
	}
	if allocs := testing.AllocsPerRun(10, func() { view.maxLeftYOffset() }); allocs != 0 {
		t.Fatalf("cached scroll bounds allocated %g times", allocs)
	}
	view.SetSize(240, 16)
	if &view.leftLines[0] == initial || view.countLeftPanelLines() >= initialCount || view.leftYOffset != view.maxLeftYOffset() {
		t.Fatal("width change did not rewrap and clamp scrolling")
	}
	view.SetSize(80, 16)
	if view.countLeftPanelLines() != initialCount {
		t.Fatal("narrowing did not restore wrapped line count")
	}
	// A replacement with the same JID must still invalidate the cache.
	view.SetJob(sidekiq.NewJobRecord(`{"jid":"same","class":"Replacement","args":["new"]}`, "default"))
	if view.leftLines != nil || view.leftYOffset != 0 || view.rightYOffset != 0 || view.rightXOffset != 0 || view.focusRight {
		t.Fatal("job change retained prepared lines or scroll state")
	}
	output := view.View()
	if !strings.Contains(output, "Replacement") || strings.Contains(output, "TestJob") {
		t.Fatal("job change retained old property content")
	}
	view.Dispose()
	if view.leftLines != nil || view.properties != nil || view.countLeftPanelLines() != 0 || view.jobJSON() != "" {
		t.Fatal("dispose retained job data")
	}
	for _, size := range [][2]int{{0, 0}, {1, 1}} {
		view.SetSize(size[0], size[1])
		view.View() // Uninitialized/small terminals must not panic.
	}
}

func TestJobDetailMultilineScrollBounds(t *testing.T) {
	view := NewJobDetail()
	view.SetSize(100, 8)
	view.SetJob(sidekiq.NewJobRecord(`{"jid":"multiline","class":"TestJob","error_class":"RuntimeError","error_message":"first\n\nlast\n"}`, "default"))
	if view.countLeftPanelLines() != 13 {
		t.Fatalf("multiline properties have %d lines; want 13", view.countLeftPanelLines())
	}
	view.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if view.leftYOffset != 7 {
		t.Fatalf("last offset = %d; want 7", view.leftYOffset)
	}
	output := ansi.Strip(view.renderLeftPanel())
	if !strings.Contains(output, "first") || !strings.Contains(output, "last") || lipgloss.Height(output) != 8 {
		t.Fatal("multiline content was lost or changed panel height")
	}
	view.SetSize(100, 20)
	if view.leftYOffset != 0 || view.maxLeftYOffset() != 0 {
		t.Fatal("height change did not clamp multiline scrolling")
	}
}
