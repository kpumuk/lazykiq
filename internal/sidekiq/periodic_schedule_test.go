package sidekiq

import (
	"testing"
	"time"
)

func TestPeriodicNextRun(t *testing.T) {
	for name, test := range map[string]struct {
		schedule, timezone, fallback, now, want string
	}{
		"minute boundary":        {schedule: "*/5 * * * *", now: "2026-10-30T11:05:00Z", want: "2026-10-30T11:10:00Z"},
		"weekday names":          {schedule: "0 9 * JAN,MAR MON-FRI", now: "2026-10-30T11:05:00Z", want: "2027-01-01T09:00:00Z"},
		"Sunday seven":           {schedule: "0 0 * * 7", now: "2026-10-30T00:00:00Z", want: "2026-11-01T00:00:00Z"},
		"Sunday range":           {schedule: "0 0 * * 5-7", now: "2026-10-31T00:00:00Z", want: "2026-11-01T00:00:00Z"},
		"DOM DOW union":          {schedule: "0 0 15 * 1", now: "2026-10-30T00:00:00Z", want: "2026-11-02T00:00:00Z"},
		"stepped wildcard union": {schedule: "0 0 */2 * 1", now: "2026-10-30T00:00:00Z", want: "2026-10-31T00:00:00Z"},
		"daily alias":            {schedule: "@daily", now: "2026-10-30T00:00:00Z", want: "2026-10-31T00:00:00Z"},
		"monthly alias":          {schedule: "@monthly", now: "2026-10-30T00:00:00Z", want: "2026-11-01T00:00:00Z"},
		"leap century":           {schedule: "0 0 29 2 *", now: "2096-03-01T00:00:00Z", want: "2104-02-29T00:00:00Z"},
		"named zone":             {schedule: "0 9 * * *", timezone: "Asia/Tokyo", now: "2026-10-30T00:00:00Z", want: "2026-10-31T09:00:00+09:00"},
		"server default":         {schedule: "0 9 * * *", fallback: "America/Toronto", now: "2026-10-30T00:00:00Z", want: "2026-10-30T09:00:00-04:00"},
		"Rails zone":             {schedule: "0 9 * * *", timezone: "Eastern Time (US & Canada)", now: "2026-10-30T00:00:00Z", want: "2026-10-30T09:00:00-04:00"},
		"spring DST gap":         {schedule: "30 2 * * *", timezone: "America/New_York", now: "2026-03-08T06:00:00Z", want: "2026-03-09T02:30:00-04:00"},
		"fall DST repeat":        {schedule: "30 1 * * *", timezone: "America/New_York", now: "2026-11-01T05:30:00Z", want: "2026-11-01T01:30:00-05:00"},
	} {
		t.Run(name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, test.now)
			if err != nil {
				t.Fatal(err)
			}
			job := PeriodicJob{Schedule: test.schedule, Timezone: test.timezone, DefaultTimezone: test.fallback}
			next, err := job.NextRun(now)
			if err != nil || next.Format(time.RFC3339) != test.want {
				t.Fatalf("NextRun = %v, %v; want %s", next, err, test.want)
			}
		})
	}
}

func TestPeriodicInvalidSchedule(t *testing.T) {
	for _, schedule := range []string{"", "@reboot", "@every 1h", "* * * * * *", "0 0 * * 8", "0 0 * * 7-1", "0 0 * * */0", "0 0 * * 1/2", "0 0 * * 7/2", "0 0 * * 1/2/3", "0 0 * * nope", "0 0 31 2 *", "0 25 * * *"} {
		t.Run(schedule, func(t *testing.T) {
			if _, err := (PeriodicJob{Schedule: schedule}).NextRun(time.Now()); err == nil {
				t.Fatal("invalid schedule accepted")
			}
		})
	}
	if _, err := (PeriodicJob{Schedule: "* * * * *", Timezone: "Invalid/Zone"}).NextRun(time.Now()); err == nil {
		t.Fatal("invalid timezone accepted")
	}
	if _, err := NewClient("redis://localhost:6379", WithPeriodicTimezone("Invalid/Zone")); err == nil {
		t.Fatal("invalid server timezone accepted")
	}
}
