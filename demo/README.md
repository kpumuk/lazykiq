# Sidekiq Simulation

A demo environment for testing Sidekiq TUI tools. Generates continuous job traffic across multiple queues with realistic failure scenarios.

The current profile is intentionally lightweight: older Sidekiq versions stay compact, while the 8.1 demo uses five thin worker processes to stretch the process-oriented UI without bringing back the original CPU load.

## Quick Start

```bash
mise run demo
mise run demo-sidekiq73
mise run demo-sidekiq80
mise run demo-sidekiq81
```

`mise run demo` starts the latest Sidekiq demo (8.1) on `http://localhost:9292` with `redis://localhost:6379/0`.
`mise run demo-sidekiq73` starts Sidekiq 7.3 on `http://localhost:9294` with `redis://localhost:6379/2`.
`mise run demo-sidekiq80` starts Sidekiq 8.0 on `http://localhost:9293` with `redis://localhost:6379/1`.
`mise run demo-sidekiq81` starts Sidekiq 8.1 on `http://localhost:9292` with `redis://localhost:6379/0`.

The three tasks can run in parallel because each demo stand uses its own Redis database and dashboard port.

## Structure

**10 Base Jobs** with varying performance (10-1000ms):

| Job | Queue | Fail Rate | Retries |
|-----|-------|-----------|---------|
| NotificationJob | critical | 0% | 3 |
| PaymentProcessingJob | critical | 5% | 2 |
| ImageProcessingJob | default | 12% | 0 (→ dead) |
| WebhookDeliveryJob | default | 10% | 2 |
| EmailDeliveryJob | mailers | 0% | 3 |
| ReportGenerationJob | batch | 0% | 1 |
| DataSyncJob (iterable) | batch | 8% | 1 |
| AnalyticsJob | low | 0% | 1 |
| CacheWarmupJob | low | 0% | 1 |
| CleanupJob | low | 0% | 0 |

**5 Queues** (by priority): critical, default, mailers, batch, low

**Limits**: 1.5k jobs/queue, 1k retry queue max, 500 scheduled max

**Extra cases**:
- ActiveJob-wrapped jobs with GlobalID-serialized arguments
- ActionMailer-wrapped jobs with arguments (including GlobalID)
- Tagged jobs sharing the same 5 tags (intersection)
- **DataSyncJob** uses `Sidekiq::IterableJob` to process entity IDs one at a time:
  - Iterates over 1-10 entity IDs per job
  - Random sleep duration per iteration (0.2-1.0s)
  - 8% failure rate per iteration for realistic retry behavior
  - Demonstrates job interruption and resumption on worker restart

## Files

```
├── start.rb          # Entry point
├── scheduler.rb      # Job scheduling logic
├── boot.rb           # Sidekiq configuration
├── config.ru         # Web UI
└── lib/jobs/         # Job classes
```
