---
title: "Configuration"
description: "Configure Lazykiq flags and Redis connection."
summary: "Configure Lazykiq flags and Redis connection."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-08T00:00:00Z
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
  -h --help      help for lazykiq
  --redis        redis URL (redis://localhost:6379/0)
  -v --version   version for lazykiq
```

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

## Dangerous actions

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
