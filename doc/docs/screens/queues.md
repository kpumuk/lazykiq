---
title: "Queues"
description: "Browse queues and inspect queued jobs."
summary: "Browse queues and inspect queued jobs."
date: 2025-12-30T00:00:00Z
lastmod: 2025-12-30T00:00:00Z
draft: false
weight: 60
toc: true
---

Queues show the backlog waiting to be processed, with per-queue counts.

{{< lightbox src="assets/queue_details.png" alt="Queue jobs screen" >}}

**Key bindings:**

| Key               | Description                 |
|-------------------|-----------------------------|
| `3`               | Go to Queues.               |
| `Up` / `k`        | Move up one row.            |
| `Down` / `j`      | Move down one row.          |
| `Enter`           | Show job details.           |
| `Ctrl+1`–`Ctrl+5` | Filter jobs by queue.       |
| `[` / `]`         | Go to previos or next page. |
| `s`               | Queue view / select.        |
| `q`               | Quit.                       |

## Queue List

{{< lightbox src="assets/queues.png" alt="Queue list screen" >}}

**Key bindings:**

| Key          | Description                             |
|--------------|-----------------------------------------|
| `Up` / `k`   | Move up one row.                        |
| `Down` / `j` | Move down one row.                      |
| `/`          | Filter queues by case-sensitive string. |
| `Enter`      | Show jobs in the queue.                 |
| `Esc`        | Back to Queue details view.             |
| `q`          | Quit.                                   |

## Job Details

Shows detailed information about a queued job.

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
