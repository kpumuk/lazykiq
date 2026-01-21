---
title: "Dead"
description: "Inspect jobs that have exhausted retries."
summary: "Inspect jobs that have exhausted retries."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-20T00:00:00Z
draft: false
weight: 90
toc: true
---

Dead jobs have failed permanently and need intervention.

{{< lightbox src="assets/dead.png" alt="Dead screen" >}}

**Key bindings:**

| Key          | Description                                               |
|--------------|-----------------------------------------------------------|
| `6`          | Go to Dead.                                               |
| `Up` / `k`   | Move up one row.                                          |
| `Down` / `j` | Move down one row.                                        |
| `Enter`      | Show job details.                                         |
| `c`          | Copy job JID.                                             |
| `/`          | Filter jobs by substring.                                 |
| `Ctrl+u`     | Clear filter.                                             |
| `[` / `]`    | Page up or down (also `Alt+Left` / `Alt+Right`).          |
| `g` / `G`    | Jump to start or end.                                     |
| `D`          | Delete job (requires `--danger`).                         |
| `R`          | Retry job now (requires `--danger`).                      |
| `Ctrl+D`     | Delete all dead jobs (requires `--danger`).               |
| `Ctrl+R`     | Retry all dead jobs now (requires `--danger`).            |
| `q`          | Quit.                                                     |

## Job Details

Shows detailed information about a dead job.

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
| `Esc`        | Back to Dead view.                             |
| `q`          | Quit.                                          |
