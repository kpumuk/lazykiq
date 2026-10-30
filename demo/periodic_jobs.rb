# frozen_string_literal: true

module DemoPeriodicJobs
  def self.enabled?
    ENV.fetch("LAZYKIQ_DEMO_FEATURES", "").split(",").include?("ent")
  end

  def self.register(manager)
    manager.register("* * * * *", "CleanupJob", queue: "low", args: ["sessions", 7], retry: 0)
    manager.register("*/5 * * * *", "CacheWarmupJob", queue: "low", args: ["cache:products:popular", "product"], retry: 1)
    manager.register("*/15 * * * *", "AnalyticsJob", queue: "low", args: ["page_view", {"source" => "periodic"}, 0], retry: 1)
    manager.register("0 */6 * * *", "ReportGenerationJob", queue: "batch", args: ["sales", "2026-Q4", 42], retry: 1)
    manager.register("0 9 * * 1-5", "EmailDeliveryJob", queue: "mailers", args: ["demo@example.com", "Daily digest", "daily_digest"], retry: 3)
    manager.register("0 12 * * 1", "NotificationJob", queue: "critical", args: [42, "in_app", {"message" => "Weekly report ready"}], retry: 3)
    manager.register("30 2 * * *", "CleanupJob", queue: "low", args: ["exports", 30], retry: 0)
    manager.register("0 0 1 * *", "WebhookDeliveryJob", queue: "default", args: ["https://example.com/webhooks/monthly", "report.ready", {"id" => 42}], retry: 2)
  end
end
