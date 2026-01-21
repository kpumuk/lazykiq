---
title: "Retries"
description: "Review failed jobs scheduled for retry."
summary: "Review failed jobs scheduled for retry."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-20T00:00:00Z
draft: false
weight: 70
toc: true
---

Retries list jobs that failed and will be retried by Sidekiq.

{{< lightbox src="assets/retries.png" alt="Retries screen" >}}

**Key bindings:**

| Key          | Description                                               |
|--------------|-----------------------------------------------------------|
| `4`          | Go to Retries.                                            |
| `Up` / `k`   | Move up one row.                                          |
| `Down` / `j` | Move down one row.                                        |
| `Enter`      | Show job details.                                         |
| `c`          | Copy job JID.                                             |
| `/`          | Filter jobs by substring.                                 |
| `Ctrl+u`     | Clear filter.                                             |
| `[` / `]`    | Page up or down (also `Alt+Left` / `Alt+Right`).          |
| `g` / `G`    | Jump to start or end.                                     |
| `D`          | Delete job (requires `--danger`).                         |
| `K`          | Kill job (move to dead, requires `--danger`).             |
| `R`          | Retry job now (requires `--danger`).                      |
| `Ctrl+D`     | Delete all retries (requires `--danger`).                 |
| `Ctrl+K`     | Kill all retries (requires `--danger`).                   |
| `Ctrl+R`     | Retry all retries now (requires `--danger`).              |
| `q`          | Quit.                                                     |

## Job Details

Shows detailed information about a retrying job.

{{< lightbox src="assets/job_details.png" alt="Job details screen" >}}

**Key bindings:**

| Key          | Description                                    |
|--------------|------------------------------------------------|
| `Up` / `k`   | Scroll up.                                     |
| `Down` / `j` | Scroll down.                                   |
| `Left` / `h` | Scroll left.                                   |
| `Right` / `l` | Scroll right.                                 |
| `g` / `G`    | Jump to top or bottom.                         |
| `Home` / `0` | Scroll to the first column.                    |
| `End` / `$`  | Scroll to the last column.                     |
| `Tab`        | Switch between job details panel and job data. |
| `c`          | Copy job JSON.                                 |
| `Esc`        | Back to Retries view.                          |
| `q`          | Quit.                                          |
