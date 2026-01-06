# Lazykiq - Agents Context

## What

Bubble Tea TUI for Sidekiq monitoring. Go 1.25.

## Code Quality

- Make sure to always perform `mise run ci` to ensure code formatting matches the style of the repository, linters don't have any offenses, and tests pass.

```bash
mise run ci   # run formatter, tests, linter
mise run fmt  # format code
mise run test # run tests
mise run lint # run all linters
# only focus on modernize linter
mise run lint --enable-only modernize
# address all auto-correctable issues
mise run lint --enable-only modernize --fix
```

## Structure

```text
cmd/lazykiq/main.go          - CLI entry point
internal/cmd/root.go         - cobra/fang CLI wiring, runs UI
internal/
  sidekiq/
    client.go                - Redis client, stats + busy/queues APIs
    dashboard.go             - Redis INFO + stats history/realtime helpers
    metrics.go               - Sidekiq Pro metrics rollups + histograms
    job.go, queue.go          - job + queue models/parsers
    sorted.go                - dead/retry/scheduled sorted-set helpers
  ui/
    app.go                   - main model, view stack + stackbar, tick + RefreshMsg, error overlay
    keys.go                  - KeyMap struct, DefaultKeyMap()
    theme/theme.go           - Theme (Dark/Light), Styles, NewStyles(), stackbar styles
    components/
      errorpopup/            - centered overlay for connection errors
      filterinput/           - /-activated filter input + ActionMsg
      frame/                 - titled bordered box with optional meta (replaces renderBorderedBox)
      jsonview/              - syntax-highlighted JSON renderer
      messagebox/            - centered empty-state box
      metrics/               - top bar: Processed|Failed|Busy|Enqueued|Retries|Scheduled|Dead
      navbar/                - bottom bar: view keys + quit
      stackbar/              - breadcrumb for stacked views
      table/                 - reusable scrollable table with selection
    format/format.go         - Duration, Bytes, Args, Number formatters
    views/
      view.go                - View interface + msgs + Disposable/Setter interfaces
      dashboard.go, queues.go, busy.go, retries.go, scheduled.go, dead.go
      errors_summary.go, errors_details.go, errors_data.go
      jobdetail.go           - stacked job detail view
```

## Patterns

- Views implement `views.View` interface
- Components take `*theme.Styles`, have SetStyles()
- Theme uses AdaptiveColor; no runtime toggle
- All color values must live in `theme.DefaultTheme`; no inline colors outside it (dashboard charts are the only temporary exception while ntcharts uses lipgloss v1)
- Border title/meta: use `components/frame` (title is on the top border line)
- No emojis in UI except strategically placed glyphs from Nerd Font, such as "⚙" - keep text clean and professional
- Shared components: no lipgloss.NewStyle() calls in render paths - pass all styles via struct/DefaultStyles
- Table in `components/table/` subpackage to avoid import cycle (components ↔ views)
- Table: last column not truncated/padded to allow horizontal scroll of variable content
- Empty states: prefer `messagebox` for centered messages
- Detail views are stacked: emit `ShowJobDetailMsg`/`ShowErrorDetailsMsg` and let `app.go` push views
- Views with transient state should implement `views.Disposable` for cleanup when popped
- App keeps a view registry + stack; `viewOrder` drives navbar ordering and view hotkeys

## Component Pattern (bubbles-style)

Follow the charmbracelet/bubbles pattern for reusable components:

```go
// 1. Styles struct - exported, holds all styles
type Styles struct {
    Title  lipgloss.Style
    Border lipgloss.Style
}

// 2. DefaultStyles() - returns sensible defaults
func DefaultStyles() Styles {
    return Styles{
        Title:  lipgloss.NewStyle().Bold(true),
        Border: lipgloss.NewStyle(),
    }
}

// 3. Model struct - holds all state
type Model struct {
    styles  Styles  // unexported, use SetStyles()
    width   int
    height  int
    // ... other state
}

// 4. Option type for functional options
type Option func(*Model)

// 5. New() constructor with functional options
func New(opts ...Option) Model {
    m := Model{
        styles: DefaultStyles(),
        // ... defaults
    }
    for _, opt := range opts {
        opt(&m)
    }
    return m
}

// 6. WithXxx() option functions
func WithStyles(s Styles) Option {
    return func(m *Model) { m.styles = s }
}

func WithSize(w, h int) Option {
    return func(m *Model) { m.width, m.height = w, h }
}

// 7. SetXxx() methods - pointer receiver, for post-creation updates
func (m *Model) SetStyles(s Styles) { m.styles = s }
func (m *Model) SetSize(w, h int)   { m.width, m.height = w, h }

// 8. Getter methods - value receiver
func (m Model) Width() int  { return m.width }
func (m Model) Height() int { return m.height }

// 9. Update() - value receiver, returns new Model + Cmd (if interactive)
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    // handle messages
    return m, nil
}

// 10. View() - value receiver, renders to string
func (m Model) View() string {
    // render
}
```

Key principles:

- **Value receivers** for `Update()`, `View()`, getters (immutable operations)
- **Pointer receivers** for `SetXxx()` methods (mutations)
- **Unexported state** accessed via `SetXxx()`/getters
- **`DefaultStyles()`** so components work without explicit styling
- **Functional options** for clean initialization
- **No `lipgloss.NewStyle()`** in render methods - styles passed in

## Data Flow

- 5-second ticker fetches Sidekiq stats for metrics + broadcasts `views.RefreshMsg` to the active view
- Views fetch their own data on `RefreshMsg` (Dashboard has its own realtime ticker)
- `metrics.UpdateMsg` updates metrics bar
- `connectionErrorMsg`/`views.ConnectionErrorMsg` show error popup overlay
- Error popup auto-clears on successful metrics update
- `ShowJobDetailMsg`/`ShowErrorDetailsMsg` push stacked views; Esc pops back; stackbar renders the breadcrumb

## Keys

1-7: views, q: quit, tab/shift+tab: reserved, ?: help, esc: pop stacked view

## Dependencies

* charm.land/bubbletea/v2
* charm.land/lipgloss/v2
* charm.land/bubbles/v2
* github.com/charmbracelet/x/ansi
* go-redis/v9
* cobra/fang
* ntcharts
* chroma

## Gotchas

- Horizontal scroll: apply offset to plain text BEFORE lipgloss styling. Slicing ANSI-escaped strings breaks escape sequences.
- Scroll state: clamp xOffset/yOffset when data or dimensions change (new data may have different max width)
- Manual vertical scroll (line slicing) is simpler than bubbles/viewport for tables with selection
- Filtered sorted-set scans use ZSCAN; always sort matches by score to preserve chronological order (dead: newest first; retry/scheduled: earliest first).
- Textinput placeholder rendering needs Width set; otherwise only the first placeholder rune appears.
- Views with filter inputs should expose `FilterFocused() bool`; the app checks this to route keys before global shortcuts.
- When an input component is focused, the app must route key events to the view before global shortcuts to avoid stealing keys (view switch, quit).
- Dashboard charts use ntcharts (patched to use lipgloss v2); keep colors aligned with `theme.DefaultTheme`.
- Height calculations: app.go renders metrics bar (top) + view content + stackbar + navbar (bottom). Views must output exactly the same number of lines consistently. If view outputs too many lines, metrics bar gets pushed off screen. Specific issues:
  - Title is part of the border line, not a separate line (so -2 for borders, not -3)
  - Views with header areas outside the main box (Busy, Queues) get extra height (+3 instead of +2). When showing alternative content (like job detail), must output the same total lines as normal view - add empty lines at top if needed to match the header area
