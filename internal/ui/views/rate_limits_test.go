package views

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

type limiterViewClient struct {
	sidekiq.API
	items            []sidekiq.Limiter
	err              error
	requests         []context.Context
	filters          []string
	scans, refreshes int
}

func (c *limiterViewClient) DisplayLimiterRedis() string { return "Job Redis" }
func (c *limiterViewClient) ScanLimiters(ctx context.Context, _ sidekiq.LimiterCursor, filter string) (sidekiq.LimiterPage, error) {
	c.requests = append(c.requests, ctx)
	c.filters = append(c.filters, filter)
	c.scans++
	return sidekiq.LimiterPage{Items: c.items, More: true}, c.err
}

func (c *limiterViewClient) GetLimiter(ctx context.Context, key string) (*sidekiq.Limiter, error) {
	c.requests = append(c.requests, ctx)
	for _, item := range c.items {
		if item.Key == key {
			return &item, c.err
		}
	}
	return &sidekiq.Limiter{Key: key, Missing: true}, c.err
}

func (c *limiterViewClient) RefreshLimiters(ctx context.Context, _ []string) ([]sidekiq.Limiter, error) {
	c.requests = append(c.requests, ctx)
	c.refreshes++
	return c.items, c.err
}

func limiterViewFixture() []sidekiq.Limiter {
	values := func(input map[string]string) map[string]sidekiq.LimiterValue {
		result := make(map[string]sidekiq.LimiterValue)
		for key, raw := range input {
			number, _ := strconv.ParseFloat(raw, 64)
			result[key] = sidekiq.LimiterValue{Raw: raw, Number: number, Valid: true}
		}
		return result
	}
	now := time.Date(2026, 11, 3, 21, 30, 0, 0, time.UTC)
	items := []sidekiq.Limiter{
		{Key: "lmtr-c-payments", Name: "payments", Type: "Concurrent", Values: values(map[string]string{"size": "8", "used": "6", "free": "2", "held": "417", "held_ms": "12345", "immediate": "400", "waited": "17", "wait_ms": "1250", "overtime": "0", "reclaimed": "0"})},
		{Key: "lmtr-w-search", Name: "search", Type: "Window", Interval: "30", Values: values(map[string]string{"size": "10"})},
		{Key: "lmtr-b-stripe", Name: "stripe", Type: "Bucket", Interval: "minute", Values: values(map[string]string{"size": "100"}), HistoryZone: "UTC", History: []sidekiq.LimiterSample{
			{At: now, Value: sidekiq.LimiterValue{Raw: "0", Number: 0, Valid: true}}, {At: now.Add(-time.Minute), Value: sidekiq.LimiterValue{Raw: "123", Number: 123, Valid: true}}, {At: now.Add(-2 * time.Minute)},
		}},
		{Key: "lmtr-k-shopify", Name: "shopify", Type: "Leaky bucket", Interval: "20", Values: values(map[string]string{"size": "40", "drops": "17", "last": "1793741400", "hit": "250", "miss": "5", "waited": "1.25"})},
		{Key: "lmtr-p-graphql", Name: "graphql", Type: "Points", Interval: "20", Values: values(map[string]string{"size": "1000", "points": "850", "last": "1793741400", "hit": "30", "miss": "0"})},
	}
	for i := range items {
		items[i].ObservedAt = now
		items[i].TTL = 10 * time.Minute
	}
	return items
}

func newLimiterTestView(detail bool, items []sidekiq.Limiter) *RateLimits {
	client := &limiterViewClient{items: items}
	m := NewRateLimits(client)
	if detail {
		m = NewRateLimitDetail(client)
	}
	m.SetStyles(Styles{})
	m.SetSize(120, 24)
	return m
}

func TestRateLimitsRefreshPagingAndSelection(t *testing.T) {
	items := limiterViewFixture()
	m := newLimiterTestView(false, items)
	m.Update(m.Init()())
	m.table.SetCursor(2)
	client := m.client.(*limiterViewClient)
	client.items = append([]sidekiq.Limiter{items[4]}, items[:4]...)
	_, cmd := m.Update(RefreshMsg{})
	m.Update(cmd())
	if m.table.SelectedRow().ID != items[2].Key || client.scans != 1 || client.refreshes != 1 {
		t.Fatal("refresh lost key selection or restarted discovery")
	}
	_, open := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	intent := open().(ShowRateLimitDetailMsg)
	if intent.Key != items[2].Key || !intent.Current() {
		t.Fatal("wrong detail intent")
	}
	_, next := m.Update(periodicKey("]"))
	if m.pageIndex != 1 || next == nil || intent.Current() {
		t.Fatal("page did not cancel old navigation")
	}
	m.Update(next())
	m.table.SetCursor(1)
	_, back := m.Update(periodicKey("["))
	if m.pageIndex != 0 || m.table.SelectedRow().ID != items[2].Key || back == nil {
		t.Fatal("return page lost key selection")
	}
	m.Update(back())
	if m.table.SelectedRow().ID != items[2].Key {
		t.Fatal("page refresh lost selection")
	}
	_, reset := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if m.pageIndex != 0 || len(m.pages) != 1 || m.ready || reset == nil {
		t.Fatal("reset retained obsolete discovery")
	}
}

func TestRateLimitsRejectStaleRepliesAndErrors(t *testing.T) {
	client := &limiterViewClient{items: limiterViewFixture()}
	m := NewRateLimits(client)
	old := m.Init()()
	client.err = errors.New("old connection failure")
	_, oldErrorCmd := m.Update(periodicKey("r"))
	oldError := oldErrorCmd()
	client.err = nil
	client.items = client.items[1:2]
	_, current := m.Update(filterdialog.ActionMsg{Action: filterdialog.ActionApply, Query: "search"})
	m.Update(current())
	m.Update(old)
	m.Update(oldError)
	if m.err != nil || len(m.pages[0].page.Items) != 1 || m.pages[0].page.Items[0].Name != "search" || client.filters[len(client.filters)-1] != "search" {
		t.Fatal("stale reply changed filter state")
	}
	_, hiddenCmd := m.Update(periodicKey("r"))
	hidden := hiddenCmd()
	m.Dispose()
	m.Update(hidden)
	if m.ready || len(m.pages) != 0 {
		t.Fatal("hidden completion resurrected view")
	}
	other := NewRateLimits(client)
	m.Update(other.Init()())
	if m.ready {
		t.Fatal("another instance's reply changed view")
	}
}

func TestRateLimitsNavigationSurvivesRefresh(t *testing.T) {
	m := newLimiterTestView(false, limiterViewFixture())
	m.Update(m.Init()())
	_, open := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	intent := open().(ShowRateLimitDetailMsg)
	_, refresh := m.Update(RefreshMsg{})
	if !intent.Current() {
		t.Fatal("periodic refresh invalidated an accepted Enter action")
	}
	m.Update(refresh())
	if !intent.Current() {
		t.Fatal("refresh completion invalidated navigation")
	}
	m.CancelRequests()
	if intent.Current() {
		t.Fatal("navigation survived a lifetime transition")
	}
}

func TestRateLimitsRequestLifetimeAndErrorRecovery(t *testing.T) {
	client := &limiterViewClient{items: limiterViewFixture()}
	m := NewRateLimits(client)
	ctx, cancel := context.WithCancel(t.Context())
	m.SetContext(ctx)
	m.Update(m.Init()())
	if m.loading || !m.ready {
		t.Fatal("initial request not complete")
	}
	client.err = errors.New("limiter pool unavailable")
	_, cmd := m.Update(periodicKey("r"))
	m.Update(cmd())
	if m.err == nil || m.loading || len(m.pages[0].page.Items) != 5 {
		t.Fatal("failure lost usable data or loading state")
	}
	client.err = nil
	_, cmd = m.Update(periodicKey("r"))
	m.Update(cmd())
	if m.err != nil {
		t.Fatal("successful retry did not recover")
	}
	_, pending := m.Update(periodicKey("r"))
	cancel()
	if pending() != nil || m.readContext.Err() != context.Canceled {
		t.Fatal("root cancellation did not reach reader")
	}
}

func TestRateLimitDetailReplacementHistoryAndUnits(t *testing.T) {
	items := limiterViewFixture()
	m := newLimiterTestView(true, items)
	m.SetLimiterKey(items[0].Key)
	old := m.Init()()
	m.SetLimiterKey(items[2].Key)
	m.Update(m.Init()())
	m.Update(old)
	if m.limiter == nil || m.limiter.Type != "Bucket" || !m.hasHistory() {
		t.Fatal("detail replacement lost current type/history")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !m.focusHistory {
		t.Fatal("history focus unavailable")
	}
	m.SetSize(60, 20)
	if !strings.Contains(ansi.Strip(m.View()), "Bucket Counters") {
		t.Fatal("narrow history inaccessible")
	}
	for _, item := range items {
		m.SetLimiterKey(item.Key)
		m.Update(m.Init()())
		rows := limiterPropertyRows(m.limiter)
		all := fmt.Sprint(rows)
		switch item.Type {
		case "Concurrent":
			for _, label := range []string{"Used capacity", "Free capacity", "Held count", "12.345 s", "Immediate acquisitions", "Waited count", "1.25 s", "Overtime count", "Reclaimed locks"} {
				if !strings.Contains(all, label) {
					t.Fatalf("missing concurrent metric %q", label)
				}
			}
		case "Points":
			for _, label := range []string{"1000 points", "50 points/s", "Last points balance", "850 points", "Full refill interval", "Unavailable"} {
				if !strings.Contains(all, label) {
					t.Fatalf("wrong points units/state: %q", label)
				}
			}
		case "Leaky bucket":
			for _, label := range []string{"40 operations", "2 operations/s", "Last bucket fill", "17 operations", "Full drain interval", "1.25 s"} {
				if !strings.Contains(all, label) {
					t.Fatalf("wrong leaky units/state: %q", label)
				}
			}
		}
	}
	m.SetLimiterKey("lmtr-w-gone")
	m.Update(m.Init()())
	if !strings.Contains(ansi.Strip(m.View()), "disappeared") {
		t.Fatal("expiration not visible")
	}
}

func TestGoldenRateLimits(t *testing.T) {
	items := limiterViewFixture()
	for name, test := range map[string]struct {
		detail, loading, empty, history, bottom bool
		index, width, height                    int
		problem                                 string
	}{
		"overview": {width: 120, height: 20}, "overview_narrow": {width: 54, height: 12}, "overview_tiny": {width: 3, height: 2},
		"loading": {loading: true, width: 80, height: 20}, "empty": {empty: true, width: 80, height: 20},
		"concurrent": {detail: true, index: 0, width: 100, height: 24}, "concurrent_narrow": {detail: true, index: 0, width: 42, height: 12},
		"concurrent_bottom": {detail: true, index: 0, bottom: true, width: 100, height: 12},
		"window":            {detail: true, index: 1, width: 100, height: 20}, "bucket": {detail: true, index: 2, width: 140, height: 20},
		"bucket_narrow": {detail: true, index: 2, width: 65, height: 16}, "bucket_history_narrow": {detail: true, index: 2, history: true, width: 65, height: 16},
		"leaky": {detail: true, index: 3, width: 110, height: 24}, "points": {detail: true, index: 4, width: 110, height: 24},
		"malformed": {detail: true, index: 1, width: 100, height: 20, problem: "Size unavailable or invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			m := newLimiterTestView(test.detail, items)
			if test.detail {
				m.SetLimiterKey(items[test.index].Key)
			}
			if !test.loading {
				m.Update(m.Init()())
			}
			if test.empty {
				m.pages[0].page = sidekiq.LimiterPage{}
				m.buildRows()
			}
			if test.problem != "" {
				m.limiter.Problems = []string{test.problem}
				m.buildRows()
			}
			m.SetSize(test.width, test.height)
			m.focusHistory = test.history
			if test.bottom {
				m.table.GotoBottom()
			}
			golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
		})
	}
}

func TestRateLimitsLayoutAndScrollBounds(t *testing.T) {
	items := limiterViewFixture()
	for name, detail := range map[string]bool{"list": false, "detail": true} {
		t.Run(name, func(t *testing.T) {
			m := newLimiterTestView(detail, items)
			if detail {
				m.SetLimiterKey(items[2].Key)
			}
			m.Update(m.Init()())
			for _, size := range [][2]int{{140, 24}, {80, 12}, {24, 6}, {4, 3}, {3, 2}, {1, 1}, {0, 0}, {-1, -1}, {120, 20}} {
				m.SetSize(size[0], size[1])
				for range 50 {
					m.table.ScrollRight()
					m.table.MoveDown(1)
					m.history.ScrollRight()
					m.history.MoveDown(1)
				}
				for _, focus := range []bool{false, true} {
					m.focusHistory = focus
					view := m.View()
					if len(strings.Split(view, "\n")) > max(size[1], 1) {
						t.Fatalf("height overflow at %v", size)
					}
					for line := range strings.SplitSeq(view, "\n") {
						if ansi.StringWidth(line) > max(size[0], 0) {
							t.Fatalf("width overflow at %v: %q", size, line)
						}
					}
				}
			}
		})
	}
}

func TestRateLimitsHasNoMutations(t *testing.T) {
	m := newLimiterTestView(false, limiterViewFixture())
	m.Update(m.Init()())
	for _, k := range []string{"d", "D", "e", "p", "x", "u"} {
		if _, cmd := m.Update(periodicKey(k)); cmd != nil {
			t.Fatalf("unexpected mutation key %s", k)
		}
	}
	if m.ShortHelp() != nil {
		t.Fatal("new navbar shortcuts")
	}
}
