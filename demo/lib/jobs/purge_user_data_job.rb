# frozen_string_literal: true

class PurgeUserDataJob
  include Sidekiq::Job

  sidekiq_options queue: :unsafe, retry: 0

  def perform(user_id, reason)
    # Simulate a long-running data purge (2-42s)
    sleep(rand(2..42))
  end
end
