package sidekiq

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestErrorGroupSelectionPagingAndShrinkingTotals(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 57)) //nolint:gosec // Fixed seeds make selection fixtures reproducible.
	sets := make(map[string][]*SortedEntry)
	group := ErrorGroupKey{DisplayClass: "Worker", ErrorClass: "Error", Queue: "default"}
	for _, key := range []string{deadSetKey, retrySetKey} {
		for i := range 301 {
			class := "Worker"
			if i%5 == 0 {
				class = "OtherWorker"
			}
			payload := errorPayload(fmt.Sprintf("%s-%d", key, i), class, "default", "Error", "boom", "needle")
			sets[key] = append(sets[key], NewSortedEntry(payload, float64(rng.IntN(23))))
		}
	}
	client := selectionScanClient(t, sets)
	for _, size := range []int{301, 37, 0} {
		for key := range sets {
			sets[key] = sets[key][:size]
		}
		for name, page := range map[string]struct{ start, count int }{
			"first":        {0, 17},
			"middle":       {size / 2, 17},
			"across sets":  {max(size-size/5-8, 0), 17},
			"last":         {max(2*(size-size/5)-17, 0), 17},
			"stale start":  {999, 17},
			"all":          {2, -1},
			"count only":   {999, 0},
			"negative":     {-10, 17},
			"large window": {0, 999},
		} {
			t.Run(fmt.Sprintf("size=%d/%s", size, name), func(t *testing.T) {
				var want []ErrorGroupEntry
				for _, set := range []string{deadSetKey, retrySetKey} {
					var matched []*SortedEntry
					for i, entry := range sets[set] {
						if i%5 != 0 {
							matched = append(matched, entry)
						}
					}
					for _, entry := range selectionBaseline(matched, set == deadSetKey, page.count > 0) {
						want = append(want, ErrorGroupEntry{Entry: entry, Source: set})
					}
				}
				total := len(want)
				start := max(page.start, 0)
				end := total
				if page.count > 0 {
					start = min(start, max(total-page.count, 0))
					end = min(start+page.count, total)
				}
				if page.count == 0 {
					want = nil
				} else {
					want = want[min(start, total):end]
				}
				got, err := client.GetErrorGroupWindow(t.Context(), group, "needle", page.start, page.count)
				if err != nil {
					t.Fatal(err)
				}
				equal := slices.EqualFunc(got.Entries, want, func(a, b ErrorGroupEntry) bool {
					return a.Source == b.Source && a.Entry.Score == b.Entry.Score && a.Entry.Value() == b.Entry.Value()
				})
				if !equal || got.Total != int64(total) || got.WindowStart != start {
					t.Fatalf("window start=%d total=%d entries=%v; want start=%d total=%d entries=%v", got.WindowStart, got.Total, got.Entries, start, total, want)
				}
			})
		}
	}
}
