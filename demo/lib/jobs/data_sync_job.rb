# frozen_string_literal: true

class DataSyncJob
  include Sidekiq::IterableJob

  sidekiq_options queue: :batch, retry: 1  # Only 1 retry - quickly goes to dead

  class APIRateLimitError < StandardError; end
  class AuthenticationError < StandardError; end
  class SyncConflictError < StandardError; end

  # Build enumerator for iteration over entity IDs
  def build_enumerator(source, destination, entity_ids, cursor:)
    array_enumerator(entity_ids, cursor: cursor)
  end

  # Process one entity ID per iteration
  def each_iteration(entity_id, source, destination, entity_ids)
    # Simulate external API sync operation (200-1000ms per entity)
    duration = rand(0.2..1.0)
    sleep(duration)

    # 8% chance of failure per iteration (will exhaust retry quickly)
    if rand(100) < 8
      error = [APIRateLimitError, AuthenticationError, SyncConflictError].sample
      raise error, "Sync failed for entity #{entity_id} from #{source} to #{destination}"
    end

    # Log progress (visible in Sidekiq logs)
    logger.info "Synced entity #{entity_id} from #{source} to #{destination} (#{duration.round(2)}s)"
  end
end
