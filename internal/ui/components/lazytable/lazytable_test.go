package lazytable

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
	opts := make([]table.Option, 0, 2+len(tableOpts))
	opts = append(opts,
		table.WithColumns([]table.Column{
			{Title: "A", Width: 3},
			{Title: "B", Width: 3},
		}),
		table.WithStyles(blankTableStyles()),
	)
	opts = append(opts, tableOpts...)
	return New(
		WithTableOptions(opts...),
		WithFetcher(noopFetcher),
	)
}

func fetchBatchCmd(t *testing.T, cmd tea.Cmd) tea.Cmd {
	t.Helper()

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("RequestWindow() returned %T, want tea.BatchMsg with fetch cmd", msg)
	}
	return batch[1]
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

func TestLazyTableRequestWindowCancelsSupersededFetch(t *testing.T) {
	started := make(chan context.Context, 2)

	m := New(
		WithTableOptions(
			table.WithColumns([]table.Column{{Title: "A", Width: 3}}),
			table.WithStyles(blankTableStyles()),
		),
		WithFetcher(func(ctx context.Context, _, _ int, _ CursorIntent) (FetchResult, error) {
			started <- ctx
			<-ctx.Done()
			return FetchResult{}, ctx.Err()
		}),
	)

	cmd1 := fetchBatchCmd(t, m.RequestWindow(0, CursorStart))
	done1 := make(chan any, 1)
	go func() {
		done1 <- cmd1()
	}()

	ctx1 := <-started
	select {
	case <-ctx1.Done():
		t.Fatal("first request canceled before replacement")
	default:
	}

	cmd2 := fetchBatchCmd(t, m.RequestWindow(25, CursorStart))
	select {
	case <-ctx1.Done():
	case <-time.After(time.Second):
		t.Fatal("first request was not canceled when a new window was requested")
	}

	done2 := make(chan any, 1)
	go func() {
		done2 <- cmd2()
	}()

	ctx2 := <-started
	select {
	case <-ctx2.Done():
		t.Fatal("replacement request canceled unexpectedly")
	default:
	}

	select {
	case msg := <-done1:
		if msg != nil {
			t.Fatalf("superseded request returned %T, want nil", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("superseded request did not finish after cancellation")
	}

	m.CancelRequest()
	if m.Loading() {
		t.Fatal("CancelRequest should clear loading state")
	}
	select {
	case <-ctx2.Done():
	case <-time.After(time.Second):
		t.Fatal("CancelRequest did not cancel the active fetch")
	}

	select {
	case msg := <-done2:
		if msg != nil {
			t.Fatalf("canceled request returned %T, want nil", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("active request did not finish after cancellation")
	}
}

func TestLazyTableFailedFetchCanRetry(t *testing.T) {
	wantErr := errors.New("scan failed")
	m := newTestModel()
	m.SetFetcher(func(context.Context, int, int, CursorIntent) (FetchResult, error) {
		return FetchResult{}, wantErr
	})
	wantID := 0
	m.SetErrorHandler(func(requestID int, err error) tea.Msg {
		if requestID != wantID {
			t.Errorf("error request ID = %d, want %d", requestID, wantID)
		}
		return err
	})
	msg := fetchBatchCmd(t, m.RequestWindow(0, CursorStart))()
	wantID = m.RequestID()
	var cmd tea.Cmd
	m, cmd = m.Update(msg)
	if m.Loading() || m.LoadingIndicator() != "" {
		t.Fatal("failed fetch left the loading indicator active")
	}
	if cmd == nil || !errors.Is(cmd().(error), wantErr) {
		t.Fatal("fetch error was not forwarded")
	}
	if cmd := m.RequestWindow(0, CursorStart); cmd == nil || !m.Loading() {
		t.Fatal("failed fetch could not be retried")
	}
	m.CancelRequest()
}

func TestLazyTableRejectsObsoleteResults(t *testing.T) {
	cases := map[string]func(*Model){
		"cancel": func(m *Model) { m.CancelRequest() },
		"reset and reload": func(m *Model) {
			m.Reset()
			m.RequestWindow(0, CursorStart)
		},
		"another table": func(m *Model) {
			m.CancelRequest()
			*m = newTestModel()
			m.RequestWindow(0, CursorStart)
		},
	}
	for name, invalidate := range cases {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			m.RequestWindow(0, CursorStart)
			oldID := m.RequestID()
			invalidate(&m)
			defer m.CancelRequest()
			loading := m.Loading()
			m, cmd := m.Update(DataMsg{
				RequestID: oldID,
				Result:    FetchResult{Rows: []table.Row{tableRow("stale", "stale")}, Total: 1},
				Err:       errors.New("stale error"),
			})
			if cmd != nil || m.Loading() != loading || len(m.Table().Rows()) != 0 {
				t.Fatal("obsolete result changed the current request")
			}
		})
	}
}

func TestLazyTableFetcherSnapshotsInputs(t *testing.T) {
	m := newTestModel()
	query := "original"
	m.SetFetcherFactory(func() Fetcher {
		snapshot := query
		return func(context.Context, int, int, CursorIntent) (FetchResult, error) {
			return FetchResult{Payload: snapshot}, nil
		}
	})
	cmd := fetchBatchCmd(t, m.RequestWindow(0, CursorStart))
	query = "changed"
	msg := cmd().(DataMsg)
	if msg.Result.Payload != "original" {
		t.Fatalf("request used %q, want original inputs", msg.Result.Payload)
	}
	m.CancelRequest()
}
