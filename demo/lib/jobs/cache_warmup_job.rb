# frozen_string_literal: true

class CacheWarmupJob
  include Sidekiq::Job

  sidekiq_options queue: :low, retry: 1

  def perform(cache_key, resource_type)
    # Simulate database query and cache population (100-300ms)
    sleep(rand(100..300) / 1000.0)
  end
end
