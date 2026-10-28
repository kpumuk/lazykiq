package jsonview

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var benchmarkJSONLine string

func BenchmarkJSONRenderWide(b *testing.B) {
	for _, size := range []int{1024, 8192, 65536, 1048576} {
		m := New()
		m.SetValue(map[string]any{"args": strings.Repeat("x", size)})
		for name, offset := range map[string]int{"start": 0, "middle": size / 2, "end": size - 64} {
			b.Run(fmt.Sprintf("bytes=%d/%s", size, name), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					benchmarkJSONLine = m.RenderLine(1, offset, 80)
				}
			})
		}
	}
}

func BenchmarkJSONRenderUnicode(b *testing.B) {
	for name, text := range map[string]string{
		"unicode": "界e\u0301👩🏽‍💻🇨🇦",
		"escaped": "\"\\\n\t\x1b[31m",
	} {
		for _, size := range []int{1024, 1048576} {
			m := New()
			m.SetValue(strings.Repeat(text, size/len(text)))
			columns := ansi.StringWidth(m.lines[0])
			for position, offset := range map[string]int{"start": 0, "middle": columns / 2, "end": columns - 64} {
				b.Run(fmt.Sprintf("%s/bytes=%d/%s", name, size, position), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						benchmarkJSONLine = m.RenderLine(0, offset, 80)
					}
				})
			}
		}
	}
}

func BenchmarkJSONSetValueWide(b *testing.B) {
	for name, unit := range map[string]string{"ascii": "x", "unicode": "界e\u0301👩🏽‍💻🇨🇦"} {
		for _, size := range []int{1024, 1048576} {
			value := map[string]any{"args": strings.Repeat(unit, size/len(unit))}
			b.Run(fmt.Sprintf("%s/bytes=%d", name, size), func(b *testing.B) {
				m := New()
				b.ReportAllocs()
				for b.Loop() {
					m.SetValue(value)
				}
			})
		}
	}
}

// Run with -benchtime=1x -count=3; heap deltas are diagnostics, not assertions.
// Keep the input alive throughout to measure only the component's retained state.
func BenchmarkJSONRetainedState(b *testing.B) {
	for name, input := range map[string]func() any{
		"ascii": func() any { return map[string]any{"args": strings.Repeat("x", 1048576)} },
		"unicode": func() any {
			unit := "界e\u0301👩🏽‍💻🇨🇦"
			return map[string]any{"args": strings.Repeat(unit, 1048576/len(unit))}
		},
		"numbers": func() any {
			values := make([]int, 10000)
			for i := range values {
				values[i] = i
			}
			return values
		},
	} {
		b.Run(name, func(b *testing.B) {
			value := input()
			heap := func() float64 {
				runtime.GC()
				runtime.GC()
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				return float64(stats.HeapAlloc)
			}
			before := heap()
			m := New()
			m.SetValue(value)
			live := heap()
			runtime.KeepAlive(m)
			m.SetValue("replacement")
			replaced := heap()
			runtime.KeepAlive(m)
			m.SetValue(nil)
			released := heap()
			runtime.KeepAlive(m)
			runtime.KeepAlive(value)
			b.ReportMetric(live-before, "live-B")
			b.ReportMetric(replaced-before, "replaced-B")
			b.ReportMetric(released-before, "released-B")
		})
	}
}
