package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/dialogs"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
	helpdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/help"
	palettedialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/palette"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

func rateLimitsTestApp(t *testing.T, features []string) (App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	mr.HSet("lmtr-c-payments", "held", "7", "held_ms", "1200")
	_, _ = mr.Push("{lmtr-c-payments}-free", "slot-a", "slot-b")
	mr.HSet("lmtr-w-search", "size", "10", "interval", "30")
	mr.HSet("lmtr-b-api", "size", "20", "interval", "minute")
	mr.HSet("lmtr-k-drip", "size", "40", "interval", "20", "drops", "3", "hit", "10", "miss", "0")
	mr.HSet("lmtr-p-cost", "size", "1000", "interval", "20", "points", "850", "hit", "1")
	capabilities, _ := sidekiq.ResolveFeatures(features)
	client, err := sidekiq.NewClient("redis://"+mr.Addr(), sidekiq.WithFeatures(capabilities))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	app := New(client, "", false, nil)
	t.Cleanup(app.cancel)
	dialogStep(&app, tea.WindowSizeMsg{Width: 120, Height: 30})
	return app, mr
}

func TestAppRateLimitsPaletteOnlyAndEnterpriseGate(t *testing.T) {
	for name, features := range map[string][]string{"default": nil, "pro": {"pro"}, "enterprise": {"ent"}} {
		t.Run(name, func(t *testing.T) {
			app, _ := rateLimitsTestApp(t, features)
			want := name == "enterprise"
			for _, id := range []viewID{viewRateLimits, viewRateLimitDetail} {
				if _, registered := app.viewRegistry[id]; registered != want {
					t.Fatal("registry escaped Enterprise gate")
				}
				if slices.Contains(app.viewOrder, id) {
					t.Fatal("limiter view entered toolbar order")
				}
			}
			found := false
			for _, item := range app.paletteItems() {
				if item.Label == "Rate Limits" && item.ID == int(viewRateLimits) {
					found = true
				}
			}
			if found != want {
				t.Fatal("palette escaped Enterprise gate")
			}
			app.navbar.SetWidth(300)
			if strings.Contains(ansi.Strip(app.navbar.View()), "Rate Limit") {
				t.Fatal("limiter navbar entry")
			}
			expected := []viewID{viewDashboard, viewBusy, viewQueueDetails, viewRetries, viewScheduled, viewDead, viewErrorsSummary, viewMetrics}
			if want {
				expected = append(expected, viewPeriodic)
			}
			if !slices.Equal(expected, app.viewOrder) {
				t.Fatal("existing toolbar order changed")
			}
			for i, id := range expected {
				key := rune('1' + i)
				dialogStep(&app, tea.KeyPressMsg{Code: key, Text: string(key)})
				if app.activeViewID() != id {
					t.Fatalf("numeric binding %c changed", key)
				}
			}
			app.setActiveView(viewDashboard)
			cmd := app.navigatePalette(int(viewRateLimits))
			if !want {
				if cmd != nil || app.activeViewID() != viewDashboard {
					t.Fatal("disabled limiter navigated")
				}
				return
			}
			app = applyLayoutRead(app, cmd)
			if app.activeViewID() != viewRateLimits || len(app.viewStack) != 1 || app.connectionError != nil {
				t.Fatal("limiter root failed")
			}
		})
	}
}

func TestAppRateLimitsPaletteJourneyAndOwnedFilter(t *testing.T) {
	app, _ := rateLimitsTestApp(t, []string{"ent"})
	dialogStep(&app, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	for _, r := range "Rate Limits" {
		dialogStep(&app, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if app.activeViewID() != viewDashboard || app.dialogs.ActiveDialogID() != palettedialog.DialogID {
		t.Fatal("palette input escaped")
	}
	completion := dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})()
	app = applyLayoutRead(app, dialogStep(&app, completion))
	if app.activeViewID() != viewRateLimits || !strings.Contains(ansi.Strip(app.View().Content), "payments") {
		t.Fatal("palette did not open populated limiter root")
	}
	open := dialogStep(&app, tea.KeyPressMsg{Code: '/', Text: "/"})
	dialogStep(&app, open())
	for _, r := range "search1q" {
		dialogStep(&app, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if app.activeViewID() != viewRateLimits || app.dialogs.ActiveDialogID() != filterdialog.DialogID {
		t.Fatal("global navigation stole filter input")
	}
	app = applyLayoutRead(app, dialogStep(&app, dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})()))
	if !strings.Contains(ansi.Strip(app.View().Content), "No matching limiter names") {
		t.Fatal("filter did not reach owner")
	}
	app = applyLayoutRead(app, dialogStep(&app, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}))
	before := ansi.Strip(app.navbar.View())
	app = applyLayoutRead(app, dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter}))
	if app.activeViewID() != viewRateLimitDetail || len(app.viewStack) != 2 || app.viewStack[0] != viewRateLimits {
		t.Fatal("detail lost rate-limit root")
	}
	if before != ansi.Strip(app.navbar.View()) {
		t.Fatal("details changed toolbar")
	}
	app = applyLayoutRead(app, dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEscape}))
	if app.activeViewID() != viewRateLimits || len(app.viewStack) != 1 {
		t.Fatal("detail back navigation failed")
	}
	stale := dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})()
	dialogStep(&app, tea.KeyPressMsg{Code: '1', Text: "1"})
	app = applyLayoutRead(app, app.navigatePalette(int(viewRateLimits)))
	dialogStep(&app, stale)
	if app.activeViewID() != viewRateLimits || len(app.viewStack) != 1 {
		t.Fatal("old detail intent reopened after leaving and returning")
	}
}

func TestAppRateLimitsDetailIntentOwnership(t *testing.T) {
	for state, tc := range map[string]struct{ wantDetail bool }{
		"current":        {wantDetail: true},
		"refresh":        {wantDetail: true},
		"covered":        {},
		"covered-closed": {},
		"rediscovery":    {},
	} {
		for _, executeLate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/executeLate=%t", state, executeLate), func(t *testing.T) {
				app, _ := rateLimitsTestApp(t, []string{"ent"})
				app = applyLayoutRead(app, app.navigatePalette(int(viewRateLimits)))
				cmd := dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter})
				if cmd == nil {
					t.Fatal("selected limiter did not emit navigation")
				}
				var result tea.Msg
				if !executeLate {
					result = cmd()
				}
				switch state {
				case "refresh":
					app = applyLayoutRead(app, dialogStep(&app, views.RefreshMsg{}))
				case "covered", "covered-closed":
					dialogStep(&app, dialogs.OpenDialogMsg{Model: helpdialog.New()})
					if state == "covered-closed" {
						dialogStep(&app, dialogs.CloseDialogMsg{})
					}
				case "rediscovery":
					app = applyLayoutRead(app, dialogStep(&app, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}))
				}
				session := app.dialogs.ActiveSession()
				if executeLate {
					result = cmd()
				}
				dialogStep(&app, result)
				if tc.wantDetail {
					if app.activeViewID() != viewRateLimitDetail || len(app.viewStack) != 2 {
						t.Fatal("current limiter navigation was rejected")
					}
				} else if app.activeViewID() != viewRateLimits || len(app.viewStack) != 1 || app.dialogs.ActiveSession() != session {
					t.Fatal("stale limiter navigation changed screen or modal")
				}
			})
		}
	}
}

func TestAppRateLimitsLayoutAndReadOnly(t *testing.T) {
	for _, detail := range []bool{false, true} {
		app, mr := rateLimitsTestApp(t, []string{"ent"})
		app = applyLayoutRead(app, app.navigatePalette(int(viewRateLimits)))
		if detail {
			app = applyLayoutRead(app, dialogStep(&app, tea.KeyPressMsg{Code: tea.KeyEnter}))
		}
		before := mr.Dump()
		for _, size := range [][2]int{{140, 30}, {80, 24}, {40, 12}, {20, 7}, {3, 2}, {0, 0}, {120, 24}} {
			dialogStep(&app, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			assertLayoutBounds(t, app.View().Content, size[0], size[1])
			if !app.View().AltScreen {
				t.Fatal("terminal mode lost")
			}
		}
		for _, key := range []string{"d", "D", "p", "e", "x"} {
			app = applyLayoutRead(app, dialogStep(&app, tea.KeyPressMsg{Code: []rune(key)[0], Text: key}))
		}
		if before != mr.Dump() {
			t.Fatal("inspector mutated Redis")
		}
		open := dialogStep(&app, dialogs.OpenDialogMsg{Model: filterdialog.New()})
		if open != nil {
			app = applyLayoutRead(app, open)
		}
		assertLayoutBounds(t, app.View().Content, 120, 24)
	}
}

type limiterLifetimeClient struct{ *lifetimeClient }

func (*limiterLifetimeClient) GetCapabilities() sidekiq.Capabilities {
	features, _ := sidekiq.ResolveFeatures([]string{"ent"})
	return features
}
func (*limiterLifetimeClient) DisplayLimiterRedis() string { return "Isolated limiter pool" }
func (c *limiterLifetimeClient) ScanLimiters(ctx context.Context, _ sidekiq.LimiterCursor, _ string) (sidekiq.LimiterPage, error) {
	return sidekiq.LimiterPage{}, c.block(ctx, "limiter list")
}

func (c *limiterLifetimeClient) GetLimiter(ctx context.Context, _ string) (*sidekiq.Limiter, error) {
	return nil, c.block(ctx, "limiter detail")
}

func TestAppRateLimitsRootLifetime(t *testing.T) {
	client := &limiterLifetimeClient{newLifetimeClient(t)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	app := NewWithContext(ctx, client, "", false, nil)
	defer app.cancel()
	cmd := app.navigatePalette(int(viewRateLimits))
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	request := awaitLifetime(t, client.started)
	cancel()
	awaitLifetime(t, done)
	awaitLifetime(t, client.finished)
	if request.ctx.Err() != context.Canceled {
		t.Fatal("limiter pool request outlived root")
	}
}
