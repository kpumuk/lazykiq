# frozen_string_literal: true

class ReportGenerationJob
  include Sidekiq::Job

  sidekiq_options queue: :batch, retry: 1

  def perform(report_type, date_range, user_id)
    # Simulate heavy database queries and PDF generation (500-1000ms)
    sleep(rand(500..1000) / 1000.0)
  end
end
