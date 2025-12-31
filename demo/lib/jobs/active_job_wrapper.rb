# frozen_string_literal: true

module Sidekiq
  module ActiveJob
    class Wrapper
      include Sidekiq::Job

      def perform(_payload)
        # Minimal ActiveJob wrapper for the demo environment.
        sleep(rand(20..80) / 1000.0)
      end
    end
  end
end

module ActiveJob
  module QueueAdapters
    module SidekiqAdapter
      class JobWrapper < Sidekiq::ActiveJob::Wrapper
      end
    end
  end
end
