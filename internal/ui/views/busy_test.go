package views

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

type busyFetchClient struct {
	sidekiq.API
	name string
}

func TestBusyRefreshPreservesProcessSelection(t *testing.T) {
	for name, tt := range map[string]struct {
		before   []string
		after    []string
		selectID string
		want     string
	}{
		"insert before":                   {[]string{"B", "C"}, []string{"A", "B", "C"}, "B", "B"},
		"insert after":                    {[]string{"A", "B"}, []string{"A", "B", "C"}, "B", "B"},
		"remove before":                   {[]string{"A", "B", "C"}, []string{"B", "C"}, "B", "B"},
		"remove after":                    {[]string{"A", "B", "C"}, []string{"A", "B"}, "B", "B"},
		"reorder":                         {[]string{"A", "B", "C"}, []string{"C", "A", "B"}, "B", "B"},
		"selected removed at valid index": {[]string{"A", "B", "C"}, []string{"A", "C"}, "B", ""},
		"selected removed at last index":  {[]string{"A", "B"}, []string{"A"}, "B", ""},
		"empty":                           {[]string{"A", "B"}, nil, "B", ""},
		"all":                             {[]string{"B", "C"}, []string{"A", "B", "C"}, "", ""},
	} {
		for mode, tree := range map[string]bool{"flat": false, "tree": true} {
			for selection, selectProcess := range map[string]func(*Busy){
				"identity": func(b *Busy) { b.SetProcessIdentity(tt.selectID) },
				"shortcut": func(b *Busy) {
					code := '0'
					if tt.selectID != "" {
						code = []rune("123456789")[slices.Index(tt.before, tt.selectID)]
					}
					b.Update(tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl})
				},
			} {
				t.Run(name+"/"+mode+"/"+selection, func(t *testing.T) {
					b := NewBusy(nil)
					b.treeMode = tree
					b.SetSize(160, 30)
					b.Update(busyDataMsg{data: busySelectionData(tt.before)})
					selectProcess(b)
					if got := b.selectedIdentity(); got != tt.selectID {
						t.Fatalf("initial selection = %q, want %q", got, tt.selectID)
					}
					b.HeaderLines()
					b.Update(busyDataMsg{data: busySelectionData(tt.after)})
					if got := b.selectedIdentity(); got != tt.want {
						t.Fatalf("selection after refresh = %q, want %q", got, tt.want)
					}
					wantJobs := tt.after
					if tt.want != "" {
						wantJobs = []string{tt.want}
						if !strings.Contains(b.View(), "Active Jobs on "+tt.want+":1") {
							t.Fatal("title lost selected process")
						}
					}
					if len(b.filteredJobs) != len(wantJobs) {
						t.Fatalf("filtered jobs = %d, want %d", len(b.filteredJobs), len(wantJobs))
					}
					for i, job := range b.filteredJobs {
						if job.ProcessIdentity != wantJobs[i] {
							t.Fatalf("job process = %q, want %q", job.ProcessIdentity, wantJobs[i])
						}
					}
					for i, proc := range b.data.Processes {
						if proc.Identity != tt.after[i] {
							t.Fatal("refresh reordered processes")
						}
					}
					// A later reappearance must not restore a removed selection.
					b.Update(busyDataMsg{data: busySelectionData(tt.before)})
					if got := b.selectedIdentity(); got != tt.want {
						t.Fatalf("selection after second refresh = %q, want %q", got, tt.want)
					}
				})
			}
		}
	}
}

func busySelectionData(identities []string) sidekiq.BusyData {
	data := sidekiq.BusyData{}
	for _, identity := range identities {
		data.Processes = append(data.Processes, sidekiq.Process{Identity: identity, Hostname: identity, PID: 1})
		data.Jobs = append(data.Jobs, sidekiq.Job{
			ProcessIdentity: identity,
			JobRecord:       sidekiq.NewJobRecord(fmt.Sprintf(`{"jid":%q,"class":"ExampleJob","args":[]}`, identity), "default"),
		})
	}
	return data
}

func (c *busyFetchClient) GetBusyData(_ context.Context, query string) (sidekiq.BusyData, error) {
	return sidekiq.BusyData{Processes: []sidekiq.Process{{Identity: c.name + query}}}, nil
}

func TestBusyFetchCapturesInputs(t *testing.T) {
	for name, mutate := range map[string]func(*Busy){
		"filter": func(b *Busy) { b.filter = "new" },
		"client": func(b *Busy) { b.client = &busyFetchClient{name: "replacement:"} },
	} {
		t.Run(name, func(t *testing.T) {
			b := NewBusy(&busyFetchClient{name: "original:"})
			t.Cleanup(b.CancelRequests)
			b.filter = "old"
			cmd := b.fetchDataCmd()
			mutate(b)

			msg, ok := cmd().(ReadResultMsg).Data.(busyDataMsg)
			if !ok {
				t.Fatal("expected busy data")
			}
			if got := msg.data.Processes[0].Identity; got != "original:old" {
				t.Fatalf("request = %q, want original:old", got)
			}
		})
	}
}
