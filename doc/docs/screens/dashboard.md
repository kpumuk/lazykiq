---
title: "Dashboard"
description: "Overview of processed, failed, busy, and queue metrics."
summary: "Overview of processed, failed, busy, and queue metrics."
date: 2025-12-30T00:00:00Z
lastmod: 2026-01-08T00:00:00Z
draft: false
weight: 40
toc: true
---

The dashboard provides a quick health check of your Sidekiq system. It shows
throughput, failures, and queue depth over time.

{{< lightbox src="assets/dashboard.png" alt="Dashboard screen" >}}

**Key bindings:**

| Key       | Description                                |
|-----------|--------------------------------------------|
| `1`       | Go to Dashboard.                           |
| `Tab`     | Switch between realtime and history panes. |
| `{` / `}` | Change time interval or historical range.  |
| `q`       | Quit.                                      |
