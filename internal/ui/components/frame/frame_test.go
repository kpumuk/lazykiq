package frame

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestFrameLineCountAndWidth(t *testing.T) {
	box := New(
		WithSize(10, 4),
		WithTitle("T"),
		WithTitlePadding(1),
		WithContent("hi"),
	)

	view := box.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 4 {
		t.Fatalf("want 4 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 10 {
			t.Fatalf("line %d: want width 10, got %d", i, lipgloss.Width(line))
		}
	}
}

func TestFrameMinHeight(t *testing.T) {
	box := New(
		WithSize(10, 2),
		WithMinHeight(5),
		WithTitle("T"),
		WithTitlePadding(1),
		WithContent("hi"),
	)

	view := box.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 5 {
		t.Fatalf("want 5 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 10 {
			t.Fatalf("line %d: want width 10, got %d", i, lipgloss.Width(line))
		}
	}
}

func TestFrameFocusStyles(t *testing.T) {
	focusedBorder := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styles := Styles{
		Focused: StyleState{
			Title:  lipgloss.NewStyle(),
			Border: focusedBorder,
		},
		Blurred: StyleState{
			Title:  lipgloss.NewStyle(),
			Border: lipgloss.NewStyle(),
		},
	}

	focused := New(
		WithStyles(styles),
		WithFocused(true),
		WithTitle("T"),
		WithSize(8, 3),
	)
	unfocused := New(
		WithStyles(styles),
		WithFocused(false),
		WithTitle("T"),
		WithSize(8, 3),
	)

	if !strings.Contains(focused.View(), "\x1b[") {
		t.Fatalf("expected focused view to contain ANSI sequences")
	}
	if strings.Contains(unfocused.View(), "\x1b[") {
		t.Fatalf("expected unfocused view to avoid ANSI sequences")
	}
}

func TestGoldenFrameBasic(t *testing.T) {
	box := New(
		WithSize(20, 5),
		WithTitle("Title"),
		WithTitlePadding(1),
		WithContent("hello"),
	)

	output := ansi.Strip(box.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenFrameWithMeta(t *testing.T) {
	box := New(
		WithSize(30, 6),
		WithTitle("Queue"),
		WithTitlePadding(1),
		WithMeta("3 items"),
		WithMetaPadding(1),
		WithContent("row 1\nrow 2"),
	)

	output := ansi.Strip(box.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenFrameWithFilter(t *testing.T) {
	box := New(
		WithSize(26, 4),
		WithTitle("Errors"),
		WithFilter("timeout"),
		WithTitlePadding(1),
		WithContent("row"),
	)

	output := ansi.Strip(box.View())
	golden.RequireEqual(t, []byte(output))
}
