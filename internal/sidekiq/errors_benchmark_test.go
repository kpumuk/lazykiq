package sidekiq

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkErrorSummaryGrouping(b *testing.B) {
	for _, field := range []string{"args", "context", "error_backtrace"} {
		for _, width := range []int{32, 4096, 65536} {
			b.Run(fmt.Sprintf("%s/bytes=%d", field, width), func(b *testing.B) {
				payloads := make([]string, 1000)
				for i := range payloads {
					payloads[i] = fmt.Sprintf(`{"jid":"%024d","class":"AuditWorker","queue":"default","%s":["%s"],"error_class":"AuditError","error_message":"boom"}`, i, field, strings.Repeat("x", width))
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					rows := make(map[ErrorGroupKey]*errorSummaryState)
					for i, payload := range payloads {
						addErrorSummaryEntry(rows, NewSortedEntry(payload, float64(i)), "retry")
					}
					if len(rows) != 1 {
						b.Fatalf("got %d groups, want 1", len(rows))
					}
				}
			})
		}
	}
}
