---
title: "Configuration"
description: "Configure Lazykiq flags and Redis connection."
summary: "Configure Lazykiq flags and Redis connection."
date: 2025-12-30T00:00:00Z
lastmod: 2025-12-30T00:00:00Z
draft: false
weight: 25
toc: true
---

## CLI help

```text
A terminal UI for Sidekiq.

USAGE
  lazykiq [--flags]

FLAGS
  --cpuprofile  Write cpu profile to file
  -h --help     Help for lazykiq
  --redis       Redis URL (redis://localhost:6379/0)
  -v --version  Version for lazykiq
```

## Connect to Redis

Use the `--redis` flag with a Redis URL. The default is
`redis://localhost:6379/0`.

```bash
lazykiq --redis redis://localhost:6379/0
```

Point to another host or database index by changing the URL:

```bash
lazykiq --redis redis://redis.internal:6379/2
```
