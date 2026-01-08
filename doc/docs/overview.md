---
title: "Overview"
description: "What Lazykiq helps you do."
summary: "What Lazykiq helps you do."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-08T00:00:00Z
draft: false
weight: 15
toc: true
---

Lazykiq helps you monitor Sidekiq from the terminal with fast navigation and
clear summaries.

## What you can do

- View Sidekiq processes and currently running jobs.
- Explore queues and inspect job payloads.
- Inspect job arguments and error backtraces.
- Review retries, scheduled, and dead jobs.
- Analyze errors across retry and dead queues.

## Global shortcuts

| Key            | Description                                                                        |
|----------------|------------------------------------------------------------------------------------|
| `1`–`8`        | Switch views (Dashboard, Busy, Queues, Retries, Scheduled, Dead, Errors, Metrics). |
| `?`            | Toggle the help dialog.                                                            |
| `q` / `Ctrl+C` | Quit.                                                                              |
| `Esc`          | Go back from stacked views (job details, queue list, job metrics).                 |
| `F12` / `~`    | Toggle dev console (requires `--development`).                                     |

## Screenshots

{{< lightbox src="assets/dashboard.png" alt="Dashboard view" >}}
