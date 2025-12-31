# frozen_string_literal: true

class NotificationJob
  include Sidekiq::Job

  sidekiq_options queue: :critical, retry: 3

  def perform(user_id, notification_type, payload)
    # Simulate push notification delivery (10-50ms)
    sleep(rand(10..50) / 1000.0)
  end
end
