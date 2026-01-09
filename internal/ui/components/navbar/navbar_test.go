package navbar

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestViewDimensions(t *testing.T) {
	views := []ViewInfo{{Name: "Dashboard"}, {Name: "Queues"}}
	help := key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))

	tests := map[string]struct {
		width     int
		wantEmpty bool
	}{
		"zero width": {width: 0, wantEmpty: true},
		"narrow":     {width: 20, wantEmpty: false},
		"wide":       {width: 60, wantEmpty: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(
				WithWidth(tc.width),
				WithViews(views),
				WithBrand("lazykiq"),
				WithHelp(help),
			)
			output := m.View()
			if tc.wantEmpty {
				if output != "" {
					t.Fatalf("expected empty output, got %q", output)
				}
				return
			}
			if w := ansi.StringWidth(output); w != tc.width {
				t.Fatalf("expected width %d, got %d", tc.width, w)
			}
		})
	}
}

func TestBrandAndHelpRendered(t *testing.T) {
	views := []ViewInfo{{Name: "Dashboard"}, {Name: "Queues"}}
	help := key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))

	m := New(
		WithWidth(80),
		WithViews(views),
		WithBrand("lazykiq"),
		WithHelp(help),
	)
	output := ansi.Strip(m.View())
	if !strings.Contains(output, "lazykiq") {
		t.Fatalf("expected brand to be rendered, got %q", output)
	}
	if !strings.Contains(output, "help") {
		t.Fatalf("expected help to be rendered, got %q", output)
	}
}

func TestGoldenNavbar(t *testing.T) {
	views := []ViewInfo{
		{Name: "Dashboard"},
		{Name: "Queues"},
		{Name: "Busy"},
	}
	help := key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))

	m := New(
		WithWidth(60),
		WithViews(views),
		WithBrand("lazykiq"),
		WithHelp(help),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
