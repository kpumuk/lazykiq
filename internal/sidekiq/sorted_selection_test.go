package sidekiq

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestSortedEntrySelectionMatchesStableSort(t *testing.T) {
	for name, score := range map[string]func(int, int) float64{
		"ascending":  func(i, _ int) float64 { return float64(i) },
		"descending": func(i, n int) float64 { return float64(n - i) },
		"ties":       func(_, _ int) float64 { return 1 },
		"random": func(i, n int) float64 {
			return float64(rand.New(rand.NewPCG(uint64(i), uint64(n))).IntN(23) - 11) //nolint:gosec // Fixed seeds make selection fixtures reproducible.
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, n := range []int{0, 1, 2, 7, 101, 401} {
				input := make([]*SortedEntry, n)
				for i := range input {
					input[i] = NewSortedEntry(fmt.Sprintf(`{"jid":"%d"}`, i), score(i, n))
				}
				for _, reverse := range []bool{false, true} {
					want := selectionBaseline(input, reverse, true)
					for _, page := range []struct{ start, count int }{
						{0, 0}, {0, 1}, {0, 7}, {n / 2, 7}, {max(n-7, 0), 7}, {n + 1, 7}, {0, n + 1},
					} {
						selected := newSortedEntrySelection(page.start+page.count, reverse)
						for _, entry := range input {
							selected.add(entry)
							if len(selected.entries) > selected.limit || cap(selected.entries) > selected.limit {
								t.Fatalf("retained %d entries, capacity %d, limit %d", len(selected.entries), cap(selected.entries), selected.limit)
							}
							if selected.heap {
								for i := 1; i < len(selected.entries); i++ {
									if selected.before(selected.entries[(i-1)/2], selected.entries[i]) {
										t.Fatalf("heap root is not the worst entry at %d", i)
									}
								}
							}
						}
						start := min(page.start, len(want))
						end := min(start+page.count, len(want))
						if got := selected.window(page.start, page.count); !slices.Equal(got, want[start:end]) {
							t.Fatalf("n=%d reverse=%v page=%+v: selection differs from stable sort", n, reverse, page)
						}
					}
				}
			}
		})
	}
}

func TestScanSortedWindowSelection(t *testing.T) {
	for name, n := range map[string]int{"dense": 401, "sparse": 7, "empty": 0} {
		t.Run(name, func(t *testing.T) {
			input := selectionTestEntries(n)
			client := selectionScanClient(t, map[string][]*SortedEntry{
				deadSetKey: input, retrySetKey: input, scheduleSetKey: input,
			})
			for _, kind := range []SortedSetKind{SortedSetDead, SortedSetRetry, SortedSetScheduled} {
				for _, page := range []struct{ start, count int }{
					{-10, 7}, {0, 7}, {n / 2, 7}, {max(n-7, 0), 7}, {n + 50, 7}, {2, 0}, {2, -1}, {math.MaxInt, 7}, {2, math.MaxInt},
				} {
					window, err := client.ScanSortedEntriesWindow(t.Context(), kind, "needle", page.start, page.count)
					if err != nil {
						t.Fatal(err)
					}
					want := selectionBaseline(input, kind == SortedSetDead, page.count > 0)
					start := min(max(page.start, 0), len(want))
					end := len(want)
					if page.count > 0 {
						end = start + min(page.count, end-start)
					}
					if window.Total != int64(n) || !equalSelectionEntries(window.Entries, want[start:end]) {
						t.Fatalf("kind=%s page=%+v: total=%d entries=%v; want total=%d entries=%v", kind, page, window.Total, window.Entries, n, want[start:end])
					}
					var first, last *SortedEntry
					for _, entry := range input {
						if first == nil || (kind == SortedSetDead && entry.Score > first.Score) || (kind != SortedSetDead && entry.Score < first.Score) {
							first = entry
						}
						if last == nil || (kind == SortedSetDead && entry.Score < last.Score) || (kind != SortedSetDead && entry.Score > last.Score) {
							last = entry
						}
					}
					if !equalSelectionEntries([]*SortedEntry{window.FirstEntry, window.LastEntry}, []*SortedEntry{first, last}) {
						t.Fatalf("kind=%s: bounds differ from scanned extremes", kind)
					}
				}
			}
		})
	}
}

func TestSortedSelectionScanCancellationAndErrors(t *testing.T) {
	wantErr := errors.New("scan failed")
	for name, scan := range map[string]func(context.Context, *Client) error{
		"sorted": func(ctx context.Context, client *Client) error {
			_, err := client.ScanSortedEntriesWindow(ctx, SortedSetRetry, "", 10, 10)
			return err
		},
		"error group": func(ctx context.Context, client *Client) error {
			_, err := client.GetErrorGroupWindow(ctx, ErrorGroupKey{}, "", 10, 10)
			return err
		},
	} {
		for _, cancelReply := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cancel=%v", name, cancelReply), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				calls := 0
				client := newSelectionHookClient(t, func(_ context.Context, cmd *redis.ScanCmd) error {
					calls++
					if calls == 2 {
						if !cancelReply {
							return wantErr
						}
						cancel()
					}
					cmd.SetVal([]string{`{"jid":"a"}`, "1"}, 1)
					return nil
				})
				want := wantErr
				if cancelReply {
					want = context.Canceled
				}
				if err := scan(ctx, client); !errors.Is(err, want) || calls != 2 {
					t.Fatalf("error=%v calls=%d; want %v after 2 calls", err, calls, want)
				}
			})
		}
	}
}

func selectionTestEntries(n int) []*SortedEntry {
	rng := rand.New(rand.NewPCG(11, 73)) //nolint:gosec // Fixed seeds make selection fixtures reproducible.
	entries := make([]*SortedEntry, n)
	for i := range entries {
		payload := fmt.Sprintf(`{"jid":"%d","marker":"needle"}`, i)
		if i%11 == 0 {
			payload = fmt.Sprintf(`{"marker":"needle","broken":%d,`, i)
		}
		entries[i] = NewSortedEntry(payload, float64(rng.IntN(23)-11))
	}
	return entries
}

func selectionBaseline(input []*SortedEntry, reverse, stable bool) []*SortedEntry {
	entries := slices.Clone(input)
	before := func(i, j int) bool {
		if reverse {
			return entries[i].Score > entries[j].Score
		}
		return entries[i].Score < entries[j].Score
	}
	if stable {
		sort.SliceStable(entries, before)
	} else {
		sort.Slice(entries, before)
	}
	return entries
}

func equalSelectionEntries(a, b []*SortedEntry) bool {
	return slices.EqualFunc(a, b, func(a, b *SortedEntry) bool {
		if a == nil || b == nil {
			return a == b
		}
		return a.Score == b.Score && a.Value() == b.Value()
	})
}

func selectionScanClient(t *testing.T, sets map[string][]*SortedEntry) *Client {
	t.Helper()
	return newSelectionHookClient(t, func(_ context.Context, cmd *redis.ScanCmd) error {
		key := cmd.Args()[1].(string)
		cursor, err := strconv.Atoi(fmt.Sprint(cmd.Args()[2]))
		if err != nil {
			return err
		}
		entries := sets[key]
		end := min(cursor+37, len(entries))
		values := make([]string, 0, 3+2*(end-cursor))
		values = append(values, "invalid-score", "bad")
		for _, entry := range entries[cursor:end] {
			values = append(values, entry.Value(), strconv.FormatFloat(entry.Score, 'g', -1, 64))
		}
		values = append(values, "incomplete-pair")
		next := uint64(end) //nolint:gosec // end is a nonnegative index into the fixture.
		if end == len(entries) {
			next = 0
		}
		cmd.SetVal(values, next)
		return nil
	})
}

type selectionScanHook func(context.Context, *redis.ScanCmd) error

func (h selectionScanHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h selectionScanHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		return h(ctx, cmd.(*redis.ScanCmd))
	}
}

func (h selectionScanHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func newSelectionHookClient(t *testing.T, process selectionScanHook) *Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{})
	t.Cleanup(func() { _ = rdb.Close() })
	rdb.AddHook(process)
	return &Client{redis: rdb}
}
