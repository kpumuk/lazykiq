---
title: "Busy"
description: "Inspect currently running jobs and workers."
summary: "Inspect currently running jobs and workers."
date: 2025-12-30T00:00:00Z
lastmod: 2025-12-30T00:00:00Z
draft: false
weight: 50
toc: true
---

The Busy screen lists top active workers and all the jobs in progress.

{{< lightbox src="assets/busy.png" alt="Busy screen" >}}

**Key bindings:**

| Key               | Description                  |
|-------------------|------------------------------|
| `2`               | Go to Busy.                  |
| `Up` / `k`        | Move up one row.             |
| `Down` / `j`      | Move down one row.           |
| `Ctrl+0`          | Show jobs for all processes. |
| `Ctrl+1`–`Ctrl+5` | Filter jobs by process.      |
| `t`               | Toggle tree view.            |
| `s`               | Process view / select.       |
| `q`               | Quit.                        |

## Tree view

Tree view shows similar information, but groups active jobs by the process which executes them.

{{< lightbox src="assets/busy_tree.png" alt="Busy screen" >}}

**Key bindings:**

| Key               | Description                  |
|-------------------|------------------------------|
| `Up` / `k`        | Move up one row.             |
| `Down` / `j`      | Move down one row.           |
| `Enter`           | Show job details.            |
| `Ctrl+0`          | Show jobs for all processes. |
| `Ctrl+1`–`Ctrl+5` | Filter jobs by process.      |
| `t`               | Toggle tree view.            |
| `s`               | Process view / select.       |
| `q`               | Quit.                        |

## Process view

Process view lists all Sidekiq processes, and allows to select one for job filtering.

{{< lightbox src="assets/processes.png" alt="Processes screen" >}}

**Key bindings:**

| Key          | Description                                |
|--------------|--------------------------------------------|
| `Up` / `k`   | Move up one row.                           |
| `Down` / `j` | Move down one row.                         |
| `/`          | Filter processes by case-sensitive string. |
| `Enter`      | Show process active jobs.                  |
| `Esc`        | Back to Busy view.                         |
| `q`          | Quit.                                      |

## Job Details

Shows detailed information about a running job.

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
