# Lazykiq - Claude Context

## What
Bubble Tea TUI for Sidekiq monitoring. Go 1.25.

## Structure
```
cmd/lazykiq/main.go          - entry point
internal/
  sidekiq/
    client.go                - Redis client, GetStats(), uses go-redis with VoidLogger
  ui/
    app.go                   - main model, renderBorderedBox() for titled border
    keys.go                  - KeyMap struct, DefaultKeyMap()
    theme/theme.go           - Theme (Dark/Light), Styles, NewStyles()
    components/
      metrics.go             - top bar: Processed|Failed|Busy|Enqueued|Retries|Scheduled|Dead
      navbar.go              - bottom bar: view keys + quit + theme
      error_popup.go         - centered overlay for connection errors
    views/
      view.go                - View interface, views.Styles{Text, Muted}
      dashboard.go, queues.go, busy.go, retries.go, scheduled.go, dead.go
```

## Patterns
- Views implement `views.View` interface
- Components take `*theme.Styles`, have SetStyles()
- Theme toggle: `t` key, app.applyTheme() propagates to all
- Border title: renderBorderedBox() in app.go
- No backgrounds on metrics/navbar (transparent)
- NO EMOJIS in UI - keep text clean and professional

## Data Flow
- 5-second ticker fetches Sidekiq stats from Redis
- MetricsUpdateMsg updates metrics bar
- connectionErrorMsg shows error popup overlay
- Error popup auto-clears when Redis reconnects

## Keys
1-6: views, t: theme, q: quit, tab/shift+tab: reserved

## Dependencies
bubbletea, lipgloss, bubbles/key, go-redis/v9
