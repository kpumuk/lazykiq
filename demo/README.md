# Sidekiq Simulation

A demo environment for testing Sidekiq TUI tools. Generates continuous job traffic across multiple queues with realistic failure scenarios.

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
| DataSyncJob | batch | 8% | 1 |
| AnalyticsJob | low | 0% | 1 |
| CacheWarmupJob | low | 0% | 1 |
| CleanupJob | low | 0% | 0 |

**5 Queues** (by priority): critical, default, mailers, batch, low

**Limits**: 10k jobs/queue, 20k retry queue max

**Extra cases**:
- ActiveJob-wrapped jobs with GlobalID-serialized arguments
- ActionMailer-wrapped jobs with arguments (including GlobalID)
- Tagged jobs sharing the same 5 tags (intersection)

## Files

```
├── start.rb          # Entry point
├── scheduler.rb      # Job scheduling logic
├── boot.rb           # Sidekiq configuration
├── config.ru         # Web UI
└── lib/jobs/         # Job classes
```
