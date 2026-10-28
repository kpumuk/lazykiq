package display

import (
	"fmt"
	"strings"
	"testing"
)

var argsPreviewSink string

func BenchmarkArgsPreview(b *testing.B) {
	for _, size := range []int{128, 8192, 65536, 1048576} {
		for _, text := range []struct{ name, unit string }{
			{"ascii", "x"}, {"unicode", "界"}, {"escaped", "\n"}, {"emoji", "👩🏽‍💻"},
		} {
			b.Run(fmt.Sprintf("%s/bytes=%d", text.name, size), func(b *testing.B) {
				args := []any{strings.Repeat(text.unit, size/len(text.unit))}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					argsPreviewSink = ArgsPreview(args)
				}
			})
		}
	}
}

func BenchmarkArgsPreviewNested(b *testing.B) {
	for _, count := range []int{32, 512, 8192} {
		values := make([]any, count)
		object := make(map[string]any, count)
		for i := range values {
			values[i] = "value"
			object[fmt.Sprintf("key%06d", i)] = "value"
		}
		for name, value := range map[string]any{"array": values, "map": object} {
			b.Run(fmt.Sprintf("%s/nodes=%d", name, count), func(b *testing.B) {
				args := []any{value}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					argsPreviewSink = ArgsPreview(args)
				}
			})
		}
	}
	for _, depth := range []int{32, 512, 2048} {
		var value any = "value"
		for range depth {
			value = []any{value}
		}
		b.Run(fmt.Sprintf("deep/depth=%d", depth), func(b *testing.B) {
			args := []any{value}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				argsPreviewSink = ArgsPreview(args)
			}
		})
	}
}

func BenchmarkArgsPreviewNestedPayload(b *testing.B) {
	for _, size := range []int{128, 65536, 1048576} {
		text := strings.Repeat("x", size)
		var deep any = text
		for range 512 {
			deep = []any{deep}
		}
		for name, value := range map[string]any{
			"map":   map[string]any{"a": text, "z": "tail"},
			"array": []any{text, "tail"},
			"deep":  deep,
		} {
			b.Run(fmt.Sprintf("%s/bytes=%d", name, size), func(b *testing.B) {
				args := []any{value}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					argsPreviewSink = ArgsPreview(args)
				}
			})
		}
	}
}
