package views

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

func limiterValue(value sidekiq.LimiterValue, divisor float64, unit string) string {
	if value.Raw == "" {
		return "Unavailable"
	}
	if !value.Valid {
		return "Invalid: " + value.Raw
	}
	if divisor != 1 {
		return strconv.FormatFloat(value.Number/divisor, 'f', -1, 64) + unit
	}
	return value.Raw + unit
}

func limiterHistoryRows(limiter *sidekiq.Limiter) []table.Row {
	if limiter == nil {
		return nil
	}
	rows := make([]table.Row, 0, len(limiter.History))
	for _, sample := range limiter.History {
		at := sample.At
		switch limiter.Interval {
		case "second":
			at = at.Truncate(time.Second)
		case "minute":
			at = at.Truncate(time.Minute)
		case "hour":
			at = at.Add(-time.Duration(at.Minute())*time.Minute - time.Duration(at.Second())*time.Second - time.Duration(at.Nanosecond()))
		case "day":
			at = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
		}
		value := limiterValue(sample.Value, 1, "")
		if sample.Note != "" {
			value = sample.Note
		}
		rows = append(rows, table.Row{ID: at.Format(time.RFC3339), Cells: []string{at.Format("2006-01-02 15:04:05 -0700"), value}})
	}
	return rows
}

func limiterPropertyRows(l *sidekiq.Limiter) []table.Row {
	if l == nil || l.Missing {
		return nil
	}
	var rows []table.Row
	add := func(label, value string) { rows = append(rows, table.Row{ID: label, Cells: []string{label, value}}) }
	metric := func(label, field string, divisor float64, unit string) {
		add(label, limiterValue(l.Values[field], divisor, unit))
	}
	add("Name", l.Name)
	add("Redis key", l.Key)
	add("Type", l.Type)
	add("Capacity / rate", l.Rate())
	if len(l.Problems) > 0 {
		add("Record warning", strings.Join(l.Problems, "; "))
	}
	switch l.Type {
	case "Concurrent":
		metric("Observed capacity", "size", 1, " slots")
		metric("Used capacity", "used", 1, " slots")
		metric("Free capacity", "free", 1, " slots")
		metric("Held count", "held", 1, "")
		metric("Held duration", "held_ms", 1000, " s")
		metric("Immediate acquisitions", "immediate", 1, "")
		metric("Waited count", "waited", 1, "")
		metric("Wait duration", "wait_ms", 1000, " s")
		metric("Overtime count", "overtime", 1, "")
		metric("Reclaimed locks", "reclaimed", 1, "")
		add("Capacity semantics", "Observed free list + used set; expired locks are not reclaimed by inspection.")
	case "Window", "Bucket":
		metric("Configured size", "size", 1, " operations")
		interval := l.Interval
		if _, err := strconv.ParseFloat(interval, 64); err == nil {
			interval += " s"
		}
		if interval == "" {
			interval = "Unavailable"
		}
		add("Configured interval", interval)
		add("Acquisition statistics", "Not recorded for this type")
		if l.Type == "Window" {
			metric("Current window counter", "window_counter", 1, "")
			metric("Window expires in", "window_ttl_ms", 1000, " s")
			add("Counter semantics", "Recorded attempted units in the active window, including over-limit attempts.")
		}
		if l.Type == "Bucket" {
			add("Counter semantics", "Recorded attempts, including over-limit attempts. Absent counters are unavailable.")
		}
	case "Leaky bucket", "Points":
		unit, state := "operations", "drops"
		if l.Type == "Points" {
			unit, state = "points", "points"
		}
		metric("Burst capacity", "size", 1, " "+unit)
		add("Full "+map[bool]string{true: "refill", false: "drain"}[l.Type == "Points"]+" interval", l.Interval+" s")
		seconds, err := strconv.ParseFloat(l.Interval, 64)
		if size := l.Values["size"]; size.Valid && err == nil && seconds > 0 {
			add("Configured rate", fmt.Sprintf("%g %s/s", size.Number/seconds, unit))
		}
		metric("Recorded "+state, state, 1, "")
		metric("Hit count", "hit", 1, "")
		metric("Miss count", "miss", 1, "")
		metric("Wait duration", "waited", 1, " s")
		last := "Unavailable"
		if value := l.Values["last"]; value.Valid {
			last = time.Unix(int64(value.Number), int64((value.Number-float64(int64(value.Number)))*1e9)).UTC().Format(time.RFC3339Nano)
		}
		add("Last accounting time", last)
		add("State semantics", "Last persisted state; elapsed-time drain/refill is not simulated.")
	default:
		add("Availability", "Unsupported limiter type; this record does not prevent inspecting other limiters.")
	}
	ttl := "No expiry"
	if l.TTL >= 0 {
		ttl = l.TTL.String()
	}
	add("Record TTL", ttl)
	add("Observed at", l.ObservedAt.UTC().Format(time.RFC3339))
	add("Inspection scope", "Configuration is limited to persisted fields. Wait/lock timeout and policy are not stored.")
	return rows
}
