---
title: "Errors"
description: "Explore error summaries and details across queues."
summary: "Explore error summaries and details across queues."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-08T00:00:00Z
draft: false
weight: 100
toc: true
---

Errors group failures by exception so you can spot the biggest problems fast.

{{< lightbox src="assets/errors_summary.png" alt="Errors screen" >}}

**Key bindings:**

| Key          | Description                   |
|--------------|-------------------------------|
| `7`          | Go to Errors.                 |
| `Up` / `k`   | Move up one row.              |
| `Down` / `j` | Move down one row.            |
| `Enter`      | Open error details.           |
| `/`          | Filter errors by substring.   |
| `Ctrl+u`     | Clear filter.                 |
| `q`          | Quit.                         |

## Error details

Drill into a specific error to see its backtrace, payload, and occurrence data.

{{< lightbox src="assets/errors_details.png" alt="Error details screen" >}}

**Key bindings:**

| Key          | Description                         |
|--------------|-------------------------------------|
| `Up` / `k`   | Move up one row.                    |
| `Down` / `j` | Move down one row.                  |
| `Enter`      | Show job details.                   |
| `/`          | Filter errors by substring.         |
| `Ctrl+u`     | Clear filter.                       |
| `c`          | Copy job JID.                       |
| `Esc`        | Back to Errors summary view.        |
| `q`          | Quit.                               |

## Job Details

Shows detailed information about a failing job.

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
| `Esc`        | Back to Error details view.                    |
| `q`          | Quit.                                          |
