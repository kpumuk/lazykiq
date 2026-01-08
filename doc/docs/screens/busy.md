---
title: "Busy"
description: "Inspect currently running jobs and workers."
summary: "Inspect currently running jobs and workers."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-08T00:00:00Z
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
| `Enter`           | Show job details.            |
| `/`               | Filter jobs by substring.    |
| `Ctrl+u`          | Clear filter.                |
| `Ctrl+0`          | Show jobs for all processes. |
| `Ctrl+1`–`Ctrl+9` | Filter jobs by process.      |
| `t`               | Toggle tree view.            |
| `s`               | Open process list.           |
| `c`               | Copy job JID.                |
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
| `/`               | Filter jobs by substring.    |
| `Ctrl+u`          | Clear filter.                |
| `Ctrl+0`          | Show jobs for all processes. |
| `Ctrl+1`–`Ctrl+9` | Filter jobs by process.      |
| `t`               | Toggle tree view.            |
| `s`               | Open process list.           |
| `c`               | Copy job JID.                |
| `q`               | Quit.                        |

## Process view

Process view lists all Sidekiq processes, and allows to select one for job filtering.

{{< lightbox src="assets/processes.png" alt="Processes screen" >}}

**Key bindings:**

| Key          | Description                          |
|--------------|--------------------------------------|
| `Up` / `k`   | Move up one row.                     |
| `Down` / `j` | Move down one row.                   |
| `/`          | Filter processes by substring.       |
| `Enter`      | Select process and return to Busy.   |
| `c`          | Copy process identity.               |
| `p`          | Pause process (requires `--danger`). |
| `s`          | Stop process (requires `--danger`).  |
| `Esc`        | Back to Busy view.                   |
| `q`          | Quit.                                |

## Job Details

Shows detailed information about a running job.

{{< lightbox src="assets/job_details.png" alt="Job details screen" >}}

**Key bindings:**

| Key           | Description                                    |
|---------------|------------------------------------------------|
| `Up` / `k`    | Scroll up.                                     |
| `Down` / `j`  | Scroll down.                                   |
| `Left` / `h`  | Scroll left.                                   |
| `Right` / `l` | Scroll right.                                 |
| `g` / `G`     | Jump to top or bottom.                         |
| `Home` / `0`  | Scroll to the first column.                    |
| `End` / `$`   | Scroll to the last column.                     |
| `Tab`         | Switch between job details panel and job data. |
| `c`           | Copy job JSON.                                 |
| `Esc`         | Back to Busy view.                             |
| `q`           | Quit.                                          |
