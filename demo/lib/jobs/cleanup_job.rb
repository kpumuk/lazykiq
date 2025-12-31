# frozen_string_literal: true

class CleanupJob
  include Sidekiq::Job

  sidekiq_options queue: :low, retry: 0

  def perform(resource_type, older_than_days)
    # Simulate cleanup operations (20-100ms)
    sleep(rand(20..100) / 1000.0)
  end
end
