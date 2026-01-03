package sidekiq

import "strconv"

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
