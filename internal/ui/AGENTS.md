# UI conventions

## Where to look

- `app.go`: view registry and stack, refresh cycle, dialog routing, and application layout.
- `views/view.go`: view interface, messages, and optional lifecycle interfaces.
- `components/`: reusable rendering and interaction; `dialogs/`: modal inputs and overlays.
- `theme/theme.go`: palette; `display/`: text formatting; `requestctx/`: cancelable view requests.

## Components and styles

Follow the Bubbles component pattern. Use [components/messagebox/messagebox.go](components/messagebox/messagebox.go) as a small example and [components/frame/frame.go](components/frame/frame.go) for focus-aware styles.

- Keep model state unexported and expose setters and getters.
- Provide package-local `Styles`, `DefaultStyles()`, `WithStyles()`, and `SetStyles()`; compose styles from `theme.NewStyles()` at app/view boundaries.
- Initialize defaults in `New(opts ...Option)` and apply functional options afterward so components work without explicit styling.
- Reusable component `Update()`, `View()`, and getters use value receivers; setters use pointer receivers. Preserve the existing pointer-based lifecycle of application views.
- Create styles outside render paths and pass them through the styles struct; do not call `lipgloss.NewStyle()` while rendering.
- Keep color values in `theme.DefaultTheme`. Resolve light/dark `ColorPair` values in app-owned state from Bubble Tea background replies, with a dark fallback. Never query the terminal during palette initialization.
- Use `components/frame` for borders with titles or metadata and `messagebox` for centered empty states.
- Keep tables in `components/table` to avoid import cycles between components and views.
- Keep UI text clean and professional; use Nerd Font glyphs sparingly and avoid emojis.

## Views and data flow

- Implement `views.View`; use `views.Disposable` to clean up transient state when a view is popped.
- Views that open dialogs embed `dialogs.Opener` and call `OpenDialog` before composing commands. The app supplies the owner before `Init`/`Update`; never read mutable ownership inside an asynchronous command or create an unowned view dialog.
- Emit view-local navigation through `IntentScope.EmitIntent` before composing commands, including `ShowJobDetailMsg` and `ShowErrorDetailsMsg` for stacked detail views. Let `app.go` manage the stack.
- The app's five-second ticker fetches stats and sends `views.RefreshMsg` to the active view. Views fetch their own data; Dashboard also has a realtime ticker.
- `stats.UpdateMsg` updates the metrics bar and clears the connection error. Stats fetch errors and `views.ConnectionErrorMsg` can show the error overlay.
- Route modal input through the dialog manager before app-level key handling. The app checks `a.dialogs.HasDialogs()`; keep filter input in the dialog flow.

## Rendering constraints

- Apply horizontal offsets before styling plain text. Use ANSI-aware helpers for styled text; never slice escape sequences by byte offset.
- Clamp horizontal and vertical scroll offsets when data or dimensions change.
- Leave the table's last column untruncated and unpadded so variable content can scroll horizontally.
- Prefer line slicing for manual vertical scrolling in tables with selection.
- Set text input width so placeholders render fully.
- Keep chart colors aligned with `theme.DefaultTheme`.
- The app renders the metrics bar, context bar, active view, stack bar, and navigation bar. `resizeViews()` subtracts the app-owned bars before calling view `SetSize()`; keep view output within that allocation across normal, empty, and detail states.
- Frame titles occupy the top border line. Subtract two lines for borders, without another line for the title.

## UI tests

Combine behavioral tests with golden tests for layout. Behavioral tests should check dimensions, content, zero sizes, disabled items, truncation, and scrolling.

Add or update golden tests for components with layout requirements. Prefix them with `TestGolden`, strip ANSI before comparison, and cover representative empty states, columns, widths, and realistic content:

```go
output := ansi.Strip(m.View())
golden.RequireEqual(t, []byte(output))
```

Update only the affected package after an intentional visual change, then inspect the golden diff:

```bash
go test ./internal/ui/components/frame -update
```
