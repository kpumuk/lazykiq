---
title: "Configuration"
description: "Configure Lazykiq flags and Redis connection."
summary: "Configure Lazykiq flags and Redis connection."
date: 2025-12-30T00:00:00Z
lastmod: 2026-10-29T00:00:00Z
draft: false
weight: 25
toc: true
---

## CLI help

```text
A terminal UI for Sidekiq.

USAGE
  lazykiq [--flags]

FLAGS
  --cpuprofile   write cpu profile to file
  --danger       enable dangerous operations
  --development  enable development diagnostics
  --enable       enable optional features (pro, ent)
  --periodic-timezone  server timezone for periodic jobs without an explicit zone (UTC)
  -h --help      help for lazykiq
  --redis        redis URL (redis://localhost:6379/0)
  -v --version   version for lazykiq
```

## Shell completion

Generate completion scripts with `lazykiq completion bash`, `lazykiq completion zsh`,
`lazykiq completion fish`, or `lazykiq completion powershell`.

For the current Zsh session, run this after `compinit`:

```zsh
source <(lazykiq completion zsh)
```

For Bash with `bash-completion` installed:

```bash
source <(lazykiq completion bash)
```

Completion suggests flag names and available `--enable` features, including
comma-separated and repeated selections. The `--cpuprofile` flag uses file
completion. Run `lazykiq completion <shell> --help` for installation instructions.

## Connect to Redis

Use the `--redis` flag with a Redis URL. The default is
`redis://localhost:6379/0`.

```bash
lazykiq --redis redis://localhost:6379/0
```

Point to another host or database index by changing the URL:

```bash
lazykiq --redis redis://redis.internal:6379/2
```

## Optional features

Use `--enable=pro` for Sidekiq Pro or `--enable=ent` for Sidekiq Enterprise,
which includes Pro features. Optional features are disabled by default.
Multiple features can be comma-separated or supplied with repeated `--enable` flags.

```bash
lazykiq --enable=pro
lazykiq --enable=ent --danger
```

Enterprise adds [Periodic]({{< relref "../screens/periodic.md" >}}) at `9`.
Enqueue and pause/resume require `--danger`. Set `--periodic-timezone` to your
Sidekiq server's timezone for definitions without an explicit timezone; it
defaults to UTC because Enterprise does not store the server timezone in Redis.

```bash
lazykiq --enable=ent --periodic-timezone=America/Toronto
```

## Dangerous actions

{{< callout context="danger" title="Danger" icon="outline/alert-octagon" >}}
For limitations and caveats, see [Danger Mode]({{< relref "danger-mode.md" >}}).
{{< /callout >}}

Lazykiq is read-only by default. To enable actions that *change* Sidekiq state,
start it with `--danger` (alias: `--yolo`). This enables mutation operations
such as deleting jobs from retry/scheduled/dead sets, killing retries (move to
dead), deleting queues, and pausing or stopping processes. Use this flag only
when you are comfortable writing to your Sidekiq Redis instance.

```bash
lazykiq --redis redis://localhost:6379/0 --danger
```

Dangerous actions always require confirmation. Use `y`/`n`, `Enter`, or `Esc`
to confirm or cancel; `Tab`/`Shift+Tab` switches between buttons.

## Development diagnostics

Use `--development` only when debugging Lazykiq itself. This enables the
internal dev console and extra diagnostics that are not intended for regular
day-to-day monitoring. Toggle it in the UI with `F12` or `~`.
