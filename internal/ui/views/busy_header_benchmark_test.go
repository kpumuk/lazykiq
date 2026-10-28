package views

import (
	"fmt"
	"testing"
	"time"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func busyHeaderProcesses(size int) []sidekiq.Process {
	processes := make([]sidekiq.Process, size)
	for i := range size {
		processes[i] = sidekiq.Process{
			Identity: fmt.Sprintf("worker-%05d:12345:nonce", i),
			Hostname: fmt.Sprintf("worker-%05d", i), PID: 12345,
			StartedAt: time.Now().Add(-time.Hour), Busy: 3, Concurrency: 25,
		}
	}
	return processes
}

func BenchmarkAuditViewBusyHeader(b *testing.B) {
	for _, size := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprintf("processes_%d", size), func(b *testing.B) {
			view := NewBusy(nil)
			view.SetSize(160, 40)
			view.Update(busyDataMsg{data: sidekiq.BusyData{Processes: busyHeaderProcesses(size)}})
			b.ReportAllocs()
			for b.Loop() {
				if len(view.HeaderLines()) != 5 {
					b.Fatal("header must contain five rows")
				}
			}
		})
	}
}

func BenchmarkBusyHeaderRefresh(b *testing.B) {
	for _, size := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprintf("processes_%d", size), func(b *testing.B) {
			view := NewBusy(nil)
			view.SetSize(160, 40)
			msg := busyDataMsg{data: sidekiq.BusyData{Processes: busyHeaderProcesses(size)}}
			b.ReportAllocs()
			for b.Loop() {
				view.Update(msg)
				view.HeaderLines()
			}
		})
	}
}
