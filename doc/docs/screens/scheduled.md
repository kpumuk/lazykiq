---
title: "Scheduled"
description: "Inspect jobs scheduled to run in the future."
summary: "Inspect jobs scheduled to run in the future."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-09T00:00:00Z
draft: false
weight: 80
toc: true
---

Scheduled jobs are queued to run at a specific time.

{{< lightbox src="assets/scheduled.png" alt="Scheduled screen" >}}

**Key bindings:**

| Key          | Description                                               |
|--------------|-----------------------------------------------------------|
| `5`          | Go to Scheduled.                                          |
| `Up` / `k`   | Move up one row.                                          |
| `Down` / `j` | Move down one row.                                        |
| `Enter`      | Show job details.                                         |
| `c`          | Copy job JID.                                             |
| `/`          | Filter jobs by substring.                                 |
| `Ctrl+u`     | Clear filter.                                             |
| `[` / `]`    | Previous or next page (also `Alt+Left` / `Alt+Right`).    |
| `D`          | Delete job (requires `--danger`).                         |
| `R`          | Add job to queue now (requires `--danger`).               |
| `q`          | Quit.                                                     |

## Job Details

Shows detailed information about a scheduled job.

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
| `Esc`        | Back to Scheduled view.                        |
| `q`          | Quit.                                          |
