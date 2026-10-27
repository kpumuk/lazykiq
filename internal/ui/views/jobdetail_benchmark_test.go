package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

var (
	jobDetailBenchmarkLines []string
	jobDetailBenchmarkPanel string
)

func BenchmarkJobDetailWrap(b *testing.B) {
	for _, size := range []int{1024, 4096, 16384, 65536} {
		for name, input := range map[string]string{
			"ascii":     strings.Repeat("x", size),
			"graphemes": strings.Repeat("e\u0301👩🏽‍💻界", size/len("e\u0301👩🏽‍💻界")),
		} {
			b.Run(fmt.Sprintf("%s/bytes_%d", name, len(input)), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					jobDetailBenchmarkLines = wrapText(input, 40)
				}
			})
		}
	}
}

func BenchmarkJobDetailLeft(b *testing.B) {
	for _, size := range []int{1024, 4096, 16384, 65536} {
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			for _, operation := range []string{"redraw", "scroll", "resize", "set_job"} {
				b.Run(operation, func(b *testing.B) {
					view := NewJobDetail()
					view.SetSize(160, 40)
					job := sidekiq.NewJobRecord(fmt.Sprintf(
						`{"jid":"wide","class":"TestJob","args":[%q]}`, strings.Repeat("x", size),
					), "default")
					view.SetJob(job)
					view.renderLeftPanel() // Warm the property/JSON caches outside the timer.
					b.ReportAllocs()
					for b.Loop() {
						switch operation {
						case "scroll":
							if view.leftYOffset == view.maxLeftYOffset() {
								view.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
							} else {
								view.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
							}
						case "resize":
							width := 120
							if view.width == 120 {
								width = 160
							}
							view.SetSize(width, 40)
						case "set_job":
							view.SetJob(job)
						}
						jobDetailBenchmarkPanel = view.renderLeftPanel()
					}
				})
			}
		})
	}
}
