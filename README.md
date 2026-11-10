# lazykiq

<p align="center"><img width="512" src="./doc/assets/lazykiq.png" alt="lazykiq logo" /></p>

<p align="center">
  A rich terminal UI for Sidekiq.
  <br />
  <br />
  <a href="https://github.com/kpumuk/lazykiq/releases"><img src="https://img.shields.io/github/release/kpumuk/lazykiq.svg" alt="Latest Release"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/kpumuk/lazykiq"><img src="https://api.scorecard.dev/projects/github.com/kpumuk/lazykiq/badge" alt="OpenSSF Scorecard"></a>
  <a href="https://github.com/kpumuk/lazykiq/actions"><img src="https://github.com/kpumuk/lazykiq/workflows/test/badge.svg" alt="Build Status"></a>
  <a href="https://github.com/kpumuk/lazykiq/blob/main/LICENSE"><img alt="GitHub License" src="https://img.shields.io/github/license/kpumuk/lazykiq"></a>
</p>

- View Sidekiq processes and currently running jobs
- Explore Sidekiq queues and jobs
- Inspect job arguments and error backtraces
- View Sidekiq retries, scheduled, and dead jobs
- Analyze errors in dead and retry queues
- Check job execution metrics
- ... more to come!

[![Watch the lazykiq demo](./doc/assets/dashboard.png)](./doc/assets/demo.mp4)

[Watch the demo (MP4)](./doc/assets/demo.mp4).

## Usage

> [!TIP]
> Latest installation instructions for different platforms are available on the [Lazykiq website](https://kpumuk.github.io/lazykiq/docs/getting-started/installation/).

> [!NOTE]
> Lazykiq uses Unicode box-drawing and chart glyphs in the UI; a modern UTF-8 terminal font is recommended.

You can download the latest release from the [Releases](https://github.com/kpumuk/lazykiq/releases) page for your platform.

Alternatively, install the current development version with `go install`:

```bash
go install github.com/kpumuk/lazykiq/cmd/lazykiq@latest
```

### Keys

- `1-8` - switch views
- `j` / `k` - navigate down / up (or `Down` / `Up`)
- `h` / `l` - navigate left / right (or `Left` / `Right`)
- `Enter` - view job details, `Esc` to close
- `[` / `]` - previous / next page
- `{` / `}` - switch interval on the dashboard or metrics
- `/` - filter job list (case-sensitive)
- `q` - quit

### Redis

Connect to a specific Redis instance with:

```bash
lazykiq --redis redis://localhost:6379/0
```

## Development

We use [`mise`](https://mise.jdx.dev/) for development. Install tooling with:

```bash
mise install
```

Run all CI tasks with:

```bash
mise run ci
```

Build the app locally with:

```bash
mise run build
```

Build a smaller release binary with:

```bash
mise run build-release
```

### Memory-constrained development

Mise defaults to one task or tool installation at a time and sets `GOMEMLIMIT=400MiB` for Go programs it runs. This keeps cold builds, linting, and tests from competing for memory in a 2 GB development environment. `GOMEMLIMIT` is a soft target for each Go process, not a limit on total memory or process RSS. These settings do not change the compiled app's runtime defaults.

For Go commands outside Mise, set the same target explicitly:

```bash
GOMEMLIMIT=400MiB go test ./...
```

On a machine with more memory, override the defaults with `GOMEMLIMIT=1GiB mise --jobs 4 run ci`. Avoid overlapping separate cold validation runs; each invocation has its own task limit. Run only the demo versions you need, and stop unused demo services before cold validation or browser-based terminal captures.

CodeQL analysis runs separately in GitHub Actions, not in `mise run ci`. It is not covered by the 2 GB development budget: GitHub [recommends at least 8 GB for small codebases](https://docs.github.com/en/code-security/code-scanning/creating-an-advanced-setup-for-code-scanning/recommended-hardware-resources-for-running-codeql), and the [query evaluator rounds memory targets below 2048 MB up](https://docs.github.com/en/code-security/codeql-cli/codeql-cli-manual/resolve-ram). Run full CodeQL analysis on the existing hosted runners rather than alongside local development tools.

### Documentation captures

With VHS, FFmpeg, the JetBrains Mono Nerd Font, and the Sidekiq 8.0 demo installed and running, regenerate the documentation media with:

```bash
mise run demo-record
```

The task builds the app before starting the browser-based capture, then writes screenshots and `doc/assets/demo.mp4` at the full 2400 × 1600 resolution. MP4 avoids the frame buffering used by VHS's GIF palette generation, which can exceed the 2 GB development budget. The README links a static dashboard preview to the video.

Documentation images and video use Git LFS. Install [Git LFS](https://git-lfs.com/) and run `git lfs pull` to download the media after cloning.

### Release Process

Releases are published from signed `v*` tags through the GitHub Actions release workflow.

- Only release tags matching `v*` should be used to trigger a release.
- Release artifacts are built by GoReleaser, include SBOMs, and ship with signed checksums plus GitHub artifact attestations.
- The maintainer is responsible for creating release tags and for reviewing release-environment secrets and publishing configuration before a release.

To update all dependencies:

```bash
mise run deps
```

### Git hooks

Lefthook can be used to setup Git hooks:

```bash
lefthook install
```

The pre-push hooks check Go dependencies and all demo Ruby lockfiles (`Gemfile.lock` and `Gemfile.sidekiq-*.lock`) for known vulnerabilities. The pre-commit hooks check code quality.

### Test environment

There is a test environment prepared in the `demo/` directory. Simply start it with:

```bash
mise run demo
mise run demo-sidekiq73
mise run demo-sidekiq80
mise run demo-sidekiq81
```

This will:

- Start a Redis server
- Start one Sidekiq demo stack for the selected version, with demo jobs and the Web interface

| Command                   | Sidekiq   | Redis URL                  | Web Dashboard         |
| ------------------------- | --------- | -------------------------- | --------------------- |
| `mise run demo`           | **8.1.0** | `redis://localhost:6379/0` | http://localhost:9292 |
| `mise run demo-sidekiq73` | **7.3.x** | `redis://localhost:6379/2` | http://localhost:9294 |
| `mise run demo-sidekiq80` | **8.0.0** | `redis://localhost:6379/1` | http://localhost:9293 |
| `mise run demo-sidekiq81` | **8.1.0** | `redis://localhost:6379/0` | http://localhost:9292 |

The version-specific tasks can run in parallel because each demo stand uses its own Redis database and dashboard port.

You can connect to a specific Sidekiq version using:

```bash
go run ./cmd/lazykiq --redis redis://localhost:6379/1
```

### Website

The documentation website uses [Hugo](https://gohugo.io/) and Dart Sass. Install the build tools, then start the site:

```bash
mise install
mise run website-dev
```

## Feedback

I’d love to hear your thoughts on this project. Feel free to drop a note!

- [Twitter](https://twitter.com/kpumuk)
- [The Fediverse](https://ruby.social/@kpumuk)

## License

[MIT](https://github.com/kpumuk/lazykiq/raw/main/LICENSE).
