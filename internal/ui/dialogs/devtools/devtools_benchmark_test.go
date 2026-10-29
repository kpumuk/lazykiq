package devtools

import (
	"fmt"
	"strings"
	"testing"
)

var benchmarkResult string

func BenchmarkRedisResult(b *testing.B) {
	for _, width := range []int{32, 65536, 262144, 1048576} {
		for _, form := range []string{"string", "bytes", "array", "nested"} {
			b.Run(fmt.Sprintf("width=%d/%s", width, form), func(b *testing.B) {
				s := strings.Repeat("界\n", (width+3)/4)
				var value any = s
				switch form {
				case "bytes":
					value = []byte(s)
				case "array":
					value = []string{s, s, s}
				case "nested":
					value = []any{[]any{s, []any{s}}, "tail"}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					benchmarkResult = formatRedisResult(value)
				}
			})
		}
	}
}
