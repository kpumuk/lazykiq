---
title: "Danger Mode"
description: "Understand the risks and limitations of mutating Sidekiq state with Lazykiq."
summary: "Caveats and limitations when enabling dangerous actions."
date: 2026-01-09T00:00:00Z
lastmod: 2026-01-09T00:00:00Z
draft: false
weight: 30
toc: true
---

Danger mode enables Lazykiq to change Sidekiq state (delete, retry now, kill, add to queue).
These operations are powerful and irreversible, so use them carefully.

## Caveats and limitations

Lazykiq talks directly to Redis. It does **not** run any Ruby code on the server, which means:

- **No client middleware:** "Retry now" and other enqueue-like actions do not execute
  Sidekiq client middleware or normalization logic.
- **No death handlers:** Killing a retry job only moves the payload into the dead set.
  It does **not** invoke `DeadSet#kill`, so `death_handlers` and any related server-side
  hooks are not called (see [Death Notification](https://github.com/sidekiq/sidekiq/wiki/Error-Handling#death-notification) for details).
- **No dead-set trimming on kill:** When Lazykiq kills a retry, it does not trim the dead
  set using Sidekiq's `dead_timeout`/`dead_max_jobs` limits. Those limits only apply when
  Sidekiq itself trims the dead set later.

If you need exact Sidekiq server behavior (middleware, death handlers, trimming), use the
Sidekiq Web UI or your application’s Ruby tooling.
