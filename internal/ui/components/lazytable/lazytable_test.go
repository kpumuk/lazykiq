package lazytable

import (
	"context"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

func blankTableStyles() table.Styles {
	return table.Styles{
		Text:           lipgloss.NewStyle(),
		Muted:          lipgloss.NewStyle(),
		Header:         lipgloss.NewStyle(),
		Selected:       lipgloss.NewStyle(),
		Separator:      lipgloss.NewStyle(),
		ScrollbarTrack: lipgloss.NewStyle(),
		ScrollbarThumb: lipgloss.NewStyle(),
	}
}

func tableRow(id string, cells ...string) table.Row {
	return table.Row{ID: id, Cells: cells}
}

func noopFetcher(_ context.Context, _, _ int, _ CursorIntent) (FetchResult, error) {
	return FetchResult{}, nil
}

func newTestModel(tableOpts ...table.Option) Model {
	opts := []table.Option{
		table.WithColumns([]table.Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		table.WithStyles(blankTableStyles()),
	}
	opts = append(opts, tableOpts...)
	return New(
		WithTableOptions(opts...),
		WithFetcher(noopFetcher),
	)
}

func TestLazyTableAnchorKeepsSelection(t *testing.T) {
	m := newTestModel()

	m.SetSize(10, 5)
	m.RequestWindow(0, CursorStart)
	rows := []table.Row{
		tableRow("row-1", "one", "two"),
		tableRow("row-2", "three", "four"),
		tableRow("row-3", "five", "six"),
		tableRow("row-4", "seven", "eight"),
		tableRow("row-5", "nine", "ten"),
	}
	m, _ = m.Update(DataMsg{RequestID: m.RequestID(), Result: FetchResult{Rows: rows, Total: int64(len(rows)), WindowStart: 0}})

	m.Table().SetCursor(3)
	m.RequestWindow(2, CursorKeep)
	shifted := []table.Row{
		tableRow("row-3", "five", "six"),
		tableRow("row-4", "seven", "eight"),
		tableRow("row-5", "nine", "ten"),
	}
	m, _ = m.Update(DataMsg{RequestID: m.RequestID(), Result: FetchResult{Rows: shifted, Total: int64(len(rows)), WindowStart: 2}})

	if got := m.Table().Cursor(); got != 1 {
		t.Fatalf("expected cursor to stay on row-4 (index 1), got %d", got)
	}
}

func TestGoldenLazyTableLoaded(t *testing.T) {
	m := newTestModel()
	m.SetSize(10, 4)
	m.RequestWindow(0, CursorStart)
	rows := []table.Row{
		tableRow("row-1", "one", "two"),
		tableRow("row-2", "three", "four"),
		tableRow("row-3", "five", "six"),
	}
	m, _ = m.Update(DataMsg{RequestID: m.RequestID(), Result: FetchResult{Rows: rows, Total: 10, WindowStart: 0}})

	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenLazyTableLoading(t *testing.T) {
	m := newTestModel(table.WithEmptyMessage("Loading"))
	m.SetSize(10, 4)
	m.RequestWindow(0, CursorStart)

	output := ansi.Strip(m.View())
	golden.RequireEqual(t, []byte(output))
}
