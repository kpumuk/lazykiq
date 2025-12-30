package stackbar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestViewRendersSingleItemWithArrows(t *testing.T) {
	const arrow = ""
	label := "Dashboard"
	expected := arrow + label + arrow

	m := New(
		WithStyles(Styles{
			Bar:        lipgloss.NewStyle(),
			Item:       lipgloss.NewStyle(),
			ArrowLeft:  lipgloss.NewStyle(),
			ArrowRight: lipgloss.NewStyle(),
		}),
		WithStack([]string{label}),
	)

	if got := m.View(); got != expected {
		t.Fatalf("View() = %q, want %q", got, expected)
	}
}

func TestViewRendersMultipleItemsSeparatedBySpace(t *testing.T) {
	const arrow = ""
	labels := []string{"Errors", "Detail"}
	items := []string{
		arrow + labels[0] + arrow,
		arrow + labels[1] + arrow,
	}
	expected := strings.Join(items, " ")

	m := New(
		WithStyles(Styles{
			Bar:        lipgloss.NewStyle(),
			Item:       lipgloss.NewStyle(),
			ArrowLeft:  lipgloss.NewStyle(),
			ArrowRight: lipgloss.NewStyle(),
		}),
		WithStack(labels),
	)

	if got := m.View(); got != expected {
		t.Fatalf("View() = %q, want %q", got, expected)
	}
}
