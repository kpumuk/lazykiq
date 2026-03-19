#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative "boot"
require_relative "scheduler"

def worker_profiles_for(layout)
  case layout
  when "stretched"
    [
      {queues: ["default,5"], concurrency: 2},
      {queues: ["low,1"], concurrency: 1},
      {queues: ["critical,10"], concurrency: 2},
      {queues: ["mailers,5"], concurrency: 1},
      {queues: ["batch,3"], concurrency: 1, unsafe_capsule: true}
    ]
  else
    [
      {queues: ["default,5", "low,1"], concurrency: 4},
      {queues: ["critical,10", "mailers,5", "batch,3"], concurrency: 4, unsafe_capsule: true}
    ]
  end
end

# Determine mode
mode = ARGV[0] || "all"

case mode
when "worker"
  # Start Sidekiq worker only
  puts "Starting Sidekiq worker..."
  exec("bundle", "exec", "sidekiq", "-r", "./boot.rb", "-C", "config/sidekiq.yml")

when "scheduler"
  # Start scheduler only
  scheduler = JobScheduler.new
  trap("INT") { scheduler.stop }
  trap("TERM") { scheduler.stop }
  scheduler.start

when "web"
  # Start Sidekiq Web UI only
  puts "Starting Sidekiq Web UI on http://localhost:9292..."
  exec("bundle", "exec", "rackup", "-o", "0.0.0.0", "-p", "9292")

when "all"
  # Start worker, scheduler, and web UI
  puts "Starting Sidekiq simulation..."
  puts "Sidekiq Web UI: http://localhost:9292"
  puts "Press Ctrl+C to stop"
  puts ""

  pids = []

  # Stretch the latest demo with more Sidekiq processes while keeping older
  # versions compact and cheap.
  worker_profiles = worker_profiles_for(ENV.fetch("LAZYKIQ_DEMO_LAYOUT", "compact"))

  worker_profiles.each do |profile|
    pids << fork do
      cmd = ["bundle", "exec", "sidekiq", "-r", "./boot.rb", "-C", "config/sidekiq.yml"] +
        profile[:queues].flat_map { |q| ["-q", q] } +
        ["-c", profile[:concurrency].to_s]
      if profile[:unsafe_capsule]
        exec({"LAZYKIQ_UNSAFE_CAPSULE" => "1"}, *cmd)
      else
        exec(*cmd)
      end
    end
  end

  # Fork web UI process
  pids << fork do
    exec("bundle", "exec", "rackup", "-o", "0.0.0.0", "-p", "9292")
  end

  # Give processes time to start
  sleep 2

  # Run scheduler in main process
  scheduler = JobScheduler.new

  shutdown = proc do
    puts "\nShutting down..."
    scheduler.stop
    pids.each { |pid|
      begin
        Process.kill("TERM", pid)
      rescue
        nil
      end
    }
  end

  trap("INT", &shutdown)
  trap("TERM", &shutdown)

  scheduler.start

  # Wait for child processes
  pids.each { |pid|
    begin
      Process.wait(pid)
    rescue
      nil
    end
  }

else
  puts "Usage: ruby start.rb [mode]"
  puts "Modes:"
  puts "  all       - Start worker, scheduler, and web UI (default)"
  puts "  worker    - Start Sidekiq worker only"
  puts "  scheduler - Start job scheduler only"
  puts "  web       - Start Sidekiq Web UI only"
  exit 1
end
