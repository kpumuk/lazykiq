package views

import (
	"context"
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

type fetchCall struct {
	start int
	size  int
}

type fetchCalls struct {
	scan       int
	scanWindow []fetchCall
	fetch      []fetchCall
	bounds     int
}

type fakeSortedEntriesClient struct {
	getSortedEntries     func(context.Context, sidekiq.SortedSetKind, int, int) ([]*sidekiq.SortedEntry, int64, error)
	scanSortedEntries    func(context.Context, sidekiq.SortedSetKind, string) ([]*sidekiq.SortedEntry, error)
	getSortedEntryBounds func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error)
}

func (c fakeSortedEntriesClient) GetSortedEntries(
	ctx context.Context,
	kind sidekiq.SortedSetKind,
	start, size int,
) ([]*sidekiq.SortedEntry, int64, error) {
	if c.getSortedEntries == nil {
		panic("unexpected GetSortedEntries call")
	}
	return c.getSortedEntries(ctx, kind, start, size)
}

func (c fakeSortedEntriesClient) ScanSortedEntries(
	ctx context.Context,
	kind sidekiq.SortedSetKind,
	query string,
) ([]*sidekiq.SortedEntry, error) {
	if c.scanSortedEntries == nil {
		panic("unexpected ScanSortedEntries call")
	}
	return c.scanSortedEntries(ctx, kind, query)
}

func (c fakeSortedEntriesClient) GetSortedEntryBounds(
	ctx context.Context,
	kind sidekiq.SortedSetKind,
) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
	if c.getSortedEntryBounds == nil {
		panic("unexpected GetSortedEntryBounds call")
	}
	return c.getSortedEntryBounds(ctx, kind)
}

type fakeSortedEntriesWindowClient struct {
	fakeSortedEntriesClient
	scanSortedEntriesWindow func(context.Context, sidekiq.SortedSetKind, string, int, int) (sidekiq.SortedEntriesWindow, error)
}

func (c fakeSortedEntriesWindowClient) ScanSortedEntriesWindow(
	ctx context.Context,
	kind sidekiq.SortedSetKind,
	query string,
	start, size int,
) (sidekiq.SortedEntriesWindow, error) {
	if c.scanSortedEntriesWindow == nil {
		panic("unexpected ScanSortedEntriesWindow call")
	}
	return c.scanSortedEntriesWindow(ctx, kind, query, start, size)
}

func TestFetchSortedWindow(t *testing.T) {
	cases := map[string]struct {
		setup  func(t *testing.T) (sortedWindowConfig, *fetchCalls)
		assert func(t *testing.T, got sortedWindowResult, calls *fetchCalls)
	}{
		"FilterUsesScanWindow": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					client: fakeSortedEntriesWindowClient{
						fakeSortedEntriesClient: fakeSortedEntriesClient{
							scanSortedEntries: func(_ context.Context, _ sidekiq.SortedSetKind, _ string) ([]*sidekiq.SortedEntry, error) {
								calls.scan++
								return nil, nil
							},
							getSortedEntries: func(context.Context, sidekiq.SortedSetKind, int, int) ([]*sidekiq.SortedEntry, int64, error) {
								t.Fatalf("unexpected fetch call")
								return nil, 0, nil
							},
							getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
								t.Fatalf("unexpected bounds call")
								return nil, nil, nil
							},
						},
						scanSortedEntriesWindow: func(_ context.Context, _ sidekiq.SortedSetKind, _ string, start, size int) (sidekiq.SortedEntriesWindow, error) {
							calls.scanWindow = append(calls.scanWindow, fetchCall{start: start, size: size})
							return sidekiq.SortedEntriesWindow{
								Entries:    []*sidekiq.SortedEntry{{Score: 8}, {Score: 7}, {Score: 6}},
								Total:      9,
								FirstEntry: &sidekiq.SortedEntry{Score: 10},
								LastEntry:  &sidekiq.SortedEntry{Score: 1},
							}, nil
						},
					},
					kind:        sidekiq.SortedSetDead,
					filter:      "boom",
					windowStart: 2,
					windowSize:  3,
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got sortedWindowResult, calls *fetchCalls) {
				if len(calls.scanWindow) != 1 {
					t.Fatalf("expected scanWindow to be called once, got %d", len(calls.scanWindow))
				}
				if calls.scanWindow[0] != (fetchCall{start: 2, size: 3}) {
					t.Fatalf("unexpected scanWindow args: %+v", calls.scanWindow[0])
				}
				if calls.scan != 0 {
					t.Fatalf("expected scan fallback not to be called, got %d", calls.scan)
				}
				if got.total != 9 {
					t.Fatalf("expected total 9, got %d", got.total)
				}
				if got.windowStart != 2 {
					t.Fatalf("expected windowStart 2, got %d", got.windowStart)
				}
				if got.firstEntry == nil || got.firstEntry.Score != 10 {
					t.Fatalf("expected firstEntry score 10, got %#v", got.firstEntry)
				}
				if got.lastEntry == nil || got.lastEntry.Score != 1 {
					t.Fatalf("expected lastEntry score 1, got %#v", got.lastEntry)
				}
				if len(got.jobs) != 3 {
					t.Fatalf("expected 3 jobs in window, got %d", len(got.jobs))
				}
			},
		},
		"FilterFallbackScanSlicesWindow": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					client: fakeSortedEntriesClient{
						scanSortedEntries: func(_ context.Context, _ sidekiq.SortedSetKind, _ string) ([]*sidekiq.SortedEntry, error) {
							calls.scan++
							return []*sidekiq.SortedEntry{{Score: 9}, {Score: 7}, {Score: 3}, {Score: 1}}, nil
						},
						getSortedEntries: func(context.Context, sidekiq.SortedSetKind, int, int) ([]*sidekiq.SortedEntry, int64, error) {
							t.Fatalf("unexpected fetch call")
							return nil, 0, nil
						},
						getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
							t.Fatalf("unexpected bounds call")
							return nil, nil, nil
						},
					},
					kind:        sidekiq.SortedSetDead,
					filter:      "boom",
					windowStart: 1,
					windowSize:  2,
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got sortedWindowResult, calls *fetchCalls) {
				if calls.scan != 1 {
					t.Fatalf("expected scan to be called once, got %d", calls.scan)
				}
				if got.total != 4 {
					t.Fatalf("expected total 4, got %d", got.total)
				}
				if got.windowStart != 1 {
					t.Fatalf("expected windowStart 1, got %d", got.windowStart)
				}
				if len(got.jobs) != 2 {
					t.Fatalf("expected 2 jobs in window, got %d", len(got.jobs))
				}
				if got.jobs[0].Score != 7 || got.jobs[1].Score != 3 {
					t.Fatalf("unexpected window scores: %#v", got.jobs)
				}
			},
		},
		"FilterClampsWindowStart": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					client: fakeSortedEntriesWindowClient{
						fakeSortedEntriesClient: fakeSortedEntriesClient{
							scanSortedEntries: func(context.Context, sidekiq.SortedSetKind, string) ([]*sidekiq.SortedEntry, error) {
								t.Fatalf("unexpected scan call")
								return nil, nil
							},
							getSortedEntries: func(context.Context, sidekiq.SortedSetKind, int, int) ([]*sidekiq.SortedEntry, int64, error) {
								t.Fatalf("unexpected fetch call")
								return nil, 0, nil
							},
							getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
								t.Fatalf("unexpected bounds call")
								return nil, nil, nil
							},
						},
						scanSortedEntriesWindow: func(_ context.Context, _ sidekiq.SortedSetKind, _ string, start, size int) (sidekiq.SortedEntriesWindow, error) {
							calls.scanWindow = append(calls.scanWindow, fetchCall{start: start, size: size})
							return sidekiq.SortedEntriesWindow{
								Entries:    []*sidekiq.SortedEntry{{Score: float64(start)}},
								Total:      5,
								FirstEntry: &sidekiq.SortedEntry{Score: 5},
								LastEntry:  &sidekiq.SortedEntry{Score: 1},
							}, nil
						},
					},
					kind:        sidekiq.SortedSetDead,
					filter:      "boom",
					windowStart: 9,
					windowSize:  3,
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got sortedWindowResult, calls *fetchCalls) {
				if len(calls.scanWindow) != 2 {
					t.Fatalf("expected scanWindow to be called twice, got %d", len(calls.scanWindow))
				}
				if calls.scanWindow[1].start != 2 {
					t.Fatalf("expected clamped refetch start 2, got %d", calls.scanWindow[1].start)
				}
				if got.windowStart != 2 {
					t.Fatalf("expected windowStart 2, got %d", got.windowStart)
				}
			},
		},
		"DefaultsWindowSize": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					client: fakeSortedEntriesClient{
						scanSortedEntries: func(context.Context, sidekiq.SortedSetKind, string) ([]*sidekiq.SortedEntry, error) {
							t.Fatalf("unexpected scan call")
							return nil, nil
						},
						getSortedEntries: func(_ context.Context, _ sidekiq.SortedSetKind, start, size int) ([]*sidekiq.SortedEntry, int64, error) {
							calls.fetch = append(calls.fetch, fetchCall{start: start, size: size})
							return []*sidekiq.SortedEntry{{Score: 2}}, 2, nil
						},
						getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
							calls.bounds++
							return &sidekiq.SortedEntry{Score: 1}, &sidekiq.SortedEntry{Score: 2}, nil
						},
					},
					kind:             sidekiq.SortedSetDead,
					windowStart:      0,
					windowSize:       0,
					fallbackPageSize: 5,
					windowPages:      2,
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got sortedWindowResult, calls *fetchCalls) {
				if len(calls.fetch) != 1 {
					t.Fatalf("expected 1 fetch call, got %d", len(calls.fetch))
				}
				if calls.fetch[0].size != 10 {
					t.Fatalf("expected default window size 10, got %d", calls.fetch[0].size)
				}
				if calls.bounds != 1 {
					t.Fatalf("expected bounds to be called once, got %d", calls.bounds)
				}
				if got.windowStart != 0 {
					t.Fatalf("expected windowStart 0, got %d", got.windowStart)
				}
			},
		},
		"ClampWindowStart": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					client: fakeSortedEntriesClient{
						scanSortedEntries: func(context.Context, sidekiq.SortedSetKind, string) ([]*sidekiq.SortedEntry, error) {
							t.Fatalf("unexpected scan call")
							return nil, nil
						},
						getSortedEntries: func(_ context.Context, _ sidekiq.SortedSetKind, start, size int) ([]*sidekiq.SortedEntry, int64, error) {
							calls.fetch = append(calls.fetch, fetchCall{start: start, size: size})
							return []*sidekiq.SortedEntry{{Score: float64(start)}}, 10, nil
						},
						getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
							calls.bounds++
							return &sidekiq.SortedEntry{Score: 1}, &sidekiq.SortedEntry{Score: 10}, nil
						},
					},
					kind:        sidekiq.SortedSetDead,
					windowStart: 9,
					windowSize:  3,
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got sortedWindowResult, calls *fetchCalls) {
				if len(calls.fetch) != 2 {
					t.Fatalf("expected 2 fetch calls, got %d", len(calls.fetch))
				}
				if calls.fetch[1].start != 7 {
					t.Fatalf("expected refetch start 7, got %d", calls.fetch[1].start)
				}
				if got.windowStart != 7 {
					t.Fatalf("expected windowStart 7, got %d", got.windowStart)
				}
			},
		},
		"ZeroTotalSkipsBounds": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					client: fakeSortedEntriesClient{
						scanSortedEntries: func(context.Context, sidekiq.SortedSetKind, string) ([]*sidekiq.SortedEntry, error) {
							t.Fatalf("unexpected scan call")
							return nil, nil
						},
						getSortedEntries: func(_ context.Context, _ sidekiq.SortedSetKind, start, size int) ([]*sidekiq.SortedEntry, int64, error) {
							calls.fetch = append(calls.fetch, fetchCall{start: start, size: size})
							return nil, 0, nil
						},
						getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
							calls.bounds++
							return nil, nil, nil
						},
					},
					kind:        sidekiq.SortedSetDead,
					windowStart: 4,
					windowSize:  2,
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got sortedWindowResult, calls *fetchCalls) {
				if calls.bounds != 0 {
					t.Fatalf("expected bounds not to be called, got %d", calls.bounds)
				}
				if got.windowStart != 0 {
					t.Fatalf("expected windowStart 0, got %d", got.windowStart)
				}
				if got.total != 0 {
					t.Fatalf("expected total 0, got %d", got.total)
				}
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, calls := tc.setup(t)
			got, err := fetchSortedWindow(context.Background(), cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.assert(t, got, calls)
		})
	}
}

type sortedEntriesCalls struct {
	scan       int
	scanWindow []fetchCall
	fetch      []fetchCall
	bounds     int
	buildRows  int
	jobs       []*sidekiq.SortedEntry
}

func TestFetchSortedEntriesWindow(t *testing.T) {
	cases := map[string]struct {
		setup  func(t *testing.T) (sortedEntriesFetchConfig, *sortedEntriesCalls)
		assert func(t *testing.T, got lazytable.FetchResult, calls *sortedEntriesCalls)
	}{
		"FilterUsesScanWindowAndBuildRows": {
			setup: func(t *testing.T) (sortedEntriesFetchConfig, *sortedEntriesCalls) {
				calls := &sortedEntriesCalls{}
				cfg := sortedEntriesFetchConfig{
					client: fakeSortedEntriesWindowClient{
						fakeSortedEntriesClient: fakeSortedEntriesClient{
							scanSortedEntries: func(_ context.Context, _ sidekiq.SortedSetKind, _ string) ([]*sidekiq.SortedEntry, error) {
								calls.scan++
								return nil, nil
							},
							getSortedEntries: func(context.Context, sidekiq.SortedSetKind, int, int) ([]*sidekiq.SortedEntry, int64, error) {
								t.Fatalf("unexpected fetch call")
								return nil, 0, nil
							},
							getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
								t.Fatalf("unexpected bounds call")
								return nil, nil, nil
							},
						},
						scanSortedEntriesWindow: func(_ context.Context, _ sidekiq.SortedSetKind, _ string, start, size int) (sidekiq.SortedEntriesWindow, error) {
							calls.scanWindow = append(calls.scanWindow, fetchCall{start: start, size: size})
							return sidekiq.SortedEntriesWindow{
								Entries:    []*sidekiq.SortedEntry{{Score: 8}, {Score: 7}},
								Total:      5,
								FirstEntry: &sidekiq.SortedEntry{Score: 9},
								LastEntry:  &sidekiq.SortedEntry{Score: 1},
							}, nil
						},
					},
					kind:   sidekiq.SortedSetDead,
					filter: "boom",
					buildRows: func(jobs []*sidekiq.SortedEntry) []table.Row {
						calls.buildRows++
						calls.jobs = jobs
						return []table.Row{{ID: "a"}}
					},
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got lazytable.FetchResult, calls *sortedEntriesCalls) {
				if len(calls.scanWindow) != 1 {
					t.Fatalf("expected scanWindow to be called once, got %d", len(calls.scanWindow))
				}
				if calls.scan != 0 {
					t.Fatalf("expected scan fallback not to be called, got %d", calls.scan)
				}
				if calls.buildRows != 1 {
					t.Fatalf("expected buildRows to be called once, got %d", calls.buildRows)
				}
				if len(calls.jobs) != 2 {
					t.Fatalf("expected buildRows jobs size 2, got %d", len(calls.jobs))
				}
				if got.Total != 5 {
					t.Fatalf("expected total 5, got %d", got.Total)
				}
				if len(got.Rows) != 1 {
					t.Fatalf("expected 1 row, got %d", len(got.Rows))
				}
				payload, ok := got.Payload.(sortedEntriesPayload)
				if !ok {
					t.Fatalf("expected sortedEntriesPayload, got %T", got.Payload)
				}
				if payload.firstEntry == nil || payload.firstEntry.Score != 9 {
					t.Fatalf("expected firstEntry score 9, got %#v", payload.firstEntry)
				}
				if payload.lastEntry == nil || payload.lastEntry.Score != 1 {
					t.Fatalf("expected lastEntry score 1, got %#v", payload.lastEntry)
				}
			},
		},
		"FetchUsesBoundsAndRows": {
			setup: func(t *testing.T) (sortedEntriesFetchConfig, *sortedEntriesCalls) {
				calls := &sortedEntriesCalls{}
				cfg := sortedEntriesFetchConfig{
					client: fakeSortedEntriesClient{
						scanSortedEntries: func(context.Context, sidekiq.SortedSetKind, string) ([]*sidekiq.SortedEntry, error) {
							t.Fatalf("unexpected scan call")
							return nil, nil
						},
						getSortedEntries: func(_ context.Context, _ sidekiq.SortedSetKind, start, size int) ([]*sidekiq.SortedEntry, int64, error) {
							calls.fetch = append(calls.fetch, fetchCall{start: start, size: size})
							return []*sidekiq.SortedEntry{{Score: 2}}, 10, nil
						},
						getSortedEntryBounds: func(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
							calls.bounds++
							return &sidekiq.SortedEntry{Score: 1}, &sidekiq.SortedEntry{Score: 9}, nil
						},
					},
					kind:        sidekiq.SortedSetDead,
					windowStart: 5,
					windowSize:  3,
					buildRows: func(_ []*sidekiq.SortedEntry) []table.Row {
						calls.buildRows++
						return []table.Row{{ID: "row"}}
					},
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got lazytable.FetchResult, calls *sortedEntriesCalls) {
				if len(calls.fetch) != 1 {
					t.Fatalf("expected 1 fetch call, got %d", len(calls.fetch))
				}
				if calls.fetch[0].start != 5 {
					t.Fatalf("expected fetch start 5, got %d", calls.fetch[0].start)
				}
				if calls.bounds != 1 {
					t.Fatalf("expected bounds call once, got %d", calls.bounds)
				}
				payload, ok := got.Payload.(sortedEntriesPayload)
				if !ok {
					t.Fatalf("expected sortedEntriesPayload, got %T", got.Payload)
				}
				if payload.firstEntry == nil || payload.firstEntry.Score != 1 {
					t.Fatalf("expected firstEntry score 1, got %#v", payload.firstEntry)
				}
				if payload.lastEntry == nil || payload.lastEntry.Score != 9 {
					t.Fatalf("expected lastEntry score 9, got %#v", payload.lastEntry)
				}
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, calls := tc.setup(t)
			got, err := fetchSortedEntriesWindow(context.Background(), cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.assert(t, got, calls)
		})
	}
}
