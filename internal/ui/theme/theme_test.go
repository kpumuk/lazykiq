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
