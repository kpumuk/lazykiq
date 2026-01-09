package errorpopup

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestHasError(t *testing.T) {
	tests := map[string]struct {
		message string
		want    bool
	}{
		"empty":  {message: "", want: false},
		"filled": {message: "boom", want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(WithMessage(tc.message))
			if got := m.HasError(); got != tc.want {
				t.Fatalf("HasError() = %v, want %v", got, tc.want)
			}
			if got := m.Message(); got != tc.message {
				t.Fatalf("Message() = %q, want %q", got, tc.message)
			}
		})
	}
}

func TestViewDimensions(t *testing.T) {
	tests := map[string]struct {
		width     int
		height    int
		message   string
		wantEmpty bool
	}{
		"empty message": {width: 60, height: 7, message: "", wantEmpty: true},
		"too narrow":    {width: 1, height: 7, message: "boom", wantEmpty: true},
		"zero height":   {width: 60, height: 0, message: "boom", wantEmpty: true},
		"normal":        {width: 80, height: 7, message: "boom", wantEmpty: false},
		"clamped width": {width: 120, height: 7, message: "boom", wantEmpty: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := New(WithSize(tc.width, tc.height), WithMessage(tc.message))
			output := m.View()
			if tc.wantEmpty {
				if output != "" {
					t.Fatalf("expected empty output, got %q", output)
				}
				return
			}

			stripped := ansi.Strip(output)
			lines := strings.Split(stripped, "\n")

			messageLines := strings.Split(tc.message+"\n\nRetrying every 5 seconds...", "\n")
			expectedHeight := min(len(messageLines)+2, tc.height)
			if len(lines) != expectedHeight {
				t.Fatalf("expected %d lines, got %d", expectedHeight, len(lines))
			}

			expectedWidth := min(tc.width, 60)
			for i, line := range lines {
				if w := ansi.StringWidth(line); w != expectedWidth {
					t.Fatalf("line %d: expected width %d, got %d", i, expectedWidth, w)
				}
			}
		})
	}
}

func TestGoldenErrorPopup(t *testing.T) {
	m := New(
		WithSize(60, 7),
		WithMessage("Redis connection failed"),
	)
	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
