# frozen_string_literal: true

class WebhookDeliveryJob
  include Sidekiq::Job

  sidekiq_options queue: :default, retry: 2

  class ConnectionRefusedError < StandardError; end
  class TimeoutError < StandardError; end
  class HTTPError < StandardError; end

  def perform(webhook_url, event_type, payload)
    # Simulate HTTP POST to external endpoint (100-400ms)
    sleep(rand(100..400) / 1000.0)

    # 10% chance of failure (will retry up to 2 times)
    if rand(100) < 10
      error = [ConnectionRefusedError, TimeoutError, HTTPError].sample
      raise error, "Failed to deliver webhook to #{webhook_url}"
    end
  end
end
