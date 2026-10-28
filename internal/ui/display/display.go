// Package display provides UI formatting helpers.
package display

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

// ArgsPreview formats job arguments for tables, capped at 200 terminal columns.
func ArgsPreview(args []any) string {
	if len(args) == 0 {
		return ""
	}
	// Check the whole value shape before skipping invisible content. A marshal
	// error anywhere in an argument must still select Args' original fallback.
	// Small inputs also use the simpler formatter; their escaped size is bounded.
	size := previewJSONSize(args, 0)
	if size < 0 {
		return ansi.Truncate(Args(args), 200, "…")
	}
	if size <= argsPreviewSmallSize {
		return truncateArgsPreview(Args(args))
	}

	p := argsPreview{data: make([]byte, 0, 256)}
	for i, arg := range args {
		if i > 0 {
			p.write(", ")
		}
		p.value(arg)
		if p.full {
			break
		}
	}
	return truncateArgsPreview(string(p.data))
}

const argsPreviewSmallSize = 512

// truncateArgsPreview clips JSON text at complete graphemes, including keycaps
// that start with an ASCII digit. JSON escaping removes terminal control codes.
func truncateArgsPreview(s string) string {
	if ansi.StringWidth(s) <= 200 {
		return s
	}
	if strings.IndexFunc(s, func(r rune) bool { return r < ' ' || r > '~' }) < 0 {
		return s[:199] + "…"
	}
	end, columns := 0, 0
	for end < len(s) {
		cluster, width := ansi.FirstGraphemeCluster(s[end:], ansi.GraphemeWidth)
		if columns+width > 199 {
			break
		}
		end += len(cluster)
		columns += width
	}
	return s[:end] + "…"
}

// previewJSONSize recognizes decoded JSON values and ordinary Go numbers. It
// estimates their size before string escaping, capped above the small-input
// threshold. A negative result preserves encoding/json's handling of other
// types, custom marshalers, invalid values and excessively deep/cyclic inputs.
func previewJSONSize(value any, depth int) int {
	if depth > 1000 {
		return -1
	}
	size := 0
	switch v := value.(type) {
	case string:
		return 2 + min(len(v), argsPreviewSmallSize+1)
	case nil, bool:
		return 5
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr:
		return 24
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return -1
		}
		return 24
	case float32:
		return previewJSONSize(float64(v), depth)
	case []any:
		size = 2 + min(len(v), argsPreviewSmallSize+1)
		for _, item := range v {
			child := previewJSONSize(item, depth+1)
			if child < 0 {
				return -1
			}
			size = min(size+child, argsPreviewSmallSize+1)
		}
	case map[string]any:
		size = 2
		for key, item := range v {
			child := previewJSONSize(item, depth+1)
			if child < 0 {
				return -1
			}
			size = min(size+min(len(key), argsPreviewSmallSize+1)+4+child, argsPreviewSmallSize+1)
		}
	default:
		return -1
	}
	return size
}

type argsPreview struct {
	data      []byte
	processed int
	columns   int
	full      bool
}

func (p *argsPreview) value(value any) {
	if p.full {
		return
	}
	switch v := value.(type) {
	case string:
		p.quoted(v)
	case []any:
		if v == nil {
			p.write("null")
			return
		}
		p.write("[")
		for i, item := range v {
			if i > 0 {
				p.write(",")
			}
			p.value(item)
			if p.full {
				return
			}
		}
		p.write("]")
	case map[string]any:
		if v == nil {
			p.write("null")
			return
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		p.write("{")
		for i, key := range keys {
			if i > 0 {
				p.write(",")
			}
			p.quoted(key)
			p.write(":")
			p.value(v[key])
			if p.full {
				return
			}
		}
		p.write("}")
	default:
		// Shape validation excludes unsupported values and non-finite floats.
		encoded, _ := json.Marshal(value)
		p.write(string(encoded))
	}
}

func (p *argsPreview) quoted(s string) {
	if p.full {
		return
	}
	p.write(`"`)
	for len(s) > 0 && !p.full {
		// Quote only a small block, using the standard library for all escaping
		// and invalid UTF-8 handling. Keep complete source graphemes together.
		end := 0
		for end < len(s) && end < 256 {
			cluster, _ := ansi.FirstGraphemeCluster(s[end:], ansi.GraphemeWidth)
			end += len(cluster)
		}
		encoded, _ := json.Marshal(s[:end])
		p.write(string(encoded[1 : len(encoded)-1]))
		s = s[end:]
	}
	p.write(`"`)
}

func (p *argsPreview) write(s string) {
	if p.full {
		return
	}
	p.data = append(p.data, s...)
	for p.processed < len(p.data) {
		remaining := p.data[p.processed:]
		cluster, width := ansi.FirstGraphemeCluster(remaining, ansi.GraphemeWidth)
		if len(cluster) == len(remaining) {
			// A later block can extend this grapheme (for example a combining
			// mark after an escaped digit). Count it only once its end is known.
			return
		}
		p.processed += len(cluster)
		p.columns += width
		if p.columns > 200 {
			p.data = p.data[:p.processed]
			p.full = true
			return
		}
	}
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

// HorizontalScroll cuts a plain text line by display width and pads it to the
// requested visible width without breaking ANSI escape sequences.
func HorizontalScroll(line string, offset, visibleWidth int) string {
	if visibleWidth <= 0 {
		return ""
	}
	offset = max(offset, 0)

	cut := ansi.Cut(line, offset, offset+visibleWidth)
	cutWidth := lipgloss.Width(cut)
	if cutWidth < visibleWidth {
		cut += strings.Repeat(" ", visibleWidth-cutWidth)
	}
	return cut
}
