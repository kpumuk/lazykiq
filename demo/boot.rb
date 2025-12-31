# frozen_string_literal: true

require "sidekiq"

# Configure Sidekiq
Sidekiq.configure_client do |config|
  config.redis = {url: ENV.fetch("REDIS_URL", "redis://localhost:6379/0")}
end

Sidekiq.configure_server do |config|
  config.redis = {url: ENV.fetch("REDIS_URL", "redis://localhost:6379/0")}
end

# Load all job classes
Dir[File.join(__dir__, "lib", "jobs", "*.rb")].each { |file| require file }
