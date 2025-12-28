# lazykiq

<p align="center"><img width="512" src="./doc/assets/lazykiq.png" alt="lazykiq logo" /></p>

A terminal UI for Sidekiq.

* View Sidekiq processes and currently running jobs
* Explore Sidekiq queues and jobs
* Inspect job arguments and error backtraces
* View Sidekiq retries, scheduled, and dead jobs
* ... more to come!

![lazykiq demo](./doc/assets/demo.gif)

## Usage

To install, run:

```bash
go install github.com/kpumuk/lazykiq@latest
```

### Keys

- `1-6` - switch views
- `j` / `k` - navigate down / up (or `Down` / `Up`)
- `Enter` - view job details, `Esc` to close
- `[` / `]` - previous / next page (switch interval on the Dashboard)
- `/` - filter job list (case-sensitive)
- `q` - quit

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

### Test environment

There is a test environment prepared in the `demo/` directory. Simply start it with:

```bash
docker-compose up --build
```

This will:

* Start a Redis server
* Start a Sidekiq server with some demo jobs
* Start a web server to monitor Sidekiq

You can access the Sidekiq dashboard at http://localhost:9292 and connect `lazykiq` to Redis at `localhost:6379`.
