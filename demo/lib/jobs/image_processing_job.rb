# frozen_string_literal: true

class ImageProcessingJob
  include Sidekiq::Job

  sidekiq_options queue: :default, retry: 0  # No retries - goes straight to dead

  class ImageCorruptedError < StandardError; end
  class UnsupportedFormatError < StandardError; end
  class OutOfMemoryError < StandardError; end

  def perform(image_id, operations)
    # Simulate image resize/transform operations (300-800ms)
    sleep(rand(300..800) / 1000.0)

    # 12% chance of failure (goes straight to dead queue)
    if rand(100) < 12
      error = [ImageCorruptedError, UnsupportedFormatError, OutOfMemoryError].sample
      raise error, "Failed to process image #{image_id}"
    end
  end
end
