package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

// confirmationReply skips lazy-table spinner timers in command batches.
func confirmationReply(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		return confirmationReply(batch[len(batch)-1])
	}
	return msg
}

func confirmationKey(key rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: key, Text: string(key)}
}

func TestAppSortedConfirmationPreservesOperation(t *testing.T) {
	shortcuts := map[string]tea.KeyPressMsg{
		"R": confirmationKey('R'), "K": confirmationKey('K'), "D": confirmationKey('D'),
		"ctrl+r": {Code: 'r', Mod: tea.ModCtrl},
		"ctrl+k": {Code: 'k', Mod: tea.ModCtrl},
		"ctrl+d": {Code: 'd', Mod: tea.ModCtrl},
	}
	for name, tc := range map[string]struct {
		id   viewID
		set  string
		keys []string
	}{
		"retries":   {viewRetries, "retry", []string{"R", "K", "D", "ctrl+r", "ctrl+k", "ctrl+d"}},
		"scheduled": {viewScheduled, "schedule", []string{"R", "D", "ctrl+r", "ctrl+d"}},
		"dead":      {viewDead, "dead", []string{"R", "D", "ctrl+r", "ctrl+d"}},
	} {
		for _, first := range tc.keys {
			for _, second := range tc.keys {
				for order, reverse := range map[string]bool{"first open first": false, "second open first": true} {
					for selection, changed := range map[string]bool{"same entry": false, "different entry": true} {
						t.Run(fmt.Sprintf("%s/%s,%s/%s/%s", name, first, second, order, selection), func(t *testing.T) {
							mr := miniredis.RunT(t)
							client, err := sidekiq.NewClient("redis://" + mr.Addr())
							if err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() { _ = client.Close() })
							raw := `{"jid":"original","class":"OriginalJob","queue":"default","args":[]}`
							other := `{"jid":"other","class":"OtherJob","queue":"default","args":[]}`
							_, _ = mr.ZAdd(tc.set, 1, raw)
							_, _ = mr.ZAdd(tc.set, 1, other)
							app := New(client, "", true, nil)
							t.Cleanup(app.cancel)
							dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
							dialogStep(&app, confirmationReply(app.setActiveView(tc.id)))
							// Dead is newest first; equal scores sort by member descending.
							if tc.set == "dead" {
								dialogStep(&app, confirmationKey('j'))
							}
							open := dialogStep(&app, shortcuts[first])
							if changed {
								key := 'j'
								if tc.set == "dead" {
									key = 'k'
								}
								dialogStep(&app, confirmationKey(key))
							}
							next := dialogStep(&app, shortcuts[second])
							if reverse {
								dialogStep(&app, confirmationReply(next))
							}
							dialogStep(&app, confirmationReply(open))
							if !reverse {
								dialogStep(&app, confirmationReply(next))
							}
							wantText := map[string]string{
								"R": "enqueue it immediately", "ctrl+r": "enqueue them immediately",
								"K": "move the job to the dead queue", "ctrl+k": "move them to the dead queue",
								"D": "not recoverable", "ctrl+d": "not recoverable",
							}[first]
							wantTarget, wantMutated := "OriginalJob", int64(1)
							if strings.HasPrefix(first, "ctrl+") {
								wantTarget, wantMutated = "all", 2
							}
							if app.dialogs.ActiveModel() == nil {
								t.Fatal("missing confirmation")
							}
							if text := ansi.Strip(app.dialogs.ActiveModel().View()); !strings.Contains(text, wantText) || !strings.Contains(text, wantTarget) {
								t.Fatalf("confirmation changed the first operation: %s", text)
							}
							completion := confirmationReply(dialogStep(&app, confirmationKey('y')))
							mutation := dialogStep(&app, completion)
							dialogStep(&app, confirmationReply(mutation))
							if duplicate := dialogStep(&app, completion); duplicate != nil {
								t.Fatal("duplicate confirmation submitted another mutation")
							}
							if app.connectionError != nil {
								t.Fatal(app.connectionError)
							}
							if client.Redis().ZScore(t.Context(), tc.set, raw).Err() == nil || client.Redis().ZCard(t.Context(), tc.set).Val() != 2-wantMutated {
								t.Fatal("confirmation changed the original operation's scope")
							}
							queued := client.Redis().LLen(t.Context(), "queue:default").Val()
							wantQueued := int64(0)
							if first == "R" || first == "ctrl+r" {
								wantQueued = wantMutated
							}
							if queued != wantQueued {
								t.Fatalf("queued = %d, want %d", queued, wantQueued)
							}
							wantDead := int64(0)
							if first == "K" || first == "ctrl+k" {
								wantDead = wantMutated
							}
							if tc.set != "dead" && client.Redis().ZCard(t.Context(), "dead").Val() != wantDead {
								t.Fatalf("confirmation changed the dead queue, want %d jobs", wantDead)
							}
							if wantQueued > 0 {
								payloads := strings.Join(client.Redis().LRange(t.Context(), "queue:default", 0, -1).Val(), "\n")
								if !strings.Contains(payloads, `"jid":"original"`) || (wantQueued == 2 && !strings.Contains(payloads, `"jid":"other"`)) {
									t.Fatalf("enqueued different entries: %s", payloads)
								}
							}
						})
					}
				}
			}
		}
	}
}

func TestAppSortedConfirmationKeepsEntrySnapshot(t *testing.T) {
	for name, tc := range map[string]struct {
		id  viewID
		set string
	}{
		"retries":   {viewRetries, "retry"},
		"scheduled": {viewScheduled, "schedule"},
		"dead":      {viewDead, "dead"},
	} {
		t.Run(name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			client, err := sidekiq.NewClient("redis://" + mr.Addr())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			original := `{"jid":"same-jid","class":"OriginalJob","queue":"default","args":[1]}`
			replacement := `{"jid":"same-jid","class":"ReplacementJob","queue":"other","args":[2]}`
			_, _ = mr.ZAdd(tc.set, 1, original)
			app := New(client, "", true, nil)
			t.Cleanup(app.cancel)
			dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
			dialogStep(&app, confirmationReply(app.setActiveView(tc.id)))
			open := dialogStep(&app, confirmationKey('R'))
			client.Redis().ZRem(t.Context(), tc.set, original)
			_, _ = mr.ZAdd(tc.set, 2, replacement)
			dialogStep(&app, confirmationReply(app.updateView(tc.id, views.RefreshMsg{})))
			dialogStep(&app, confirmationKey('D'))
			dialogStep(&app, confirmationReply(open))
			if text := ansi.Strip(app.dialogs.ActiveModel().View()); !strings.Contains(text, "OriginalJob") || !strings.Contains(text, "enqueue it immediately") {
				t.Fatalf("confirmation changed entry: %s", text)
			}
			mutation := dialogStep(&app, confirmationReply(dialogStep(&app, confirmationKey('y'))))
			dialogStep(&app, confirmationReply(mutation))
			if app.connectionError != nil || !strings.HasSuffix(app.actionError, " failed for JID same-jid: job not found") {
				t.Fatalf("replaced source should fail without using the replacement: %q, %v", app.actionError, app.connectionError)
			}
			if client.Redis().ZScore(t.Context(), tc.set, replacement).Err() != nil || mr.Exists("queue:default") || mr.Exists("queue:other") {
				t.Fatal("confirmation mutated a different entry with the same JID")
			}
		})
	}
}

func TestAppAbandonedConfirmationReleasesAction(t *testing.T) {
	for name, tc := range map[string]struct {
		periodic bool
		detail   bool
		key      rune
		id       viewID
		set      string
	}{
		"queue delete":            {key: 'd'},
		"queue pause":             {key: 'p'},
		"queue detail":            {detail: true, key: 'p'},
		"periodic enqueue":        {periodic: true, key: 'e'},
		"periodic pause":          {periodic: true, key: 'p'},
		"periodic detail enqueue": {periodic: true, detail: true, key: 'e'},
		"periodic detail pause":   {periodic: true, detail: true, key: 'p'},
		"retries":                 {key: 'R', id: viewRetries, set: "retry"},
		"scheduled":               {key: 'R', id: viewScheduled, set: "schedule"},
		"dead":                    {key: 'R', id: viewDead, set: "dead"},
	} {
		for cover, key := range map[string]tea.KeyPressMsg{
			"help":    confirmationKey('?'),
			"console": {Code: tea.KeyF12},
			"palette": {Code: 't', Mod: tea.ModCtrl},
		} {
			for timing, phase := range map[string]int{"rejected under cover": 0, "late after navigation": 1, "late during mutation": 2} {
				t.Run(name+"/"+cover+"/"+timing, func(t *testing.T) {
					var app App
					var mr *miniredis.Miniredis
					switch {
					case tc.periodic:
						app, mr = newPeriodicTestApp(t, true, tc.detail)
					default:
						mr = miniredis.RunT(t)
						capabilities, err := sidekiq.ResolveFeatures([]string{"ent"})
						if err != nil {
							t.Fatal(err)
						}
						client, err := sidekiq.NewClient("redis://"+mr.Addr(), sidekiq.WithFeatures(capabilities))
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = client.Close() })
						_, _ = mr.SetAdd("queues", "default")
						_, _ = mr.Push("queue:default", `{"jid":"original","class":"OriginalJob","queue":"default","args":[]}`)
						app = New(client, "", true, devtools.NewTracker())
						dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
						if tc.set != "" {
							_ = mr.Del("queue:default")
							_, _ = mr.ZAdd(tc.set, 1, `{"jid":"original","class":"OriginalJob","queue":"default","args":[]}`)
							dialogStep(&app, confirmationReply(app.setActiveView(tc.id)))
						} else {
							dialogStep(&app, confirmationReply(app.setActiveView(viewQueuesList)))
						}
						if tc.detail {
							dialogStep(&app, confirmationReply(dialogStep(&app, views.ShowQueueDetailsMsg{QueueName: "default"})))
						}
					}
					t.Cleanup(app.cancel)
					app.devTracker = devtools.NewTracker()
					app.keys.DevTools.SetEnabled(true)
					pending := confirmationReply(dialogStep(&app, confirmationKey(tc.key)))
					if pending == nil {
						t.Fatal("missing initial confirmation")
					}
					dialogStep(&app, confirmationReply(dialogStep(&app, key)))
					session := app.dialogs.ActiveSession()
					if session == nil {
						t.Fatal("global dialog did not open")
					}
					if phase == 0 {
						dialogStep(&app, pending)
					}
					if app.dialogs.ActiveSession() != session {
						t.Fatal("abandoned open replaced covering dialog")
					}
					dialogStep(&app, confirmationReply(dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEsc})))
					if phase != 0 {
						active := app.activeViewID()
						dialogStep(&app, confirmationReply(app.setActiveView(viewDashboard)))
						dialogStep(&app, confirmationReply(app.setActiveView(active)))
					}
					dialogStep(&app, confirmationReply(app.updateView(app.activeViewID(), views.RefreshMsg{})))
					next := dialogStep(&app, confirmationKey(tc.key))
					if next == nil {
						t.Fatal("abandoned confirmation blocked a fresh action after refresh")
					}
					opened := confirmationReply(next)
					dialogStep(&app, opened)
					// Neither a late rejected open nor a duplicate accepted open may
					// release the replacement operation.
					if phase != 2 {
						dialogStep(&app, pending)
					}
					dialogStep(&app, opened)
					if tc.periodic && (mr.Exists("queue:default") || mr.HGet("loop-fixture", "paused") != "") {
						t.Fatal("abandoned preparation mutated periodic state")
					}
					if !tc.periodic && tc.set == "" && !mr.Exists("queue:default") {
						t.Fatal("abandoned preparation deleted queue")
					}
					completion := confirmationReply(dialogStep(&app, confirmationKey('y')))
					mutation := dialogStep(&app, completion)
					dialogStep(&app, pending)
					if tc.set == "" && dialogStep(&app, confirmationKey(tc.key)) != nil {
						t.Fatal("late cleanup released a running mutation")
					}
					app = applyLayoutRead(app, mutation)
					app = applyLayoutRead(app, dialogStep(&app, completion))
					if app.connectionError != nil {
						t.Fatal(app.connectionError)
					}
					switch {
					case tc.periodic && tc.key == 'e':
						jobs, _ := mr.List("queue:default")
						if len(jobs) != 1 {
							t.Fatalf("enqueue created %d jobs, want one", len(jobs))
						}
					case tc.periodic:
						if mr.HGet("loop-fixture", "paused") != "1" {
							t.Fatal("fresh pause confirmation was lost")
						}
					case tc.set != "":
						jobs, _ := mr.List("queue:default")
						if len(jobs) != 1 || mr.Exists(tc.set) {
							t.Fatal("fresh confirmation did not enqueue original sorted entry once")
						}
					case tc.key == 'd':
						if mr.Exists("queue:default") {
							t.Fatal("fresh delete confirmation was lost")
						}
					default:
						if paused, _ := mr.SIsMember("paused", "default"); !paused || !mr.Exists("queue:default") {
							t.Fatal("fresh pause did not preserve and pause queue")
						}
					}
				})
			}
		}
	}
}
