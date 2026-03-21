package views

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/contextbar"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

type errorsSummaryClientStub struct {
	sidekiq.API
	calls int
	rows  []sidekiq.ErrorSummaryRow
	meta  sidekiq.ErrorSummaryMeta
}

func (s *errorsSummaryClientStub) GetErrorSummary(
	context.Context,
	string,
) ([]sidekiq.ErrorSummaryRow, sidekiq.ErrorSummaryMeta, error) {
	s.calls++
	return append([]sidekiq.ErrorSummaryRow(nil), s.rows...), s.meta, nil
}

func TestErrorsSummaryRefreshMsgUsesTTL(t *testing.T) {
	prevNow := nowFuncErrorsSummary
	nowFuncErrorsSummary = func() time.Time {
		return time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() {
		nowFuncErrorsSummary = prevNow
	})

	client := &errorsSummaryClientStub{
		rows: []sidekiq.ErrorSummaryRow{
			{
				DisplayClass: "CleanupJob",
				ErrorClass:   "ArgumentError",
				Queue:        "default",
				Count:        3,
				ErrorMessage: "boom",
			},
		},
		meta: sidekiq.ErrorSummaryMeta{DeadCount: 2, RetryCount: 1},
	}

	view := NewErrorsSummary(client)
	view.SetSize(100, 12)
	view.SetStyles(Styles{})

	cmd := view.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("Init fetch returned nil msg")
	}
	updated, _ := view.Update(msg)
	summary := updated.(*ErrorsSummary)

	if client.calls != 1 {
		t.Fatalf("client.calls after init = %d, want 1", client.calls)
	}

	updated, cmd = summary.Update(RefreshMsg{})
	summary = updated.(*ErrorsSummary)
	if cmd != nil {
		t.Fatal("RefreshMsg before TTL returned a fetch cmd")
	}
	if client.calls != 1 {
		t.Fatalf("client.calls before TTL = %d, want 1", client.calls)
	}

	summary.fetchedAt = nowFuncErrorsSummary().Add(-61 * time.Second)
	updated, cmd = summary.Update(RefreshMsg{})
	summary = updated.(*ErrorsSummary)
	if cmd == nil {
		t.Fatal("RefreshMsg after TTL returned nil cmd")
	}

	msg = cmd()
	if msg == nil {
		t.Fatal("TTL refresh returned nil msg")
	}
	updated, _ = summary.Update(msg)
	summary = updated.(*ErrorsSummary)
	if client.calls != 2 {
		t.Fatalf("client.calls after TTL refresh = %d, want 2", client.calls)
	}

	updated, cmd = summary.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	_ = updated.(*ErrorsSummary)
	if cmd == nil {
		t.Fatal("manual refresh returned nil cmd")
	}
	_ = cmd()
	if client.calls != 3 {
		t.Fatalf("client.calls after manual refresh = %d, want 3", client.calls)
	}
}

func TestGoldenErrorsSummaryContext(t *testing.T) {
	prevNow := nowFuncErrorsSummary
	nowFuncErrorsSummary = func() time.Time {
		return time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() {
		nowFuncErrorsSummary = prevNow
	})

	view := NewErrorsSummary(nil)
	view.ready = true
	view.meta = sidekiq.ErrorSummaryMeta{DeadCount: 12, RetryCount: 7}
	view.fetchedAt = nowFuncErrorsSummary().Add(-37 * time.Second)
	view.filter = "CleanupJob"

	output := ansi.Strip(renderContextBar(view.ContextItems(), view.HintBindings()))
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenErrorsDetailsContext(t *testing.T) {
	view := NewErrorsDetails(nil)
	view.ready = true
	view.groupKey = sidekiq.ErrorGroupKey{
		DisplayClass: "CleanupJob",
		ErrorClass:   "ArgumentError",
		Queue:        "default",
	}
	view.filter = "cleanup"
	view.lazy, _ = view.lazy.Update(lazytable.DataMsg{
		RequestID: view.lazy.RequestID(),
		Result: lazytable.FetchResult{
			Total:       42,
			WindowStart: 0,
		},
	})

	output := ansi.Strip(renderContextBar(view.ContextItems(), view.HintBindings()))
	golden.RequireEqual(t, []byte(output))
}

func TestErrorsDetailsResetDataResetsArgumentWidth(t *testing.T) {
	view := NewErrorsDetails(nil)
	view.SetSize(280, 8)
	view.SetStyles(Styles{})

	view.lazy.Table().SetRows([]table.Row{
		{
			ID: "wide",
			Cells: []string{
				"retry",
				"1s",
				"default",
				"CleanupJob",
				strings.Repeat("x", 80),
				"boom",
			},
		},
	})
	wideLine := strings.Split(ansi.Strip(view.lazy.Table().View()), "\n")[2]
	wideIndex := strings.Index(wideLine, "boom")
	if wideIndex < 0 {
		t.Fatal("wide row missing error column")
	}

	view.resetData()
	view.lazy.Table().SetRows([]table.Row{
		{
			ID: "narrow",
			Cells: []string{
				"retry",
				"1s",
				"default",
				"CleanupJob",
				"",
				"boom",
			},
		},
	})
	narrowLine := strings.Split(ansi.Strip(view.lazy.Table().View()), "\n")[2]
	narrowIndex := strings.Index(narrowLine, "boom")
	if narrowIndex < 0 {
		t.Fatal("narrow row missing error column")
	}

	if narrowIndex >= wideIndex {
		t.Fatalf("error column did not shift back after reset: wide=%d narrow=%d", wideIndex, narrowIndex)
	}
}

func renderContextBar(items []ContextItem, bindings []key.Binding) string {
	barItems := make([]contextbar.Item, 0, len(items))
	for _, item := range items {
		barItems = append(barItems, contextbar.KeyValueItem{
			Label: item.Label,
			Value: item.Value,
		})
	}

	hints := make([]contextbar.Hint, 0, len(bindings))
	for _, binding := range bindings {
		hints = append(hints, contextbar.Hint{
			Binding: binding,
			Kind:    contextbar.HintNormal,
		})
	}

	bar := contextbar.New(
		contextbar.WithSize(100, 5),
		contextbar.WithItems(barItems),
		contextbar.WithHints(hints),
	)
	return bar.View()
}
