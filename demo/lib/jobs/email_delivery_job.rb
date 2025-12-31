# frozen_string_literal: true

class EmailDeliveryJob
  include Sidekiq::Job

  sidekiq_options queue: :mailers, retry: 3

  def perform(recipient, subject, template)
    # Simulate SMTP connection and delivery (50-200ms)
    sleep(rand(50..200) / 1000.0)
  end
end
