package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

func assertLayoutBounds(t *testing.T, output string, width, height int) {
	t.Helper()
	if output == "" {
		return
	}
	w, h := lipgloss.Size(output)
	if w > max(width, 0) || h > max(height, 0) {
		t.Fatalf("allocated %dx%d, rendered %dx%d:\n%s", width, height, w, h, ansi.Strip(output))
	}
}

func TestAppLayoutBounds(t *testing.T) {
	sizes := map[string][2]int{"zero": {}, "zero width": {0, 20}, "zero height": {80, 0}, "negative": {-1, -1}, "one cell": {1, 1}, "one column": {1, 20}, "short": {80, 8}, "small": {80, 10}, "narrow": {20, 10}, "normal": {120, 24}}
	for _, id := range []viewID{viewDashboard, viewBusy, viewQueueDetails, viewRetries, viewScheduled, viewDead, viewErrorsSummary, viewMetrics} {
		for name, size := range sizes {
			t.Run(fmt.Sprintf("%d/%s", id, name), func(t *testing.T) {
				a := New(&ownershipClient{}, "", false, nil)
				a.setActiveView(id)
				m, _ := a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				a = m.(App)
				assertLayoutBounds(t, a.View().Content, size[0], size[1])
				if a.contentHeight() < 0 || a.contextbar.Height() < 0 {
					t.Fatal("negative allocation")
				}
				a.connectionError = errors.New(strings.Repeat("界e\u0301👩🏽‍💻", 30))
				assertLayoutBounds(t, a.View().Content, size[0], size[1])
				m, _ = a.Update(dialogs.OpenDialogMsg{Model: filterdialog.New()})
				a = m.(App)
				assertLayoutBounds(t, a.View().Content, size[0], size[1])
			})
		}
	}
}

func TestAppDialogResizePreservesInput(t *testing.T) {
	for _, console := range []bool{false, true} {
		a := appWithInputDialog(t, console)
		for _, r := range "界e\u0301 queue 123" {
			m, _ := a.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			a = m.(App)
		}
		original := a.dialogs.ActiveModel()
		session := a.dialogs.ActiveSession()
		for _, size := range [][2]int{{80, 10}, {20, 10}, {1, 1}, {0, 0}, {120, 24}, {300, 40}} {
			m, _ := a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			a = m.(App)
			assertLayoutBounds(t, a.View().Content, size[0], size[1])
			if a.dialogs.ActiveModel() != original || a.dialogs.ActiveSession() != session {
				t.Fatal("resize replaced input owner")
			}
		}
		if !strings.Contains(ansi.Strip(a.dialogs.ActiveModel().View()), "界e\u0301 queue 123") {
			t.Fatal("resize lost input")
		}
	}
}

func TestAppLayoutCompactAndRestore(t *testing.T) {
	a := busyHeaderApp(10)
	a.syncContextbar()
	original := a.View().Content
	for _, size := range [][2]int{{80, 10}, {20, 10}, {1, 1}, {0, 0}, {160, 40}} {
		m, _ := a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		a = m.(App)
		assertLayoutBounds(t, a.View().Content, size[0], size[1])
		if size == [2]int{80, 10} && (a.contextbar.Height() != 2 || a.contentHeight() != 5) {
			t.Fatal("short layout did not reserve content")
		}
	}
	if got := a.View().Content; got != original {
		t.Fatal("normal layout did not restore after resizing")
	}
}

func TestGoldenAppLayout(t *testing.T) {
	for name, tc := range map[string]struct {
		width, height int
		state         string
	}{
		"zero": {}, "tiny": {1, 1, "loading"}, "short loading": {80, 8, "loading"}, "empty": {20, 10, "empty"},
		"populated": {80, 10, "populated"}, "normal": {120, 24, "populated"}, "filter": {20, 10, "filter"},
		"error": {20, 10, "error"}, "confirm": {20, 10, "confirm"}, "help": {20, 10, "help"},
	} {
		t.Run(name, func(t *testing.T) {
			a := busyHeaderApp(0)
			switch tc.state {
			case "loading":
				a = New(&ownershipClient{}, "", false, nil)
				a.setActiveView(viewRetries)
			case "populated":
				a.viewRegistry[viewJobDetail].(views.JobDetailSetter).SetJob(sidekiq.NewJobRecord(`{"jid":"fixture","class":"界Worker","args":["é","👩🏽‍💻"]}`, "default"))
				a.pushView(viewJobDetail)
			case "error":
				a.connectionError = errors.New("Redis unavailable: 界")
			}
			m, _ := a.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			a = m.(App)
			switch tc.state {
			case "filter":
				m, _ = a.Update(dialogs.OpenDialogMsg{Model: filterdialog.New()})
				a = m.(App)
			case "confirm":
				m, _ = a.Update(dialogs.OpenDialogMsg{Model: confirmdialog.New()})
				a = m.(App)
			case "help":
				_, cmd := a.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
				m, _ = a.Update(cmd())
				a = m.(App)
			}
			output := a.View().Content
			assertLayoutBounds(t, output, tc.width, tc.height)
			golden.RequireEqual(t, []byte(ansi.Strip(output)))
		})
	}
}

// Resolve the initial read batch only; returned timers and refreshes belong to
// the running program, not this deterministic loading-to-data transition.
func applyLayoutRead(a App, cmd tea.Cmd) App {
	if cmd == nil {
		return a
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			a = applyLayoutRead(a, child)
		}
		return a
	}
	m, _ := a.Update(msg)
	return m.(App)
}

func TestAppLoadedLayoutBounds(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated=%t", populated), func(t *testing.T) {
			mr := miniredis.RunT(t)
			if populated {
				seedLayoutData(t, mr)
			}
			client, err := sidekiq.NewClient("redis://" + mr.Addr())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			for _, id := range []viewID{viewDashboard, viewBusy, viewQueueDetails, viewRetries, viewScheduled, viewDead, viewErrorsSummary, viewMetrics} {
				t.Run(fmt.Sprint(id), func(t *testing.T) {
					a := New(layoutRedisClient{client}, "", false, nil)
					cmd := a.setActiveView(id)
					a = applyLayoutRead(a, cmd)
					defer func() {
						for id := range a.viewRegistry {
							a.cancelViewRequests(id)
						}
					}()
					for _, size := range [][2]int{{120, 24}, {80, 10}, {80, 8}, {20, 10}, {1, 20}, {0, 0}, {120, 24}} {
						m, _ := a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
						a = m.(App)
						assertLayoutBounds(t, a.View().Content, size[0], size[1])
						if a.connectionError != nil {
							t.Fatalf("fixture read failed: %v", a.connectionError)
						}
					}
					if populated && id != viewDashboard && !strings.Contains(ansi.Strip(a.View().Content), "FixtureWorker") {
						t.Fatalf("fixture did not populate %s:\n%s", a.viewRegistry[id].Name(), ansi.Strip(a.View().Content))
					}
				})
			}
		})
	}
}

// Miniredis does not implement the multi-section INFO used by Dashboard.
type layoutRedisClient struct{ *sidekiq.Client }

func (c layoutRedisClient) GetRedisInfo(context.Context) (sidekiq.RedisInfo, error) {
	return sidekiq.RedisInfo{Version: "fixture"}, nil
}

func seedLayoutData(t *testing.T, mr *miniredis.Miniredis) {
	t.Helper()
	job := `{"jid":"fixture","class":"FixtureWorker","queue":"default","args":["界é👩🏽‍💻"],"error_class":"RuntimeError","error_message":"fixture error"}`
	if _, err := mr.SetAdd("queues", "default"); err != nil {
		t.Fatal(err)
	}
	if _, err := mr.Push("queue:default", job); err != nil {
		t.Fatal(err)
	}
	for _, set := range []string{"retry", "schedule", "dead"} {
		if _, err := mr.ZAdd(set, 1700000000, job); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mr.SetAdd("processes", "fixture:1:nonce"); err != nil {
		t.Fatal(err)
	}
	mr.HSet("fixture:1:nonce", "info", `{"hostname":"fixture","pid":1,"concurrency":5,"version":"8.1.0","queues":["default"]}`, "busy", "1", "beat", "1700000000")
	work, err := json.Marshal(map[string]any{"queue": "default", "payload": job, "run_at": 1700000000})
	if err != nil {
		t.Fatal(err)
	}
	mr.HSet("fixture:1:nonce:work", "a", string(work))
	now := time.Now().UTC()
	for _, minute := range []time.Time{now, now.Add(-time.Minute)} {
		mr.HSet(fmt.Sprintf("j|%s|%d:%02d", minute.Format("060102"), minute.Hour(), minute.Minute()), "FixtureWorker|p", "5", "FixtureWorker|f", "1", "FixtureWorker|ms", "100")
	}
}
