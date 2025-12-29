package sidekiq

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JobRecord represents a pending job within a Sidekiq queue.
// Mirrors Sidekiq::JobRecord
type JobRecord struct {
	value string                 // the underlying String in Redis
	item  map[string]interface{} // the parsed job data
	queue string                 // the queue associated with this job

	args               []interface{}
	displayArgs        []interface{}
	displayArgsLoaded  bool
	displayClass       string
	displayClassLoaded bool
	errorBacktrace     []string
	errorBacktraceSet  bool
}

// NewJobRecord creates a JobRecord from raw JSON data.
// If queueName is empty, it will be extracted from the parsed JSON item.
func NewJobRecord(value string, queueName string) *JobRecord {
	jr := &JobRecord{
		value: value,
		queue: queueName,
	}

	if err := json.Unmarshal([]byte(value), &jr.item); err != nil {
		jr.item = make(map[string]interface{})
		jr.args = []interface{}{value}
	}

	// Extract queue from item if not provided
	if jr.queue == "" {
		if q, ok := jr.item["queue"].(string); ok {
			jr.queue = q
		}
	}

	return jr
}

// Queue returns the queue name associated with this job.
func (jr *JobRecord) Queue() string {
	return jr.queue
}

// JID returns the job ID.
func (jr *JobRecord) JID() string {
	if jid, ok := jr.item["jid"].(string); ok {
		return jid
	}
	return ""
}

// Klass returns the job class which Sidekiq will execute.
func (jr *JobRecord) Klass() string {
	if klass, ok := jr.item["class"].(string); ok {
		return klass
	}
	return ""
}

// DisplayClass returns a human-friendly class name, unwrapping known wrappers.
func (jr *JobRecord) DisplayClass() string {
	if jr.displayClassLoaded {
		return jr.displayClass
	}

	klass := jr.Klass()
	displayClass := klass

	// Unwrap ActiveJob wrapper
	if klass == "ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper" || klass == "Sidekiq::ActiveJob::Wrapper" {
		if wrapped, ok := jr.item["wrapped"].(string); ok {
			displayClass = wrapped
		} else if args := jr.Args(); len(args) > 0 {
			if firstArg, ok := args[0].(string); ok {
				displayClass = firstArg
			}
		}

		if displayClass == "ActionMailer::DeliveryJob" || displayClass == "ActionMailer::MailDeliveryJob" {
			args := jr.Args()
			if argsMap, ok := firstArgsMap(args); ok {
				if rawArgs, ok := argsMap["arguments"]; ok {
					if deserialized, ok := deserializeArgument(rawArgs).([]interface{}); ok && len(deserialized) >= 2 {
						mailer, okMailer := deserialized[0].(string)
						method, okMethod := deserialized[1].(string)
						if okMailer && okMethod {
							displayClass = mailer + "#" + method
						}
					}
				}
			}
		}
	}

	jr.displayClass = displayClass
	jr.displayClassLoaded = true
	return displayClass
}

// Args returns the job arguments.
func (jr *JobRecord) Args() []interface{} {
	if jr.args != nil {
		return jr.args
	}
	if args, ok := jr.item["args"].([]interface{}); ok {
		jr.args = args
		return jr.args
	}
	return nil
}

// DisplayArgs returns arguments unwrapped for ActiveJob and other known wrappers.
func (jr *JobRecord) DisplayArgs() []interface{} {
	if jr.displayArgsLoaded {
		return jr.displayArgs
	}

	klass := jr.Klass()
	if klass == "ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper" || klass == "Sidekiq::ActiveJob::Wrapper" {
		args := jr.Args()
		jobArgs := []interface{}{}
		wrapped, hasWrapped := jr.item["wrapped"].(string)
		if hasWrapped {
			if argsMap, ok := firstArgsMap(args); ok {
				if rawArgs, ok := argsMap["arguments"]; ok {
					if deserialized, ok := deserializeArgument(rawArgs).([]interface{}); ok {
						jobArgs = deserialized
					}
				}
			}
		}

		jobClass := wrapped
		if !hasWrapped {
			if len(args) > 0 {
				if first, ok := args[0].(string); ok {
					jobClass = first
				}
			}
		}

		switch jobClass {
		case "ActionMailer::DeliveryJob":
			if len(jobArgs) > 3 {
				jobArgs = jobArgs[3:]
			} else {
				jobArgs = []interface{}{}
			}
		case "ActionMailer::MailDeliveryJob":
			if len(jobArgs) > 3 {
				jobArgs = jobArgs[3:]
			} else {
				jobArgs = []interface{}{}
			}
			if len(jobArgs) > 0 {
				if paramsMap, ok := jobArgs[0].(map[string]interface{}); ok {
					jobArgs = []interface{}{paramsMap["params"], paramsMap["args"]}
				} else {
					jobArgs = []interface{}{}
				}
			}
		}

		jr.displayArgs = jobArgs
		jr.displayArgsLoaded = true
		return jr.displayArgs
	}

	args := jr.Args()
	if len(args) == 0 {
		jr.displayArgs = nil
		jr.displayArgsLoaded = true
		return jr.displayArgs
	}

	displayArgs := make([]interface{}, len(args))
	copy(displayArgs, args)

	if encrypted, ok := jr.item["encrypt"].(bool); ok && encrypted {
		displayArgs[len(displayArgs)-1] = "[encrypted data]"
	} else if jr.item["encrypt"] != nil {
		displayArgs[len(displayArgs)-1] = "[encrypted data]"
	}

	jr.displayArgs = displayArgs
	jr.displayArgsLoaded = true
	return jr.displayArgs
}

// Context returns the current attributes (cattr) for the job.
func (jr *JobRecord) Context() map[string]interface{} {
	if cattr, ok := jr.item["cattr"].(map[string]interface{}); ok {
		return cattr
	}
	return nil
}

// Item returns the full parsed job data.
func (jr *JobRecord) Item() map[string]interface{} {
	return jr.item
}

// Value returns the raw JSON string from Redis.
func (jr *JobRecord) Value() string {
	return jr.value
}

// ErrorClass returns the error class if this job failed.
func (jr *JobRecord) ErrorClass() string {
	if errClass, ok := jr.item["error_class"].(string); ok {
		return errClass
	}
	return ""
}

// ErrorMessage returns the error message if this job failed.
func (jr *JobRecord) ErrorMessage() string {
	if errMsg, ok := jr.item["error_message"].(string); ok {
		return errMsg
	}
	return ""
}

// HasError returns true if this job has error information.
func (jr *JobRecord) HasError() bool {
	_, ok := jr.item["error_class"]
	return ok
}

// RetryCount returns the number of times this job has been retried.
func (jr *JobRecord) RetryCount() int {
	if rc, ok := jr.item["retry_count"].(float64); ok {
		return int(rc)
	}
	return 0
}

// FailedAt returns the timestamp when the job failed (0 if not failed).
func (jr *JobRecord) FailedAt() float64 {
	if failedAt, ok := jr.item["failed_at"].(float64); ok {
		return failedAt
	}
	return 0
}

// RetriedAt returns the timestamp of the last retry (0 if never retried).
func (jr *JobRecord) RetriedAt() float64 {
	if retriedAt, ok := jr.item["retried_at"].(float64); ok {
		return retriedAt
	}
	return 0
}

// Bid returns the batch ID.
func (jr *JobRecord) Bid() string {
	if bid, ok := jr.item["bid"].(string); ok {
		return bid
	}
	return ""
}

// EnqueuedAt returns the enqueued timestamp (0 if missing).
func (jr *JobRecord) EnqueuedAt() float64 {
	if enqueuedAt, ok := jr.item["enqueued_at"].(float64); ok {
		return enqueuedAt
	}
	return 0
}

// CreatedAt returns the created timestamp, falling back to enqueued_at (0 if missing).
func (jr *JobRecord) CreatedAt() float64 {
	if createdAt, ok := jr.item["created_at"].(float64); ok {
		return createdAt
	}
	if enqueuedAt, ok := jr.item["enqueued_at"].(float64); ok {
		return enqueuedAt
	}
	return 0
}

// Tags returns any tags associated with the job.
func (jr *JobRecord) Tags() []string {
	rawTags, ok := jr.item["tags"].([]interface{})
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(rawTags))
	for _, raw := range rawTags {
		if tag, ok := raw.(string); ok {
			tags = append(tags, tag)
		} else {
			tags = append(tags, fmt.Sprint(raw))
		}
	}
	return tags
}

// ErrorBacktrace returns the decoded error backtrace lines, if present.
func (jr *JobRecord) ErrorBacktrace() []string {
	if jr.errorBacktraceSet {
		return jr.errorBacktrace
	}
	jr.errorBacktraceSet = true

	switch raw := jr.item["error_backtrace"].(type) {
	case []string:
		jr.errorBacktrace = raw
		return jr.errorBacktrace
	case []interface{}:
		lines := make([]string, 0, len(raw))
		for _, line := range raw {
			lines = append(lines, fmt.Sprint(line))
		}
		jr.errorBacktrace = lines
		return jr.errorBacktrace
	case string:
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil
		}
		reader, err := zlib.NewReader(bytes.NewReader(decoded))
		if err != nil {
			return nil
		}
		defer func() {
			_ = reader.Close()
		}()
		var backtrace []string
		if err := json.NewDecoder(reader).Decode(&backtrace); err != nil {
			return nil
		}
		jr.errorBacktrace = backtrace
		return jr.errorBacktrace
	default:
		return nil
	}
}

// Latency returns the time since enqueue/create in seconds.
func (jr *JobRecord) Latency() float64 {
	timestamp := jr.EnqueuedAt()
	if timestamp == 0 {
		timestamp = jr.CreatedAt()
	}
	if timestamp == 0 {
		return 0
	}

	if timestamp > 1e12 {
		nowMs := float64(time.Now().UnixMilli())
		return (nowMs - timestamp) / 1000.0
	}
	nowSec := float64(time.Now().Unix())
	return nowSec - timestamp
}

func deserializeArgument(argument interface{}) interface{} {
	switch value := argument.(type) {
	case []interface{}:
		out := make([]interface{}, len(value))
		for i, item := range value {
			out[i] = deserializeArgument(item)
		}
		return out
	case map[string]interface{}:
		if isSerializedGlobalID(value) {
			return value[globalIDKey]
		}
		out := make(map[string]interface{}, len(value))
		for key, item := range value {
			if strings.HasPrefix(key, activeJobPrefix) {
				continue
			}
			out[key] = deserializeArgument(item)
		}
		return out
	default:
		return value
	}
}

func isSerializedGlobalID(value map[string]interface{}) bool {
	if len(value) != 1 {
		return false
	}
	_, ok := value[globalIDKey]
	return ok
}

func firstArgsMap(args []interface{}) (map[string]interface{}, bool) {
	if len(args) == 0 {
		return nil, false
	}
	if argsMap, ok := args[0].(map[string]interface{}); ok {
		return argsMap, true
	}
	return nil, false
}

const (
	activeJobPrefix = "_aj_"
	globalIDKey     = "_aj_globalid"
)
