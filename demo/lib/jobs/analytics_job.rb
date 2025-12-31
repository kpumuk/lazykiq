# frozen_string_literal: true

class AnalyticsJob
  include Sidekiq::Job

  sidekiq_options queue: :low, retry: 1

  def perform(event_type, properties, timestamp)
    # Simulate analytics event processing (50-150ms)
    sleep(rand(50..150) / 1000.0)
  end
end
