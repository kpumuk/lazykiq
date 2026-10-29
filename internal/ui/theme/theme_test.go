package theme

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestQueuePausedStyle(t *testing.T) {
	style := NewStyles().QueuePaused
	if _, unset := style.GetForeground().(lipgloss.NoColor); unset {
		t.Fatal("paused status has no foreground color")
	}
	if _, unset := style.GetBackground().(lipgloss.NoColor); !unset {
		t.Fatal("paused status must not have a badge background")
	}
	if style.GetHorizontalPadding() != 0 {
		t.Fatal("paused status must not add badge padding")
	}
}

func TestPaletteResolutionIsIndependent(t *testing.T) {
	dark := NewStyles()
	light := NewStylesForBackground(false)
	if dark.ViewText.GetForeground() == light.ViewText.GetForeground() {
		t.Fatal("light and dark text colors match")
	}
	if dark.ViewText.GetForeground() != NewStyles().ViewText.GetForeground() {
		t.Fatal("resolving light palette changed the fallback")
	}
	if dark.ChartHistogram.GetForeground() == light.ChartHistogram.GetForeground() {
		t.Fatal("chart palette did not change")
	}
}
