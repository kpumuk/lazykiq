package scrollbar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func blankStyles() Styles {
	return Styles{
		Track: lipgloss.NewStyle(),
		Thumb: lipgloss.NewStyle(),
	}
}

func TestScrollbar_NoScroll(t *testing.T) {
	bar := New(
		WithStyles(blankStyles()),
		WithSize(1, 3),
		WithRange(5, 5, 0),
	)

	got := bar.View()
	want := " \n \n "
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestScrollbar_ThumbAlwaysVisible(t *testing.T) {
	bar := New(
		WithStyles(blankStyles()),
		WithSize(1, 4),
		WithRange(100, 1, 50),
	)

	got := bar.View()
	if strings.Count(got, scrollbarThumb) == 0 {
		t.Fatalf("expected at least one thumb rune, got %q", got)
	}
}

// Golden tests for layout stability.

func TestGoldenScrollbarStart(t *testing.T) {
	bar := New(
		WithStyles(blankStyles()),
		WithSize(1, 5),
		WithRange(100, 10, 0),
	)

	output := ansi.Strip(bar.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenScrollbarMiddle(t *testing.T) {
	bar := New(
		WithStyles(blankStyles()),
		WithSize(1, 6),
		WithRange(60, 10, 25),
	)

	output := ansi.Strip(bar.View())
	golden.RequireEqual(t, []byte(output))
}
