package views

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestGoldenQueuesListEmpty(t *testing.T) {
	view := NewQueuesList(nil)
	view.SetSize(100, 12)
	view.SetStyles(Styles{})
	view.ready = true
	view.updateTableRows()

	output := ansi.Strip(view.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenQueuesListNoMatches(t *testing.T) {
	view := NewQueuesList(nil)
	view.SetSize(100, 12)
	view.SetStyles(Styles{})
	view.ready = true
	view.filter = "critical"
	view.updateTableRows()

	output := ansi.Strip(view.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenRetriesEmpty(t *testing.T) {
	view := NewRetries(nil)
	view.SetSize(120, 12)
	view.SetStyles(Styles{})
	view.ready = true
	view.updateEmptyMessage()

	output := ansi.Strip(view.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenRetriesNoMatches(t *testing.T) {
	view := NewRetries(nil)
	view.SetSize(120, 12)
	view.SetStyles(Styles{})
	view.ready = true
	view.filter = "critical"
	view.updateEmptyMessage()

	output := ansi.Strip(view.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenQueueDetailsNoQueues(t *testing.T) {
	view := NewQueueDetails(nil)
	view.SetSize(120, 12)
	view.SetStyles(Styles{})
	view.ready = true
	view.updateEmptyMessage()

	output := ansi.Strip(view.View())
	golden.RequireEqual(t, []byte(output))
}
