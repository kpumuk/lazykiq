# frozen_string_literal: true

require "sidekiq"

# Configure Sidekiq
Sidekiq.configure_client do |config|
  config.redis = {url: ENV.fetch("REDIS_URL", "redis://localhost:6379/0")}
end

Sidekiq.configure_server do |config|
  config.redis = {url: ENV.fetch("REDIS_URL", "redis://localhost:6379/0")}

  if ENV["LAZYKIQ_UNSAFE_CAPSULE"] == "1"
    # define a new capsule which processes jobs from the `unsafe` queue one at a time
    config.capsule("unsafe") do |cap|
      cap.concurrency = 1
      cap.queues = %w[unsafe]
    end
  end
end

# Load all job classes
Dir[File.join(__dir__, "lib", "jobs", "*.rb")].each { |file| require file }
