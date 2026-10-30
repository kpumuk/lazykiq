---
title: "Periodic"
description: "Inspect and control Enterprise recurring jobs."
summary: "Recurring schedules, pause state, and enqueue history."
date: 2026-10-30T00:00:00Z
lastmod: 2026-10-30T00:00:00Z
draft: false
weight: 120
toc: true
---

Periodic jobs are Sidekiq Enterprise recurring cron definitions. Enable
`--enable=ent` to add the Periodic screen at `9`. The Scheduled screen at `5`
continues to show individual delayed jobs.

```sh
lazykiq --enable=ent --periodic-timezone=America/Toronto
```

The list shows the class, cron expression, pause state, queue, next occurrence,
and registered options. Filtering matches literal, case-sensitive substrings in
the class, schedule, options, loop ID, and explicit timezone.

| Key | Description |
| --- | --- |
| `9` | Go to Periodic (requires `--enable=ent`). |
| `Up` / `k`, `Down` / `j` | Select a schedule. |
| `Enter` | Open the selected definition. |
| `/` | Filter definitions. |
| `Ctrl+u` | Clear the filter. |
| `Left` / `h`, `Right` / `l` | Scroll the table horizontally. |
| `[` / `]`, `PgUp` / `PgDn`, `Alt+Left` / `Alt+Right` | Page up or down. |
| `g` / `G` | Jump to the first or last row. |
| `Home` / `0`, `End` / `$` | Scroll to the first or last column. |
| `c` | Copy the selected loop ID. |
| `r` | Refresh definitions. |
| `e` | Enqueue one job now (requires `--danger` and confirmation). |
| `p` | Pause or resume future recurring enqueues (requires `--danger` and confirmation). |

## Definition details

The left panel shows the class, schedule, state, queue, timezone, next occurrence,
loop ID, and options. The right panel shows the latest 25 enqueue events, newest
first, with UTC timestamps and JIDs. At narrow widths only the focused panel is
shown; use `Tab` to switch between information and history.

History comes from Enterprise's scheduler. It records enqueue events, without
success or failure outcomes. Manual enqueues do not add history entries, matching
Enterprise's web action. Jobs can be enqueued manually while their schedule is paused.

| Key | Description |
| --- | --- |
| `Tab` / `Shift+Tab` | Switch between information and history. |
| `Up` / `k`, `Down` / `j` | Scroll information or select a history row. |
| `[` / `]`, `PgUp` / `PgDn`, `Alt+Left` / `Alt+Right` | Page the focused panel. |
| `g` / `G` | Jump to the beginning or end of the focused panel. |
| `Home` / `End` | Jump to the beginning or end of information. |
| `c` | Copy the loop ID, or the selected history JID when history is focused. |
| `r` | Refresh the definition and history. |
| `e` / `p` | Enqueue now or pause/resume, with the same confirmations as the list. |
| `Esc` | Return to the list and refresh it. |

## Timezones and enqueue behavior

Explicit IANA and Rails timezone names take precedence. For definitions without
an explicit zone, set `--periodic-timezone` to the timezone of your Sidekiq
server. Its default is UTC; Enterprise does not publish that server setting in
Redis. Next-run times are calculated locally from cron expressions and timezone rules.

Pause/resume affects future recurring enqueues. Jobs already queued or running
remain unaffected. Actions reject definitions removed or reconfigured after
they were displayed.

Enqueue now submits the registered options directly to Redis, supplying empty
arguments, queue `default`, and retry enabled when omitted. Ruby class defaults,
client middleware, and ActiveJob adapters cannot run in Lazykiq. Use explicit
queue, args, and retry options in recurring definitions intended for manual
enqueue here; use Sidekiq Web or application Ruby tooling when those hooks are
required. See [Danger Mode]({{< relref "../getting-started/danger-mode.md" >}}).
