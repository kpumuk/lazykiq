package sidekiq

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

var benchmarkSortedSelectionSink any

func benchmarkSortedPayload(i, width int) string {
	return fmt.Sprintf(`{"jid":"%024d","class":"AuditWorker","queue":"default","args":["%s"],"enqueued_at":1780000000000,"error_class":"AuditError","error_message":"boom"}`, i, strings.Repeat("x", width))
}

func benchmarkSeedSorted(tb testing.TB, c *Client, n, width int) {
	tb.Helper()
	ctx := context.Background()
	for start := 0; start < n; start += 100 {
		items := make([]redis.Z, 0, min(n-start, 100))
		for i := start; i < min(start+100, n); i++ {
			items = append(items, redis.Z{Score: float64(i + 1), Member: benchmarkSortedPayload(i, width)})
		}
		if err := c.redis.ZAdd(ctx, "retry", items...).Err(); err != nil {
			tb.Fatal(err)
		}
	}
}

func BenchmarkSortedWindowScan(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		for _, deep := range []bool{false, true} {
			b.Run(fmt.Sprintf("n=%d/deep=%v", n, deep), func(b *testing.B) {
				c := benchmarkPrivateSortedRedis(b)
				benchmarkSeedSorted(b, c, n, 32)
				start := 0
				if deep {
					start = n - 100
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					w, err := c.ScanSortedEntriesWindow(context.Background(), SortedSetRetry, "AuditWorker", start, 100)
					if err != nil {
						b.Fatal(err)
					}
					if w.Total != int64(n) || len(w.Entries) != 100 || w.Entries[0].Score != float64(start+1) || w.Entries[99].Score != float64(start+100) {
						b.Fatalf("incorrect sorted window at %d: %#v", start, w)
					}
					benchmarkSortedSelectionSink = w
				}
			})
		}
	}
}

func BenchmarkErrorGroupWindowDepth(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			client := benchmarkPrivateSortedRedis(b)
			benchmarkSeedSorted(b, client, n, 32)
			ctx := b.Context()
			if err := client.redis.ZUnionStore(ctx, "dead", &redis.ZStore{Keys: []string{"retry"}}).Err(); err != nil {
				b.Fatal(err)
			}
			key := ErrorGroupKey{DisplayClass: "AuditWorker", ErrorClass: "AuditError", Queue: "default"}
			for name, start := range map[string]int{"first": 0, "across": n - 50, "last": 2*n - 100} {
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						window, err := client.GetErrorGroupWindow(ctx, key, "AuditWorker", start, 100)
						if err != nil {
							b.Fatal(err)
						}
						if window.Total != int64(2*n) || window.WindowStart != start || len(window.Entries) != 100 {
							b.Fatalf("incorrect group window at %d: %#v", start, window)
						}
						for i, entry := range window.Entries {
							index := start + i
							source, score := "dead", float64(n-index)
							if index >= n {
								source, score = "retry", float64(index-n+1)
							}
							if entry.Source != source || entry.Entry.Score != score {
								b.Fatalf("entry at %d: source=%s score=%g; want %s %g", index, entry.Source, entry.Entry.Score, source, score)
							}
						}
						benchmarkSortedSelectionSink = window
					}
				})
			}
		})
	}
}

func BenchmarkSortedEntrySelection(b *testing.B) {
	for _, n := range []int{1000, 10000, 30000} {
		for _, limit := range []int{100, n} {
			b.Run(fmt.Sprintf("n=%d/limit=%d", n, limit), func(b *testing.B) {
				entries := make([]*SortedEntry, n)
				for i := range entries {
					entries[i] = NewSortedEntry(`{"jid":"a"}`, float64(n-i))
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					selected := newSortedEntrySelection(limit, false)
					for _, entry := range entries {
						selected.add(entry)
					}
					benchmarkSortedSelectionSink = selected.window(0, limit)
				}
				b.ReportMetric(float64(n), "candidates/op")
				b.ReportMetric(float64(min(n, limit)), "max-retained/op")
			})
		}
	}
}
