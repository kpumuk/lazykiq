# frozen_string_literal: true

class DataSyncJob
  include Sidekiq::Job

  sidekiq_options queue: :batch, retry: 1  # Only 1 retry - quickly goes to dead

  class APIRateLimitError < StandardError; end
  class AuthenticationError < StandardError; end
  class SyncConflictError < StandardError; end

  def perform(source, destination, entity_ids)
    # Simulate external API sync operations (400-900ms)
    sleep(rand(400..900) / 1000.0)

    # 8% chance of failure (will exhaust retry quickly)
    if rand(100) < 8
      error = [APIRateLimitError, AuthenticationError, SyncConflictError].sample
      raise error, "Sync failed from #{source} to #{destination}"
    end
  end
end
