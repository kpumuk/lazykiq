package views

import (
	"context"
	"testing"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type fetchCall struct {
	start int
	size  int
}

type fetchCalls struct {
	scan   int
	fetch  []fetchCall
	bounds int
}

func TestFetchSortedWindow(t *testing.T) {
	cases := map[string]struct {
		setup  func(t *testing.T) (sortedWindowConfig, *fetchCalls)
		assert func(t *testing.T, got sortedWindowResult, calls *fetchCalls)
	}{
		"FilterUsesScan": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					filter: "boom",
					scan: func(_ context.Context, _ string) ([]*sidekiq.SortedEntry, error) {
						calls.scan++
						return []*sidekiq.SortedEntry{{Score: 2}, {Score: 1}, {Score: 3}}, nil
					},
					fetch: func(context.Context, int, int) ([]*sidekiq.SortedEntry, int64, error) {
						t.Fatalf("unexpected fetch call")
						return nil, 0, nil
					},
					bounds: func(context.Context) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
						t.Fatalf("unexpected bounds call")
						return nil, nil, nil
					},
				}
				return cfg, calls
			},
			assert: func(t *testing.T, got sortedWindowResult, calls *fetchCalls) {
				if calls.scan != 1 {
					t.Fatalf("expected scan to be called once, got %d", calls.scan)
				}
				if got.total != 3 {
					t.Fatalf("expected total 3, got %d", got.total)
				}
				if got.windowStart != 0 {
					t.Fatalf("expected windowStart 0, got %d", got.windowStart)
				}
				if got.firstEntry == nil || got.firstEntry.Score != 1 {
					t.Fatalf("expected firstEntry score 1, got %#v", got.firstEntry)
				}
				if got.lastEntry == nil || got.lastEntry.Score != 3 {
					t.Fatalf("expected lastEntry score 3, got %#v", got.lastEntry)
				}
			},
		},
		"DefaultsWindowSize": {
			setup: func(t *testing.T) (sortedWindowConfig, *fetchCalls) {
				calls := &fetchCalls{}
				cfg := sortedWindowConfig{
					windowStart:      0,
					windowSize:       0,
					fallbackPageSize: 5,
					windowPages:      2,
					scan: func(context.Context, string) ([]*sidekiq.SortedEntry, error) {
						t.Fatalf("unexpected scan call")
						return nil, nil
					},
					fetch: func(_ context.Context, start, size int) ([]*sidekiq.SortedEntry, int64, error) {
						calls.fetch = append(calls.fetch, fetchCall{start: start, size: size})
						return []*sidekiq.SortedEntry{{Score: 2}}, 2, nil
					},
					bounds: func(context.Context) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
						calls.bounds++
						return &sidekiq.SortedEntry{Score: 1}, &sidekiq.SortedEntry{Score: 2}, nil
					},
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
					windowStart: 9,
					windowSize:  3,
					scan: func(context.Context, string) ([]*sidekiq.SortedEntry, error) {
						t.Fatalf("unexpected scan call")
						return nil, nil
					},
					fetch: func(_ context.Context, start, size int) ([]*sidekiq.SortedEntry, int64, error) {
						calls.fetch = append(calls.fetch, fetchCall{start: start, size: size})
						return []*sidekiq.SortedEntry{{Score: float64(start)}}, 10, nil
					},
					bounds: func(context.Context) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
						calls.bounds++
						return &sidekiq.SortedEntry{Score: 1}, &sidekiq.SortedEntry{Score: 10}, nil
					},
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
					windowStart: 4,
					windowSize:  2,
					scan: func(context.Context, string) ([]*sidekiq.SortedEntry, error) {
						t.Fatalf("unexpected scan call")
						return nil, nil
					},
					fetch: func(_ context.Context, start, size int) ([]*sidekiq.SortedEntry, int64, error) {
						calls.fetch = append(calls.fetch, fetchCall{start: start, size: size})
						return nil, 0, nil
					},
					bounds: func(context.Context) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error) {
						calls.bounds++
						return nil, nil, nil
					},
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
