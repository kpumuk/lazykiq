---
title: "Retries"
description: "Review failed jobs scheduled for retry."
summary: "Review failed jobs scheduled for retry."
date: 2025-12-30T00:00:00Z
lastmod: 2025-12-30T00:00:00Z
draft: false
weight: 70
toc: true
---

Retries list jobs that failed and will be retried by Sidekiq.

{{< lightbox src="assets/retries.png" alt="Retries screen" >}}

**Key bindings:**

| Key          | Description                           |
|--------------|---------------------------------------|
| `4`          | Go to Retries.                        |
| `Up` / `k`   | Move up one row.                      |
| `Down` / `j` | Move down one row.                    |
| `Enter`      | Show job details.                     |
| `/`          | Filter jobs by case-sensitive string. |
| `Ctrl+u`     | Reset filter.                         |
| `[` / `]`    | Go to previos or next page.           |
| `q`          | Quit.                                 |

## Job Details

Shows detailed information about a retrying job.

{{< lightbox src="assets/job_details.png" alt="Job details screen" >}}

**Key bindings:**

| Key          | Description                                    |
|--------------|------------------------------------------------|
| `Up` / `k`   | Move up one row.                               |
| `Down` / `j` | Move down one row.                             |
| `Left` / `h` | Move up one row.                               |
| `Down` / `j` | Move down one row.                             |
| `Tab`        | Switch between job details panel and job data. |
| `Esc`        | Back to Queue details view.                    |
| `q`          | Quit.                                          |
