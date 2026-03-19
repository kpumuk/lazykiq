# Sidekiq Simulation

A demo environment for testing Sidekiq TUI tools. Generates continuous job traffic across multiple queues with realistic failure scenarios.

The current profile is intentionally lightweight: older Sidekiq versions stay compact, while the 8.1 demo uses five thin worker processes to stretch the process-oriented UI without bringing back the original CPU load.

## Quick Start

```bash
docker-compose up --build
```

Web UI: http://localhost:9292

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
