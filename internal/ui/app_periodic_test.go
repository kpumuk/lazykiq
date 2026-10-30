package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

type periodicAppClient struct {
	sidekiq.API
	features sidekiq.Capabilities
}

func (c periodicAppClient) GetCapabilities() sidekiq.Capabilities { return c.features }
func (periodicAppClient) GetPeriodicJobs(context.Context) ([]sidekiq.PeriodicJob, error) {
	return nil, nil
}

func (periodicAppClient) GetPeriodicJob(context.Context, string) (*sidekiq.PeriodicJob, []sidekiq.PeriodicExecution, error) {
	return nil, nil, nil
}

func TestAppPeriodicEnterpriseGate(t *testing.T) {
	for name, features := range map[string][]string{"default": nil, "pro": {"pro"}, "enterprise": {"ent"}} {
		t.Run(name, func(t *testing.T) {
			capabilities, err := sidekiq.ResolveFeatures(features)
			if err != nil {
				t.Fatal(err)
			}
			app := New(periodicAppClient{features: capabilities}, "", false, nil)
			want := name == "enterprise"
			if _, registered := app.viewRegistry[viewPeriodic]; registered != want || app.keys.View9.Enabled() != want {
				t.Fatal("view registration/key escaped Enterprise gate")
			}
			if _, registered := app.viewRegistry[viewPeriodicDetail]; registered != want {
				t.Fatal("detail registration escaped Enterprise gate")
			}
			app.navbar.SetWidth(240)
			if shown := strings.Contains(ansi.Strip(app.navbar.View()), "Periodic"); shown != want {
				t.Fatal("navbar escaped Enterprise gate")
			}
			updateMetricsApp(&app, tea.KeyPressMsg{Code: '9', Text: "9"})
			if active := app.activeViewID() == viewPeriodic; active != want {
				t.Fatal("shortcut escaped Enterprise gate")
			}
			updateMetricsApp(&app, views.ShowPeriodicDetailMsg{LID: "id"})
			if active := app.activeViewID() == viewPeriodicDetail; active != want {
				t.Fatal("detail message escaped Enterprise gate")
			}
			if want {
				cmd := updateMetricsApp(&app, tea.KeyPressMsg{Code: tea.KeyEscape})
				if app.activeViewID() != viewPeriodic || len(app.viewStack) != 1 || cmd == nil {
					t.Fatal("return from detail did not restore and refresh list")
				}
			}
		})
	}
}

func newPeriodicTestApp(t *testing.T, populated, detail bool) (App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	if populated {
		if err := mr.Set("periodic-version", "v1"); err != nil {
			t.Fatal(err)
		}
		if _, err := mr.SetAdd("loops-v1", "fixture"); err != nil {
			t.Fatal(err)
		}
		mr.HSet("loop-fixture", "class", "FixtureWorker", "schedule", "*/5 * * * *", "options", `{"queue":"default","args":[]}`)
		if _, err := mr.ZAdd("loop-history-fixture", 1700000000, "abcdef012345678901234567"); err != nil {
			t.Fatal(err)
		}
	}
	capabilities, err := sidekiq.ResolveFeatures([]string{"ent"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := sidekiq.NewClient("redis://"+mr.Addr(), sidekiq.WithFeatures(capabilities))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	app := New(client, "", true, nil)
	dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 24})
	cmd := app.setActiveView(viewPeriodic)
	app = applyLayoutRead(app, cmd)
	if detail {
		cmd = dialogStep(&app, views.ShowPeriodicDetailMsg{LID: "fixture"})
		app = applyLayoutRead(app, cmd)
	}
	t.Cleanup(func() {
		for id := range app.viewRegistry {
			app.cancelViewRequests(id)
		}
	})
	return app, mr
}

func TestAppPeriodicOwnedConfirmations(t *testing.T) {
	for name, detail := range map[string]bool{"list": false, "detail": true} {
		for action, key := range map[string]rune{"pause": 'p', "enqueue": 'e'} {
			t.Run(name+"/"+action, func(t *testing.T) {
				app, mr := newPeriodicTestApp(t, true, detail)
				open := func() {
					t.Helper()
					cmd := dialogStep(&app, tea.KeyPressMsg{Code: key, Text: string(key)})
					if cmd == nil {
						t.Fatal("missing confirmation")
					}
					dialogStep(&app, cmd())
				}
				apply := func(cmd tea.Cmd) { app = applyLayoutRead(app, cmd) }
				open()
				stale := dialogStep(&app, tea.KeyPressMsg{Code: 'y', Text: "y"})()
				cancel := dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEscape})()
				apply(dialogStep(&app, cancel))
				open()
				session := app.dialogs.ActiveSession()
				apply(dialogStep(&app, stale))
				if app.dialogs.ActiveSession() != session || mr.HGet("loop-fixture", "paused") != "" || mr.Exists("queue:default") {
					t.Fatal("stale completion affected replacement confirmation or Redis")
				}
				confirmed := dialogStep(&app, tea.KeyPressMsg{Code: 'y', Text: "y"})()
				apply(dialogStep(&app, confirmed))
				apply(dialogStep(&app, confirmed)) // Duplicate delivery must not enqueue twice.
				if app.dialogs.HasDialogs() || app.connectionError != nil {
					t.Fatalf("confirmation did not complete: %v", app.connectionError)
				}
				if action == "pause" {
					if mr.HGet("loop-fixture", "paused") != "1" {
						t.Fatal("owned confirmation did not pause schedule")
					}
				} else if jobs, _ := mr.List("queue:default"); len(jobs) != 1 || !strings.Contains(jobs[0], "FixtureWorker") {
					t.Fatalf("owned confirmation enqueued %v", jobs)
				}
			})
		}
	}
}

func TestAppPeriodicOwnedFilter(t *testing.T) {
	app, _ := newPeriodicTestApp(t, true, false)
	open := dialogStep(&app, tea.KeyPressMsg{Code: '/', Text: "/"})
	dialogStep(&app, open())
	for _, r := range "missing" {
		dialogStep(&app, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	completion := dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})()
	dialogStep(&app, completion)
	if output := ansi.Strip(app.View().Content); !strings.Contains(output, "No matches") || !strings.Contains(output, "[missing]") || app.dialogs.HasDialogs() {
		t.Fatalf("filter did not reach Periodic: %s", output)
	}
}

func TestAppPeriodicLayoutBounds(t *testing.T) {
	for name, detail := range map[string]bool{"list": false, "detail": true} {
		for state, populated := range map[string]bool{"loading": false, "empty": false, "populated": true} {
			t.Run(name+"/"+state, func(t *testing.T) {
				app, _ := newPeriodicTestApp(t, populated, detail)
				if state == "loading" {
					app.viewRegistry[app.activeViewID()].Init()
				}
				for _, overlay := range []bool{false, true} {
					if overlay {
						dialogStep(&app, dialogs.OpenDialogMsg{Model: filterdialog.New()})
					}
					for _, size := range [][2]int{{120, 24}, {80, 10}, {80, 8}, {20, 10}, {1, 20}, {-1, -1}, {0, 0}, {120, 24}} {
						dialogStep(&app, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
						assertLayoutBounds(t, app.View().Content, size[0], size[1])
						if app.connectionError != nil {
							t.Fatal(app.connectionError)
						}
					}
					dialogStep(&app, dialogs.CloseDialogMsg{})
				}
				if populated && !strings.Contains(ansi.Strip(app.View().Content), "FixtureWorker") {
					t.Fatal("resizing lost periodic job")
				}
			})
		}
	}
}

type periodicLifetimeClient struct{ *lifetimeClient }

func (*periodicLifetimeClient) GetCapabilities() sidekiq.Capabilities {
	capabilities, _ := sidekiq.ResolveFeatures([]string{"ent"})
	return capabilities
}

func (c *periodicLifetimeClient) GetPeriodicJobs(ctx context.Context) ([]sidekiq.PeriodicJob, error) {
	return nil, c.block(ctx, "periodic list")
}

func (c *periodicLifetimeClient) GetPeriodicJob(ctx context.Context, _ string) (*sidekiq.PeriodicJob, []sidekiq.PeriodicExecution, error) {
	return nil, nil, c.block(ctx, "periodic detail")
}

func TestAppPeriodicReadLifetime(t *testing.T) {
	for name, detail := range map[string]bool{"list": false, "detail": true} {
		t.Run(name, func(t *testing.T) {
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			client := &periodicLifetimeClient{newLifetimeClient(t)}
			app := NewWithContext(parent, client, "", false, nil)
			defer app.cancel()
			cmd := app.setActiveView(viewPeriodic)
			if detail {
				cmd = dialogStep(&app, views.ShowPeriodicDetailMsg{LID: "fixture"})
			}
			done := make(chan tea.Msg, 1)
			go func() { done <- cmd() }()
			request := awaitLifetime(t, client.started)
			if request.ctx.Err() != nil {
				t.Fatal("read canceled before shutdown")
			}
			cancel()
			awaitLifetime(t, done)
			awaitLifetime(t, client.finished)
			if request.ctx.Err() != context.Canceled {
				t.Fatal("Periodic did not inherit application lifetime")
			}
		})
	}
}
