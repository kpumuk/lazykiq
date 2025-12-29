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
// Mirrors Sidekiq::JobRecord.
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
	if isActiveJobWrapper(klass) {
		displayClass = jr.unwrapActiveJobDisplayClass(displayClass)
	}

	jr.displayClass = displayClass
	jr.displayClassLoaded = true
	return displayClass
}

func (jr *JobRecord) unwrapActiveJobDisplayClass(displayClass string) string {
	if wrapped, ok := jr.item["wrapped"].(string); ok {
		displayClass = wrapped
	} else if firstArg, ok := firstStringArg(jr.Args()); ok {
		displayClass = firstArg
	}

	if !isActionMailerWrapper(displayClass) {
		return displayClass
	}

	argsMap, ok := firstArgsMap(jr.Args())
	if !ok {
		return displayClass
	}
	rawArgs, ok := argsMap["arguments"]
	if !ok {
		return displayClass
	}
	deserialized, ok := deserializeArgument(rawArgs).([]interface{})
	if !ok || len(deserialized) < 2 {
		return displayClass
	}
	mailer, okMailer := deserialized[0].(string)
	method, okMethod := deserialized[1].(string)
	if !okMailer || !okMethod {
		return displayClass
	}
	return mailer + "#" + method
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
	if isActiveJobWrapper(klass) {
		jr.displayArgs = jr.unwrapActiveJobArgs()
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

	encrypted, ok := jr.item["encrypt"].(bool)
	if (ok && encrypted) || (!ok && jr.item["encrypt"] != nil) {
		displayArgs[len(displayArgs)-1] = "[encrypted data]"
	}

	jr.displayArgs = displayArgs
	jr.displayArgsLoaded = true
	return jr.displayArgs
}

func (jr *JobRecord) unwrapActiveJobArgs() []interface{} {
	args := jr.Args()
	wrapped, hasWrapped := jr.item["wrapped"].(string)
	jobArgs := []interface{}{}
	if hasWrapped {
		jobArgs = extractActiveJobArgs(args)
	}

	jobClass := wrapped
	if !hasWrapped {
		if first, ok := firstStringArg(args); ok {
			jobClass = first
		}
	}

	switch jobClass {
	case actionMailerDeliveryJob:
		return trimActionMailerArgs(jobArgs)
	case actionMailerMailDeliveryJob:
		return normalizeMailDeliveryArgs(jobArgs)
	default:
		return jobArgs
	}
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

func isActiveJobWrapper(klass string) bool {
	return klass == activeJobAdapterWrapper || klass == activeJobWrapper
}

func isActionMailerWrapper(klass string) bool {
	return klass == actionMailerDeliveryJob || klass == actionMailerMailDeliveryJob
}

func firstStringArg(args []interface{}) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	value, ok := args[0].(string)
	return value, ok
}

func extractActiveJobArgs(args []interface{}) []interface{} {
	argsMap, ok := firstArgsMap(args)
	if !ok {
		return []interface{}{}
	}
	rawArgs, ok := argsMap["arguments"]
	if !ok {
		return []interface{}{}
	}
	deserialized, ok := deserializeArgument(rawArgs).([]interface{})
	if !ok {
		return []interface{}{}
	}
	return deserialized
}

func trimActionMailerArgs(args []interface{}) []interface{} {
	if len(args) <= 3 {
		return []interface{}{}
	}
	return args[3:]
}

func normalizeMailDeliveryArgs(args []interface{}) []interface{} {
	args = trimActionMailerArgs(args)
	if len(args) == 0 {
		return []interface{}{}
	}
	paramsMap, ok := args[0].(map[string]interface{})
	if !ok {
		return []interface{}{}
	}
	return []interface{}{paramsMap["params"], paramsMap["args"]}
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
	activeJobPrefix             = "_aj_"
	globalIDKey                 = "_aj_globalid"
	activeJobAdapterWrapper     = "ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper"
	activeJobWrapper            = "Sidekiq::ActiveJob::Wrapper"
	actionMailerDeliveryJob     = "ActionMailer::DeliveryJob"
	actionMailerMailDeliveryJob = "ActionMailer::MailDeliveryJob"
)
