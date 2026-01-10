---
title: "Metrics"
description: "At-a-glance counters and job metrics."
summary: "At-a-glance counters and job metrics."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-10T00:00:00Z
draft: false
weight: 110
toc: true
---

The metrics view tracks processed, failed, busy, enqueued, retries, scheduled,
and dead counts in real time, plus per-job metrics.

{{< lightbox src="assets/metrics.png" alt="Metrics screen" >}}

**Key bindings:**

| Key          | Description                                               |
|--------------|-----------------------------------------------------------|
| `8`          | Go to Metrics.                                            |
| `Up` / `k`   | Move up one row.                                          |
| `Down` / `j` | Move down one row.                                        |
| `Enter`      | Open job metrics.                                         |
| `/`          | Filter jobs by substring.                                 |
| `Ctrl+u`     | Clear filter.                                             |
| `[` / `]`    | Page up or down (also `Alt+Left` / `Alt+Right`).          |
| `{` / `}`    | Change metrics period.                                    |
| `q`          | Quit.                                                     |

## Job metrics

Job metrics show per-job performance and breakdowns.

{{< lightbox src="assets/job_metrics.png" alt="Job metrics screen" >}}

**Key bindings:**

| Key           | Description                    |
|---------------|--------------------------------|
| `Tab`         | Switch panel.                  |
| `Shift+Tab`   | Switch panel.                  |
| `{` / `}`     | Change metrics period.         |
| `Esc`         | Close job metrics.             |
| `q`           | Quit.                          |
