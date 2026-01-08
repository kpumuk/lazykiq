package sidekiq

import (
	"encoding/json"
	"math"
	"strconv"
	"time"
)

// parseOptionalInt64 parses various types to int64 with success indication.
// Handles string (including empty), float64, int64, and int.
func parseOptionalInt64(field any) (int64, bool) {
	switch value := field.(type) {
	case string:
		if value == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case float64:
		return int64(value), true
	case int64:
		return value, true
	case int:
		return int64(value), true
	}
	return 0, false
}

// parseOptionalFloat64 parses various types to float64 with success indication.
func parseOptionalFloat64(field any) (float64, bool) {
	switch value := field.(type) {
	case string:
		if value == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case float64:
		return value, true
	case int64:
		return float64(value), true
	case int:
		return float64(value), true
	}
	return 0, false
}

// parseOptionalBool parses various types to bool with success indication.
func parseOptionalBool(field any) (bool, bool) {
	switch value := field.(type) {
	case string:
		if value == "" {
			return false, false
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, false
		}
		return parsed, true
	case bool:
		return value, true
	}
	return false, false
}

// parseTimestamp normalizes a Sidekiq timestamp to time.Time.
func parseTimestamp(raw any) time.Time {
	seconds, ok := parseTimestampSeconds(raw)
	if !ok || seconds <= 0 {
		return time.Time{}
	}

	// The created_at, enqueued_at, failed_at and retried_at attributes are stored as epoch
	// milliseconds starting with Sidekiq 8, rather than epoch floats. This is meant to avoid
	// precision issues with JSON and JavaScript's 53-bit Floats.
	//
	// Example: "created_at" => 1234567890.123456 -> "created_at" => 1234567890123.
	//
	// To maintain compatibility with older Sidekiq versions, we check for timestamps
	// that are larger than 1e12 (approximately 2001-09-09T01:46:40Z) and treat them as
	// milliseconds.
	if seconds > 1e12 {
		return time.UnixMilli(int64(math.Round(seconds)))
	}

	return time.Unix(0, int64(seconds*float64(time.Second)))
}

// parseTimestampSeconds extracts a float64 timestamp from various types. It does not guarantee
// the units of the timestamp (seconds vs milliseconds), only that it is a float64 representation.
func parseTimestampSeconds(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case int64:
		return float64(value), true
	case int:
		return float64(value), true
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return parsed, true
		}
		if parsed, err := value.Int64(); err == nil {
			return float64(parsed), true
		}
		return 0, false
	default:
		return 0, false
	}
}
