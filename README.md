# lazykiq

<p align="center"><img width="512" src="./doc/assets/lazykiq.png" alt="lazykiq logo" /></p>

<p align="center">
  A rich terminal UI for Sidekiq.
  <br />
  <br />
  <a href="https://github.com/kpumuk/lazykiq/releases"><img src="https://img.shields.io/github/release/kpumuk/lazykiq.svg" alt="Latest Release"></a>
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

![lazykiq demo](./doc/assets/demo.gif)

## Usage

> [!TIP]
> Latest installation instructions for different platforms are available on the [Lazykiq website](https://kpumuk.github.io/lazykiq/docs/getting-started/installation/).

You can download the latest release from the [Releases](https://github.com/kpumuk/lazykiq/releases) page for your platform.

Alternatively, install the current development version with `go install`:

```bash
go install github.com/kpumuk/lazykiq/cmd/lazykiq@latest
```

### Keys

- `1-7` - switch views
- `j` / `k` - navigate down / up (or `Down` / `Up`)
- `Enter` - view job details, `Esc` to close
- `[` / `]` - previous / next page (switch interval on the dashboard)
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

To update all dependencies:

```bash
mise run deps
```

### Git hooks

Lefthook can be used to setup Git hooks:

```bash
lefthook install
```

This will ensure we do not push any dependencies with known vulnerabilities, and the code quality matches our standards.

### Test environment

There is a test environment prepared in the `demo/` directory. Simply start it with:

```bash
docker-compose up --build
```

This will:

- Start a Redis server
- Start a Sidekiq server with some demo jobs
- Start a web server to monitor Sidekiq

You can access the Sidekiq dashboard at `http://localhost:9292` and connect `lazykiq` to Redis at `localhost:6379`.

### Website

The documentation website is built with [Hugo](https://gohugo.io/). To run it locally:

```bash
mise run website-dev
```

## Feedback

I’d love to hear your thoughts on this project. Feel free to drop a note!

- [Twitter](https://twitter.com/kpumuk)
- [The Fediverse](https://ruby.social/@kpumuk)

## License

[MIT](https://github.com/kpumuk/lazykiq/raw/main/LICENSE).
