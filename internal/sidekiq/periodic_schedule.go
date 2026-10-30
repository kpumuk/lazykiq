package sidekiq

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Keep named zones available in standalone release binaries.

	"github.com/robfig/cron/v3"
)

// NextRun calculates a cron occurrence using the definition's effective timezone.
func (p PeriodicJob) NextRun(now time.Time) (time.Time, error) {
	location, err := periodicLocation(p.EffectiveTimezone())
	if err != nil {
		return time.Time{}, err
	}
	expression := p.Schedule
	aliases := map[string]string{
		"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
		"@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *",
	}
	if alias, ok := aliases[expression]; ok {
		expression = alias
	}
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return time.Time{}, errors.New("periodic schedule must have five cron fields")
	}
	weekday, err := periodicWeekday(fields[4])
	if err != nil {
		return time.Time{}, err
	}
	parsedFields := append([]string(nil), fields...)
	parsedFields[4] = weekday
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	parsed, err := parser.Parse(strings.Join(parsedFields, " "))
	if err != nil {
		return time.Time{}, err
	}
	schedule := parsed.(*cron.SpecSchedule)
	schedule.Location = location
	// Enterprise treats only a literal * as unrestricted for DOM/DOW matching.
	// The cron library also marks stepped wildcards, so remove those markers.
	if fields[2] != "*" {
		schedule.Dom &^= 1 << 63
	}
	if fields[4] != "*" {
		schedule.Dow &^= 1 << 63
	}
	next := schedule.Next(now.In(location))
	if next.IsZero() {
		// The parser searches five years; leap days can be eight years apart.
		next = schedule.Next(now.In(location).AddDate(4, 0, 0))
	}
	if next.IsZero() {
		return time.Time{}, errors.New("periodic schedule has no next occurrence")
	}
	return next, nil
}

func periodicLocation(name string) (*time.Location, error) {
	if alias, ok := railsTimezones[name]; ok {
		name = alias
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("periodic timezone %q: %w", name, err)
	}
	return location, nil
}

// Enterprise accepts both 0 and 7 for Sunday, including within ranges.
func periodicWeekday(field string) (string, error) {
	if field == "*" {
		return field, nil
	}
	field = strings.ToLower(field)
	for index, name := range []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"} {
		field = strings.ReplaceAll(field, name, strconv.Itoa(index))
	}
	var days [7]bool
	for part := range strings.SplitSeq(field, ",") {
		interval := strings.Split(part, "/")
		if len(interval) > 2 {
			return "", errors.New("invalid periodic weekday step")
		}
		step := 1
		if len(interval) == 2 {
			var err error
			step, err = strconv.Atoi(interval[1])
			if err != nil || step < 1 {
				return "", errors.New("invalid periodic weekday step")
			}
		}
		start, end := 0, 6
		if len(interval) == 2 && interval[0] != "*" && !strings.Contains(interval[0], "-") {
			return "", errors.New("periodic weekday step requires a wildcard or range")
		}
		if interval[0] != "*" {
			bounds := strings.Split(interval[0], "-")
			var err error
			start, err = strconv.Atoi(bounds[0])
			if err != nil || len(bounds) > 2 {
				return "", errors.New("invalid periodic weekday")
			}
			end = start
			if len(bounds) == 2 {
				end, err = strconv.Atoi(bounds[1])
				if err != nil {
					return "", err
				}
			}
		}
		if start < 0 || end > 7 || start > end {
			return "", errors.New("periodic weekday outside 0..7")
		}
		for day := start; day <= end; {
			days[day%7] = true
			if step > end-day {
				break
			}
			day += step
		}
	}
	var values []string
	for day, included := range days {
		if included {
			values = append(values, strconv.Itoa(day))
		}
	}
	return strings.Join(values, ","), nil
}
