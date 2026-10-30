# frozen_string_literal: true

require_relative "boot"
require "timeout"

abort "Periodic demo seeding requires LAZYKIQ_DEMO_FEATURES=ent" unless DemoPeriodicJobs.enabled?

require "sidekiq-ent/periodic/testing"

registered = Sidekiq::Periodic::ConfigTester.new.verify(&DemoPeriodicJobs.method(:register))
expected_ids = registered.map(&:lid).sort

# Let the elected leader publish the configuration instead of overwriting it.
loops = Timeout.timeout(30) do
  loop do
    current = Sidekiq::Periodic::LoopSet.new.to_a
    break current if current.map(&:lid).sort == expected_ids

    sleep 0.25
  end
end

loops.each do |entry|
  # Keep the monthly entry fresh to exercise an empty history.
  next if entry.schedule == "0 0 1 * *"

  if entry.history.empty?
    3.times do
      jid = entry.enqueue_job
      next unless jid

      Sidekiq.redis do |redis|
        key = "loop-history-#{entry.lid}"
        redis.pipelined do |pipeline|
          pipeline.zadd(key, Time.now.to_i, jid)
          pipeline.zremrangebyrank(key, 0, -26)
          pipeline.expire(key, Sidekiq::Periodic::Config::STATIC_TTL)
        end
      end
    end
  end

  entry.pause! if entry.klass == "CleanupJob" && entry.schedule == "30 2 * * *"
end

puts "Enterprise periodic demo: #{loops.size} schedules (#{Time.now.zone})"
loops.sort_by { |entry| [entry.klass, entry.schedule] }.each do |entry|
  puts "#{entry.klass.ljust(22)} #{entry.schedule.ljust(15)} paused=#{entry.paused?} history=#{entry.history.size}"
end
