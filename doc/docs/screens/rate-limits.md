---
title: "Rate Limits"
description: "Inspect Sidekiq Enterprise rate limiters and retained bucket counters."
summary: "Read-only limiter inspection through the command selector."
date: 2026-11-03T00:00:00Z
lastmod: 2026-11-03T00:00:00Z
draft: false
weight: 130
toc: true
---

Start Lazykiq with `--enable=ent`, press `Ctrl+t`, and choose **Rate Limits**.
This screen has no navigation-bar entry or numeric shortcut. Inspection works
without `--danger`; there are no create, edit, reset, delete, unlock, or token
acquisition actions.

## Find and inspect a limiter

The overview shows each persisted limiter's name, type, capacity or rate, and
record state. Press `Enter` to inspect it; `Esc` returns to the same list page.
Press `/` to search by a literal, case-sensitive name substring. Characters such
as `*`, `?`, `[` and `\` are treated as text, not Redis patterns.

Discovery uses bounded cursor scans, including every primary when connected to
Redis Cluster. Each scan page contains at most 50 records and performs at most
ten SCAN calls. An empty page can still have more results; press `]` to continue.
Rows are sorted within each page. SCAN does not provide a consistent snapshot
of a changing database, so new keys may require restarting discovery.
Repeated keys are deduplicated during each traversal. Previously visited pages
and their selection are retained until discovery restarts or you leave Rate Limits.

`r` and the five-second refresh reread the current page by Redis key. They preserve
selection and page boundaries, including a visible **Disappeared** row for an
expired limiter. `Ctrl+r` starts a fresh discovery traversal. If the cluster's
primary set changes during paging, restart discovery when prompted.

| Key | Action |
| --- | --- |
| `/` | Open the name filter. |
| `Ctrl+u` | Clear the filter and restart discovery. |
| `[` / `]` | Previous / next discovery page. |
| `r` | Refresh the current page or detail. |
| `Ctrl+r` | Restart overview discovery; refresh in detail. |
| `Enter` | Open the selected limiter. |
| `Esc` | Return from detail. |
| `c` / `C` | Copy the limiter name / Redis key. |
| `Tab` / `Shift+Tab` | Switch between bucket properties and counters. |
| `Up` / `k`, `Down` / `j` | Move within the focused table. |
| `PgUp` / `PgDn`, `g` / `G` | Page within the table; jump to its top / bottom. |
| `Left` / `h`, `Right` / `l` | Scroll horizontally. |
| `Home` / `0`, `End` / `$` | Scroll to the first / last column. |
| `?` | Open contextual help. |

On narrow terminals, bucket properties and counters share one panel; use `Tab`
to switch. Long keys, warnings, and explanations remain accessible by horizontal
scrolling. Filters use the normal input dialog, which owns typing while open.

## Read the details

**Concurrent** shows observed used and free slots and their sum as observed
capacity. It includes held count and cumulative held duration, immediate
acquisitions, waited count and cumulative wait duration, overtime count, and
reclaimed locks. Durations are seconds; recorded millisecond totals are converted
for display. Capacity reads span separate Redis keys and can change during the
observation. Inspection does not reclaim expired locks or infer configured
capacity from a partial record.

**Window** shows configured size and interval, including arbitrary numeric
intervals in seconds. It also shows the retained active-window counter and its
remaining lifetime when present. **Bucket** shows configured size and interval
and recent retained counters. These counters record attempted units, including
over-limit attempts, and can exceed the configured size. Neither type records
hit/miss totals or cumulative wait duration.

**Leaky bucket** shows burst capacity in operations, the time to drain a full
bucket in seconds, and the corresponding operations per second. **Points** shows
burst capacity in points, full-refill time in seconds, and points per second.
Both show recorded hit/miss counts, cumulative wait duration in seconds, last
accounting time, and the persisted drops or points. Persisted state is the last
accounted value; Lazykiq does not simulate drain, refill, or current token availability.

**Unavailable** means a metric was absent or its counter expired; it does not
mean zero. A displayed `0` was actually recorded, or is an observed empty capacity
structure. Malformed records and unsupported limiter types remain visible and
do not prevent inspecting valid records. A limiter that expires or changes type
while its detail is open is shown as disappeared; return and restart discovery
to find a replacement under another key.

Details also show the record's remaining lifetime and observation time.
Application options that are not persisted, including lock/wait timeouts and
policy, cannot be recovered from Redis.

## Bucket counter history

History reads existing string counters with `GET`. It never creates missing
counters, changes counts, or extends expiration. No history is retained by
Lazykiq, and missing samples remain unavailable.

| Interval | Display window | Calendar key resolution |
| --- | --- | --- |
| Second | Current second and preceding 59 seconds | Minute and second |
| Minute | Current minute and preceding 59 minutes | Hour and minute |
| Hour | Current hour and preceding 23 elapsed hours | Date and hour |
| Day | Current day and preceding 29 calendar days | Date |

Counter lifetimes depend on the writer. A missing value can mean no attempted
operations or an expired counter, so it cannot safely be reported as zero.
The current bucket is incomplete. Local calendar hours can repeat at the end
of daylight saving time; those hours share one persisted counter. Lazykiq marks
the duplicate sample as a shared calendar bucket rather than reporting its
count twice.

## Configure the limiter connection

Limiters use job Redis by default. To inspect a different pool, pass its Redis URL:

```bash
lazykiq --enable=ent --limiter-redis=redis://limiter.internal:6379/2
```

Standalone URLs support the existing Redis URL authentication, database, TLS
(`rediss://`), and Unix-socket options. Connection labels omit credentials and
URL query parameters. Separately created pools are closed on exit.

For Redis Cluster, supply an explicit seed URL and `--limiter-cluster`. Cluster
supports database zero only. Additional seed addresses share the first URL's
authentication and TLS settings:

```bash
lazykiq --enable=ent --limiter-cluster \
  --limiter-redis='rediss://limiter.internal:6379/0?addr=limiter2.internal:6379'
```

Bucket writers format their keys in the Ruby process's timezone, which is not
stored in Redis. Set `--limiter-timezone` to that timezone. Its default is UTC;
it is independent of `--periodic-timezone`:

```bash
lazykiq --enable=ent --limiter-timezone=America/Toronto
```

Use the timezone of the processes writing the limiters, not the machine running
Lazykiq. Mixed writer timezones cannot be reconstructed reliably from these keys.
Host clock skew can also affect which recent counters are available.

See the [Sidekiq Enterprise rate limiting documentation](https://github.com/sidekiq/sidekiq/wiki/Ent-Rate-Limiting)
for the application-side behavior of each limiter type.
