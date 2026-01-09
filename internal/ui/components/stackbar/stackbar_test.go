package stackbar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestViewDimensions(t *testing.T) {
	styles := Styles{
		Bar:   lipgloss.NewStyle(),
		Item:  lipgloss.NewStyle(),
		Arrow: lipgloss.NewStyle(),
	}

	tests := map[string]struct {
		width int
		stack []string
	}{
		"no width": {width: 0, stack: []string{"Dashboard"}},
		"single":   {width: 20, stack: []string{"Dashboard"}},
		"multi":    {width: 30, stack: []string{"Errors", "Detail"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(
				WithStyles(styles),
				WithWidth(tc.width),
				WithStack(tc.stack),
			)
			output := m.View()
			if tc.width == 0 {
				if !strings.Contains(output, "Dashboard") {
					t.Fatalf("expected output to contain stack labels, got %q", output)
				}
				return
			}
			if w := ansi.StringWidth(output); w != tc.width {
				t.Fatalf("expected width %d, got %d", tc.width, w)
			}
		})
	}
}

func TestViewFormatsStack(t *testing.T) {
	const arrow = ""
	labels := []string{"Errors", "Detail"}
	items := []string{
		labels[0] + arrow,
		labels[1] + arrow,
	}
	expected := strings.Join(items, " ")

	m := New(
		WithStyles(Styles{
			Bar:   lipgloss.NewStyle(),
			Item:  lipgloss.NewStyle(),
			Arrow: lipgloss.NewStyle(),
		}),
		WithStack(labels),
	)

	if got := ansi.Strip(m.View()); got != expected {
		t.Fatalf("View() = %q, want %q", got, expected)
	}
}

func TestGoldenStackBar(t *testing.T) {
	m := New(
		WithStyles(Styles{
			Bar:   lipgloss.NewStyle(),
			Item:  lipgloss.NewStyle(),
			Arrow: lipgloss.NewStyle(),
		}),
		WithWidth(40),
		WithStack([]string{"Errors", "Detail", "Payload"}),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
