package views

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

type detailNavigationView interface {
	View
	Dispose()
}

type detailNavigationHarness struct {
	view      detailNavigationView
	setJobs   func([]*sidekiq.JobRecord)
	setCursor func(int)
	replace   func(*sidekiq.JobRecord)
	message   func(*sidekiq.JobRecord) tea.Msg
}

func detailNavigationViews() map[string]func() detailNavigationHarness {
	jobMessage := func(job *sidekiq.JobRecord) tea.Msg { return ShowJobDetailMsg{Job: job} }
	sorted := func(view detailNavigationView, list *sortedJobsView) detailNavigationHarness {
		return detailNavigationHarness{
			view: view,
			setJobs: func(jobs []*sidekiq.JobRecord) {
				list.jobs = make([]*sidekiq.SortedEntry, len(jobs))
				rows := make([]table.Row, len(jobs))
				for i, job := range jobs {
					list.jobs[i] = &sidekiq.SortedEntry{JobRecord: job}
					rows[i] = table.Row{ID: job.JID()}
				}
				list.lazy.Table().SetRows(rows)
			},
			setCursor: func(cursor int) { list.lazy.Table().SetCursor(cursor) },
			replace:   func(job *sidekiq.JobRecord) { list.jobs[1].JobRecord = job },
			message:   jobMessage,
		}
	}
	return map[string]func() detailNavigationHarness{
		"busy": func() detailNavigationHarness {
			view := NewBusy(nil)
			return detailNavigationHarness{
				view: view,
				setJobs: func(jobs []*sidekiq.JobRecord) {
					data := sidekiq.BusyData{}
					for _, job := range jobs {
						data.Jobs = append(data.Jobs, sidekiq.Job{JobRecord: job})
					}
					view.Update(busyDataMsg{data: data})
				},
				setCursor: view.table.SetCursor,
				replace:   func(job *sidekiq.JobRecord) { view.filteredJobs[1].JobRecord = job },
				message:   jobMessage,
			}
		},
		"queues": func() detailNavigationHarness {
			view := NewQueuesList(nil)
			return detailNavigationHarness{
				view: view,
				setJobs: func(jobs []*sidekiq.JobRecord) {
					queues := make([]*QueuesListInfo, len(jobs))
					for i, job := range jobs {
						queues[i] = &QueuesListInfo{Name: job.Queue()}
					}
					view.Update(queuesListDataMsg{queues: queues, requestID: view.requestID})
				},
				setCursor: view.table.SetCursor,
				replace:   func(job *sidekiq.JobRecord) { view.queues[1].Name = job.Queue() },
				message: func(job *sidekiq.JobRecord) tea.Msg {
					return ShowQueueDetailsMsg{QueueName: job.Queue()}
				},
			}
		},
		"retries": func() detailNavigationHarness {
			view := NewRetries(nil)
			return sorted(view, &view.sortedJobsView)
		},
		"scheduled": func() detailNavigationHarness {
			view := NewScheduled(nil)
			return sorted(view, &view.sortedJobsView)
		},
		"dead": func() detailNavigationHarness {
			view := NewDead(nil)
			return sorted(view, &view.sortedJobsView)
		},
	}
}

func detailNavigationJob(name string) *sidekiq.JobRecord {
	return sidekiq.NewJobRecord(fmt.Sprintf(`{"jid":%q,"class":"ExampleJob","args":[]}`, name), name)
}

func TestDetailNavigationSnapshot(t *testing.T) {
	first, selected, replacement := detailNavigationJob("first"), detailNavigationJob("selected"), detailNavigationJob("replacement")
	for name, build := range detailNavigationViews() {
		for change, mutate := range map[string]func(detailNavigationHarness){
			"unchanged":             func(detailNavigationHarness) {},
			"cursor moved":          func(h detailNavigationHarness) { h.setCursor(0) },
			"disposed":              func(h detailNavigationHarness) { h.view.Dispose() },
			"empty replacement":     func(h detailNavigationHarness) { h.setJobs(nil) },
			"shorter replacement":   func(h detailNavigationHarness) { h.setJobs([]*sidekiq.JobRecord{replacement}) },
			"same size replacement": func(h detailNavigationHarness) { h.setJobs([]*sidekiq.JobRecord{first, replacement}) },
			"in place replacement":  func(h detailNavigationHarness) { h.replace(replacement) },
		} {
			t.Run(name+"/"+change, func(t *testing.T) {
				h := build()
				h.setJobs([]*sidekiq.JobRecord{first, selected})
				h.setCursor(1)
				_, cmd := h.view.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if cmd == nil {
					t.Fatal("Enter did not return a navigation command")
				}
				mutate(h)
				defer func() {
					if err := recover(); err != nil {
						t.Fatalf("deferred Enter panicked: %v", err)
					}
				}()
				if got, want := cmd(), h.message(selected); got != want {
					t.Fatalf("navigation = %#v, want %#v", got, want)
				}
			})
		}
	}
}

func TestDetailNavigationEmpty(t *testing.T) {
	for name, build := range detailNavigationViews() {
		t.Run(name, func(t *testing.T) {
			h := build()
			if _, cmd := h.view.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
				t.Fatal("empty view returned a navigation command")
			}
		})
	}
}

func TestBusyTreeDetailNavigation(t *testing.T) {
	view := NewBusy(nil)
	view.treeMode = true
	selected := detailNavigationJob("selected")
	view.Update(busyDataMsg{data: sidekiq.BusyData{
		Processes: []sidekiq.Process{{Identity: "worker"}},
		Jobs:      []sidekiq.Job{{JobRecord: selected, ProcessIdentity: "worker"}},
	}})
	if _, cmd := view.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("process header returned a job navigation command")
	}
	view.table.SetCursor(1)
	_, cmd := view.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("job row did not return a navigation command")
	}
	view.Dispose()
	if got, want := cmd(), (ShowJobDetailMsg{Job: selected}); got != want {
		t.Fatalf("navigation = %#v, want %#v", got, want)
	}
}

func TestDetailNavigationConcurrentReplacement(t *testing.T) {
	first, selected, replacement := detailNavigationJob("first"), detailNavigationJob("selected"), detailNavigationJob("replacement")
	for name, build := range detailNavigationViews() {
		for change, mutate := range map[string]func(detailNavigationHarness){
			"in place replacement": func(h detailNavigationHarness) { h.replace(replacement) },
			"row replacement":      func(h detailNavigationHarness) { h.setJobs([]*sidekiq.JobRecord{first, replacement}) },
			"disposal":             func(h detailNavigationHarness) { h.view.Dispose() },
		} {
			t.Run(name+"/"+change, func(t *testing.T) {
				h := build()
				h.setJobs([]*sidekiq.JobRecord{first, selected})
				h.setCursor(1)
				_, cmd := h.view.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if cmd == nil {
					t.Fatal("Enter did not return a navigation command")
				}
				start := make(chan struct{})
				type result struct {
					message tea.Msg
					panic   any
				}
				done := make(chan result, 1)
				go func() {
					var got result
					defer func() {
						got.panic = recover()
						done <- got
					}()
					<-start
					for range 1000 {
						got.message = cmd()
					}
				}()
				close(start)
				for range 1000 {
					mutate(h)
				}
				got := <-done
				if got.panic != nil {
					t.Fatalf("deferred Enter panicked: %v", got.panic)
				}
				if want := h.message(selected); got.message != want {
					t.Fatalf("navigation = %#v, want %#v", got.message, want)
				}
			})
		}
	}
}
