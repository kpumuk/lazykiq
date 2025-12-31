# frozen_string_literal: true

require_relative "boot"
require "sidekiq/web"
require "securerandom"
require "rack/session"

# Enable sessions for CSRF protection
use Rack::Session::Cookie,
  key: "sidekiq_session",
  secret: ENV.fetch("SESSION_SECRET") { SecureRandom.hex(32) },
  same_site: true,
  max_age: 86_400

run Sidekiq::Web
