// Package format provides UI formatting helpers.
package format

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Duration formats elapsed seconds as "2m3s", "1h30m", etc. (max 2 segments).
func Duration(seconds int64) string {
	seconds = max(seconds, 0)

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

var nowFunc = time.Now

// DurationSince formats elapsed time since the given timestamp.
// Returns "-" if time is zero.
func DurationSince(start time.Time) string {
	if start.IsZero() {
		return "-"
	}
	return Duration(int64(nowFunc().Sub(start).Seconds()))
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
func Args(args []any) string {
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

// ShortNumber formats a number with K/M suffixes for readability.
func ShortNumber(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// Number formats a number with thousands separators (e.g., 1,234,567).
func Number(n int64) string {
	if n < 0 {
		return "-" + Number(-n)
	}
	s := strconv.FormatInt(n, 10)
	return addThousandsSeparators(s)
}

// Float formats a float with thousands separators (e.g., 1,234.56).
func Float(f float64, precision int) string {
	if f < 0 {
		return "-" + Float(-f, precision)
	}

	s := fmt.Sprintf("%.*f", precision, f)

	// Split into integer and decimal parts
	intPart, decPart := s, ""
	if idx := strings.Index(s, "."); idx >= 0 {
		intPart = s[:idx]
		decPart = s[idx:]
	}

	return addThousandsSeparators(intPart) + decPart
}

func addThousandsSeparators(s string) string {
	if len(s) <= 3 {
		return s
	}

	// Insert commas from right to left
	var result strings.Builder
	result.Grow(len(s) + (len(s)-1)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// CompactNumber formats a number into a compact 4-char max string (e.g., 999, 9.9K, 120K).
func CompactNumber(n int64) string {
	switch {
	case n < 1_000:
		return strconv.FormatInt(n, 10)
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
