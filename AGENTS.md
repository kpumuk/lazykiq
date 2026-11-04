# Lazykiq

Bubble Tea terminal UI for Sidekiq monitoring. The Go version, toolchain, and dependencies are defined in [go.mod](go.mod).

## Development

Install development tools and build the app from the repository root:

```bash
mise install
mise run build
```

For interactive verification, start the demo with Docker and Compose available, then launch the UI:

```bash
mise run demo
go run ./cmd/lazykiq --redis redis://localhost:6379/0
```

See [demo/README.md](demo/README.md) for version-specific environments and [README.md](README.md#development) for other development tasks. Task definitions live in [mise.toml](mise.toml).

## Validation and completion

After code changes, run the full checks before handoff:

```bash
mise run ci   # format, test, lint, and check GitHub workflows
```

The formatter can modify files; inspect the resulting diff. Read-only reviews do not require CI. For documentation-only changes, verify the commands, paths, and links.

Useful focused commands:

```bash
mise run fmt
mise run test
mise run lint
mise run lint --enable-only modernize
mise run lint --enable-only modernize --fix
```

Work is complete when the requested behavior is verified, relevant regression tests are added or updated, user-facing documentation reflects behavior changes, and the final diff has been reviewed for unintended changes. Report the checks performed and any failures or skipped checks with their exact blockers.

## Layout and conventions

- `cmd/lazykiq/main.go`: entry point; `internal/cmd/root.go`: CLI setup.
- `internal/sidekiq/`: Redis access, Sidekiq models, metrics, and job operations.
- `internal/ui/`: application, views, components, dialogs, and theme. Follow [internal/ui/AGENTS.md](internal/ui/AGENTS.md) for UI work.
- `demo/`: Redis and Sidekiq environments for interactive verification.
- `doc/`: documentation content and assets; `website/`: Hugo site and build tooling.

Do not introduce runtime reflection (`reflect.ValueOf`, `reflect.TypeOf`, or reflective traversal). Use explicit interfaces, typed helpers, or type switches. Capture ownership and other metadata at creation rather than inspecting private framework types after composition.

Prefer `mathutil.Clamp` for bounds clamping. Filtered sorted-set scans use `ZSCAN`; sort matches by score afterward: dead jobs newest first, retry and scheduled jobs earliest first.

## Testing

Use table-driven behavioral tests with `map[string]struct{ ... }`. Check normal behavior and edge cases; use `strings.Contains()` for content checks where exact positioning is irrelevant. UI layout changes also require golden coverage as described in [internal/ui/AGENTS.md](internal/ui/AGENTS.md).

## Git and pull requests

Use flat branch names, without prefixes such as `feature/`, `perf/`, or `fix/`, unless explicitly requested.

Use [create-pull-request](.agents/skills/create-pull-request/SKILL.md) when preparing, raising, or updating a pull request. It defines PR prose, bug reproductions, VHS visual evidence, and optimization benchmarks.
