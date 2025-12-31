# frozen_string_literal: true

class PaymentProcessingJob
  include Sidekiq::Job

  sidekiq_options queue: :critical, retry: 2

  class PaymentDeclinedError < StandardError; end
  class GatewayTimeoutError < StandardError; end

  def perform(order_id, amount, currency)
    # Simulate payment gateway API call (200-500ms)
    sleep(rand(200..500) / 1000.0)

    # 5% chance of failure (will retry up to 2 times)
    if rand(100) < 5
      raise [PaymentDeclinedError, GatewayTimeoutError].sample, "Payment failed for order #{order_id}"
    end
  end
end
