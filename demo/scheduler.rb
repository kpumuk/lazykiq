# frozen_string_literal: true

require "securerandom"
require "sidekiq/api"
require "time"

class JobScheduler
  QUEUES = %w[critical default mailers batch low unsafe].freeze
  MAX_JOBS_PER_QUEUE = 10_000
  MAX_UNSAFE_JOBS = 42
  MAX_RETRY_QUEUE = 20_000
  MAX_SCHEDULED_JOBS = 5_000
  SCHEDULE_BATCH_SIZE = 100
  ACTIVEJOB_WRAPPER = "Sidekiq::ActiveJob::Wrapper"
  ACTION_MAILER_DELIVERY = "ActionMailer::MailDeliveryJob"
  COMMON_TAGS = %w[tag-alpha tag-bravo tag-charlie tag-delta tag-echo].freeze

  JOB_DEFINITIONS = [
    {job: "EmailDeliveryJob", queue: "mailers", weight: 15,
     args: -> { [fake_email, fake_subject, %w[welcome reset_password invoice].sample] }},
    {job: "PaymentProcessingJob", queue: "critical", weight: 5,
     args: -> { [rand(100_000..999_999), rand(10.0..500.0).round(2), %w[USD EUR GBP].sample] }},
    {job: "ImageProcessingJob", queue: "default", weight: 10,
     args: -> { [rand(1..100_000), %w[resize crop thumbnail watermark].sample(rand(1..3))] }},
    {job: "ReportGenerationJob", queue: "batch", weight: 3,
     args: -> { [%w[sales inventory users activity].sample, "#{rand(1..12)}/2024", rand(1..1000)] }},
    {job: "NotificationJob", queue: "critical", weight: 20,
     args: -> { [rand(1..100_000), %w[push sms in_app].sample, {"message" => fake_message}] }},
    {job: "DataSyncJob", queue: "batch", weight: 5,
     args: -> { [%w[salesforce hubspot stripe].sample, "local_db", Array.new(rand(1..10)) { rand(1..10_000) }] }},
    {job: "CacheWarmupJob", queue: "low", weight: 8,
     args: -> { ["cache:#{%w[products users orders].sample}:#{rand(1..1000)}", %w[product user order].sample] }},
    {job: "AnalyticsJob", queue: "low", weight: 25,
     args: -> { [%w[page_view click purchase signup].sample, {"page" => "/page/#{rand(1..100)}"}, Time.now.to_i] }},
    {job: "CleanupJob", queue: "low", weight: 4,
     args: -> { [%w[temp_files sessions logs exports].sample, rand(7..90)] }},
    {job: "CleanupJob", queue: "low", weight: 1,
     args: -> { [] }},
    {job: "PurgeUserDataJob", queue: "unsafe", weight: 2,
     args: -> { [rand(1..100_000), %w[gdpr_request account_closure fraud_investigation].sample] }},
    {job: "WebhookDeliveryJob", queue: "default", weight: 10,
     args: -> { ["https://example.com/webhooks/#{rand(1..100)}", %w[order.created user.updated payment.completed].sample, {"id" => rand(1..10_000)}] }},
    {job: ACTIVEJOB_WRAPPER, queue: "default", weight: 3,
     payload: -> do
       active_job_payload(
         wrapped: "DemoActiveJob",
         queue: "default",
         arguments: [
           global_id_argument("User"),
           {"source" => %w[import api sync].sample}
         ]
       )
     end},
    {job: ACTIVEJOB_WRAPPER, queue: "mailers", weight: 3,
     payload: -> do
       active_job_payload(
         wrapped: ACTION_MAILER_DELIVERY,
         queue: "mailers",
         arguments: [
           "UserMailer",
           %w[weekly_digest welcome_email].sample,
           "deliver_now",
           mailer_delivery_payload
         ]
       )
     end},
    {job: "NotificationJob", queue: "critical", weight: 2,
     payload: -> { tagged_payload(NotificationJob, "critical", [rand(1..100_000), %w[push sms in_app].sample, {"message" => fake_message}], COMMON_TAGS) }},
    {job: "ImageProcessingJob", queue: "default", weight: 2,
     payload: -> { tagged_payload(ImageProcessingJob, "default", [rand(1..100_000), %w[resize crop thumbnail watermark].sample(rand(1..3))], COMMON_TAGS) }},
    {job: "CleanupJob", queue: "low", weight: 2,
     payload: -> { tagged_payload(CleanupJob, "low", [%w[temp_files sessions logs exports].sample, rand(7..90)], COMMON_TAGS) }}
  ].freeze

  def self.fake_email
    "user#{rand(1..100_000)}@example.com"
  end

  def self.fake_subject
    ["Welcome!", "Your order confirmation", "Password reset", "Weekly digest", "Invoice #%d" % rand(1000..9999)].sample
  end

  def self.fake_message
    ["New message received", "Your order shipped", "Payment confirmed", "Reminder: complete your profile"].sample
  end

  def self.global_id_for(model_name)
    "gid://lazykiq/#{model_name}/#{rand(1..100_000)}"
  end

  def self.global_id_argument(model_name)
    {"_aj_globalid" => global_id_for(model_name)}
  end

  def self.active_job_payload(wrapped:, queue:, arguments:)
    {
      "class" => ACTIVEJOB_WRAPPER,
      "queue" => queue,
      "wrapped" => wrapped,
      "args" => [
        {
          "job_class" => wrapped,
          "job_id" => SecureRandom.uuid,
          "provider_job_id" => nil,
          "queue_name" => queue,
          "priority" => nil,
          "arguments" => arguments,
          "executions" => 0,
          "exception_executions" => {},
          "locale" => "en",
          "timezone" => "UTC",
          "enqueued_at" => Time.now.utc.iso8601(6),
          "scheduled_at" => nil
        }
      ],
      "retry" => true
    }
  end

  def self.mailer_delivery_payload
    {
      "params" => {
        "user" => global_id_argument("User"),
        "_aj_symbol_keys" => ["user"]
      },
      "args" => [global_id_argument("Account"), rand(1..5)],
      "_aj_symbol_keys" => ["params", "args"]
    }
  end

  def self.tagged_payload(job_class, queue, args, tags)
    picked_tags = tags.sample(rand(1..3))
    {
      "class" => job_class,
      "queue" => queue,
      "args" => args,
      "tags" => picked_tags
    }
  end

  def initialize
    @weighted_jobs = build_weighted_job_list
    @running = false
  end

  def start
    @running = true
    puts "Starting job scheduler..."
    puts "Max jobs per queue: #{MAX_JOBS_PER_QUEUE} (unsafe: #{MAX_UNSAFE_JOBS})"
    puts "Max retry queue: #{MAX_RETRY_QUEUE}"
    puts "Max scheduled jobs: #{MAX_SCHEDULED_JOBS}"
    puts "Queues: #{QUEUES.join(", ")}"

    while @running
      maintain_queues
      maintain_scheduled
      sleep 0.5
    end
  end

  def stop
    @running = false
  end

  private

  def build_weighted_job_list
    JOB_DEFINITIONS.flat_map { |defn| Array.new(defn[:weight], defn) }
  end

  def maintain_queues
    retry_size = Sidekiq::RetrySet.new.size

    # Pause scheduling if retry queue is too large
    if retry_size >= MAX_RETRY_QUEUE
      puts "[#{Time.now.strftime("%H:%M:%S")}] Retry queue full (#{retry_size}), pausing scheduling..."
      return
    end

    queue_sizes = fetch_queue_sizes
    scheduled_sizes = fetch_scheduled_sizes

    QUEUES.each do |queue_name|
      current_size = queue_sizes[queue_name] || 0
      scheduled_size = scheduled_sizes[queue_name] || 0
      available_capacity = max_jobs_for_queue(queue_name) - current_size - scheduled_size

      next if available_capacity <= 0

      jobs_to_schedule = [available_capacity, SCHEDULE_BATCH_SIZE].min
      schedule_jobs_for_queue(queue_name, jobs_to_schedule)
    end
  end

  def fetch_queue_sizes
    QUEUES.each_with_object({}) do |queue_name, sizes|
      sizes[queue_name] = Sidekiq::Queue.new(queue_name).size
    end
  end

  def schedule_jobs_for_queue(queue_name, count)
    queue_jobs = @weighted_jobs.select { |j| j[:queue] == queue_name }
    return if queue_jobs.empty?

    count.times do
      job_def = queue_jobs.sample
      enqueue_job(job_def)
    end

    puts "[#{Time.now.strftime("%H:%M:%S")}] Scheduled #{count} jobs for queue '#{queue_name}'"
  end

  def maintain_scheduled
    scheduled_size = Sidekiq::ScheduledSet.new.size
    queue_sizes = fetch_queue_sizes
    scheduled_sizes = fetch_scheduled_sizes

    if scheduled_size < MAX_SCHEDULED_JOBS * 0.8
      jobs_to_add = [MAX_SCHEDULED_JOBS - scheduled_size, SCHEDULE_BATCH_SIZE].min
      add_scheduled_jobs(jobs_to_add, queue_sizes, scheduled_sizes) if jobs_to_add > 0
    end
  end

  def add_scheduled_jobs(count, queue_sizes, scheduled_sizes)
    added = 0
    attempts = 0
    max_attempts = count * 5

    while added < count && attempts < max_attempts
      attempts += 1
      job_def = @weighted_jobs.sample
      queue_name = job_def[:queue]
      current_size = queue_sizes[queue_name] || 0
      scheduled_size = scheduled_sizes[queue_name] || 0
      next if current_size + scheduled_size >= max_jobs_for_queue(queue_name)

      delay = rand(1..86400) # Schedule between 1 second and 24 hours
      enqueue_job(job_def, delay: delay)
      scheduled_sizes[queue_name] = scheduled_size + 1
      added += 1
    end

    puts "[#{Time.now.strftime("%H:%M:%S")}] Added #{added} scheduled jobs"
  end

  def fetch_scheduled_sizes
    sizes = Hash.new(0)
    Sidekiq::ScheduledSet.new.each do |job|
      queue_name = job.item["queue"]
      sizes[queue_name] += 1 if queue_name
    end
    sizes
  end

  def enqueue_job(job_def, delay: nil)
    if job_def[:payload]
      payload = job_def[:payload].call
      payload["cattr"] ||= {tenanant_id: rand(1..10_000)}
      payload["at"] = Time.now.to_f + delay if delay
      Sidekiq::Client.push(payload)
      return
    end

    job_class = Object.const_get(job_def[:job])
    if delay
      job_class.set(cattr: {tenanant_id: rand(1..10_000)}).perform_in(delay, *job_def[:args].call)
    else
      job_class.set(cattr: {tenanant_id: rand(1..10_000)}).perform_async(*job_def[:args].call)
    end
  end

  def max_jobs_for_queue(queue_name)
    return MAX_UNSAFE_JOBS if queue_name == "unsafe"

    MAX_JOBS_PER_QUEUE
  end
end
