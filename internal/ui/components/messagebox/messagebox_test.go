package messagebox

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestViewDimensions(t *testing.T) {
	tests := map[string]struct {
		width     int
		height    int
		wantEmpty bool
	}{
		"zero width":   {width: 0, height: 5, wantEmpty: true},
		"zero height":  {width: 20, height: 0, wantEmpty: false},
		"short height": {width: 20, height: 3, wantEmpty: false},
		"normal":       {width: 30, height: 6, wantEmpty: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(
				WithSize(tc.width, tc.height),
				WithTitle("Empty"),
				WithMessage("No data"),
			)
			output := m.View()
			if tc.wantEmpty {
				if output != "" {
					t.Fatalf("expected empty output, got %q", output)
				}
				return
			}

			lines := strings.Split(ansi.Strip(output), "\n")
			expectedHeight := max(tc.height, 5)
			if len(lines) != expectedHeight {
				t.Fatalf("expected %d lines, got %d", expectedHeight, len(lines))
			}
			for i, line := range lines {
				if w := ansi.StringWidth(line); w != tc.width {
					t.Fatalf("line %d: expected width %d, got %d", i, tc.width, w)
				}
			}
		})
	}
}

func TestMessageRendered(t *testing.T) {
	m := New(
		WithSize(30, 5),
		WithTitle("Empty"),
		WithMessage("No data"),
	)
	output := ansi.Strip(m.View())
	if !strings.Contains(output, "No data") {
		t.Fatalf("expected message to be rendered, got %q", output)
	}
}

func TestGoldenMessageBox(t *testing.T) {
	m := New(
		WithSize(30, 5),
		WithTitle("Empty"),
		WithMessage("No data"),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
