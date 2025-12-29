// Package format provides UI formatting helpers.
package format

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Duration formats elapsed seconds as "2m3s", "1h30m", etc. (max 2 segments).
func Duration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}

	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	secs := seconds % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh%dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm%ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// Bytes formats bytes as "168 MB", "1.2 GB", etc.
func Bytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Args formats job arguments as JSON without outer brackets.
func Args(args []interface{}) string {
	if len(args) == 0 {
		return ""
	}

	parts := make([]string, 0, len(args))
	for _, arg := range args {
		b, err := json.Marshal(arg)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%v", arg))
		} else {
			parts = append(parts, string(b))
		}
	}

	return strings.Join(parts, ", ")
}

// Number formats a number with K/M suffixes for readability.
func Number(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ShortNumber formats a number into a compact 4-char max string (e.g., 999, 9.9K, 120K).
func ShortNumber(n int64) string {
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	case n < 1_000_000:
		return fmt.Sprintf("%dK", n/1_000)
	case n < 10_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n < 1_000_000_000:
		return fmt.Sprintf("%dM", n/1_000_000)
	case n < 10_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	default:
		return fmt.Sprintf("%dB", n/1_000_000_000)
	}
}
