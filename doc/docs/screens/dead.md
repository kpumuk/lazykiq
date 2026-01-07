---
title: "Dead"
description: "Inspect jobs that have exhausted retries."
summary: "Inspect jobs that have exhausted retries."
date: 2025-12-30T00:00:00Z
lastmod: 2025-12-30T00:00:00Z
draft: false
weight: 90
toc: true
---

Dead jobs have failed permanently and need intervention.

{{< lightbox src="assets/dead.png" alt="Dead screen" >}}

**Key bindings:**

| Key          | Description                           |
|--------------|---------------------------------------|
| `6`          | Go to Dead.                           |
| `Up` / `k`   | Move up one row.                      |
| `Down` / `j` | Move down one row.                    |
| `Enter`      | Show job details.                     |
| `/`          | Filter jobs by case-sensitive string. |
| `Ctrl+u`     | Reset filter.                         |
| `[` / `]`    | Go to previos or next page.           |
| `q`          | Quit.                                 |

## Job Details

Shows detailed information about a dead job.

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
