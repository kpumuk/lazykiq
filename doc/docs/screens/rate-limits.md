---
title: "Rate Limits"
description: "Check rate limiter usage, waiting, and recent activity."
summary: "Usage and statistics for Sidekiq Enterprise rate limiters."
date: 2026-11-03T00:00:00Z
lastmod: 2026-11-03T00:00:00Z
draft: false
weight: 130
toc: true
---

Rate Limits shows your Sidekiq Enterprise limiters, their capacity, and available
usage statistics. Use it to check whether a limiter is full, how often calls
have to wait, or how many attempts a bucket has received.

Start Lazykiq with Enterprise features enabled:

```sh
lazykiq --enable=ent
```

Press `Ctrl+t` and choose **Rate Limits**. This screen is read-only.
If your application uses a separate Redis connection for limiters, see
[Configuration]({{< relref "../getting-started/configuration.md" >}}#rate-limits).

## Find a limiter

The list shows each limiter's name, type, and capacity or rate. Concurrent
limiters also show used and free slots in **Usage / notes**. Warnings appear
there when a limiter's data cannot be read.

Press `/` and enter part of a limiter's name to filter the list. Names are
case-sensitive. Use `[` and `]` to browse result pages, then press `Enter` to
open a limiter. `Esc` returns to your place in the list.

The current page and open details refresh every five seconds. Press `r` to
refresh immediately, or `Ctrl+r` in the list to search again from the first page
and find newly added limiters.
An empty page can still have more results; follow the `] more` hint to continue.

## Concurrent usage

{{< lightbox src="assets/rate_limits.png" alt="Concurrent limiter with six of eight slots in use" >}}

A timed-out lock can remain in the used count until another call reclaims it.

| Metric | Meaning |
| --- | --- |
| Held count | Number of times a call acquired a slot. |
| Held duration | Total time spent holding slots, in seconds. |
| Immediate acquisitions | Calls that acquired a slot without waiting. |
| Waited count | Calls that had to wait for a slot. |
| Wait duration | Total time callers spent waiting, in seconds. |
| Overtime count | Operations that exceeded the limiter's lock timeout. |
| Reclaimed locks | Times another caller took over a timed-out lock. |

These totals accumulate across calls. Rising wait counts and durations indicate
callers are competing for slots. Overtime or reclaimed locks are a reason to check
whether operations are taking longer than the lock timeout allows.

## Window and bucket usage

Both types show the allowed number of operations and the interval. Window
limiters also show attempts in the current window and how long until it ends.

Bucket limiters show recent attempts in the **Bucket Counters** panel:

| Bucket interval | History shown |
| --- | --- |
| Second | Last 60 seconds |
| Minute | Last 60 minutes |
| Hour | Last 24 hours |
| Day | Last 30 days |

Counts include attempts that exceeded the limit, so a count can be higher than
the allowed size. The current interval is still in progress. On narrow terminals,
press `Tab` to switch between details and counters.

Set [the history timezone]({{< relref "../getting-started/configuration.md" >}}#rate-limits)
to match your application. If an hour repeats when daylight saving time ends,
**Shared calendar bucket (DST)** means both occurrences share one count.

## Leaky bucket and points usage

These types show burst capacity, the drain or refill interval, and the steady
rate per second. Hits count attempts allowed by the limiter; misses count
attempts that reached the limit. Wait duration is the total time callers spent
waiting, in seconds.

**Last bucket fill** shows how full a leaky bucket was at its last update.
**Last points balance** shows how many points remained at the last update.
Check **Last limiter update** for the time: buckets drain and points refill
between calls, so these values may differ from what is available now.

## Missing data

**Unavailable** means Sidekiq has no value to show, or the value has expired.
It does not mean zero. Some statistics are only available for certain limiter
types, and older bucket counts may no longer be available.

**Disappeared** means the limiter is no longer available. Press `Esc` to return
to the list, then `Ctrl+r` to update the results.

## Key bindings

| Key | Action |
| --- | --- |
| `/` | Filter by name. |
| `Ctrl+u` | Clear the name filter. |
| `[` / `]` | Previous / next results page. |
| `r` | Refresh the current page or details. |
| `Ctrl+r` | Restart the search for limiters; refresh when viewing details. |
| `Enter` | Open the selected limiter. |
| `Esc` | Return to the list. |
| `c` / `C` | Copy the limiter name / Redis key. |
| `Tab` / `Shift+Tab` | Switch between bucket details and counters. |
| `Up` / `k`, `Down` / `j` | Move up or down. |
| `PgUp` / `PgDn` | Scroll a page at a time. |
| `g` / `G` | Jump to the first / last row. |
| `Left` / `h`, `Right` / `l` | Scroll horizontally. |
| `Home` / `0`, `End` / `$` | Scroll to the first / last column. |
| `?` | Show help. |

See the [Sidekiq Enterprise rate limiting documentation](https://github.com/sidekiq/sidekiq/wiki/Ent-Rate-Limiting)
for help choosing and configuring limiters in your application.
