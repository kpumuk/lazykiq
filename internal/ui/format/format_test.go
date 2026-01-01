package format

import (
	"errors"
	"testing"
	"time"
)

type badJSON struct{}

func (badJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New("boom")
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name    string
		seconds int64
		want    string
	}{
		{name: "negative", seconds: -5, want: "0s"},
		{name: "seconds", seconds: 59, want: "59s"},
		{name: "minute", seconds: 60, want: "1m0s"},
		{name: "minute-seconds", seconds: 61, want: "1m1s"},
		{name: "hour", seconds: 3600, want: "1h0m"},
		{name: "hour-minutes", seconds: 3661, want: "1h1m"},
		{name: "day", seconds: 86400, want: "1d0h"},
		{name: "day-hour", seconds: 90061, want: "1d1h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Duration(tt.seconds); got != tt.want {
				t.Fatalf("Duration(%d) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}

func TestDurationSince(t *testing.T) {
	fixedNow := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	restoreNow := nowFunc
	nowFunc = func() time.Time { return fixedNow }
	t.Cleanup(func() { nowFunc = restoreNow })

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "zero", at: time.Time{}, want: "-"},
		{name: "seconds", at: fixedNow.Add(-59 * time.Second), want: "59s"},
		{name: "minute", at: fixedNow.Add(-90 * time.Second), want: "1m30s"},
		{name: "future", at: fixedNow.Add(10 * time.Second), want: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DurationSince(tt.at); got != tt.want {
				t.Fatalf("DurationSince(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "bytes", bytes: 512, want: "512 B"},
		{name: "kilobyte", bytes: 1024, want: "1.0 KB"},
		{name: "kilobyte-fraction", bytes: 1536, want: "1.5 KB"},
		{name: "megabyte", bytes: 1024 * 1024, want: "1.0 MB"},
		{name: "gigabyte", bytes: 1024 * 1024 * 1024, want: "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Bytes(tt.bytes); got != tt.want {
				t.Fatalf("Bytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestArgs(t *testing.T) {
	tests := []struct {
		name string
		args []any
		want string
	}{
		{name: "empty", args: nil, want: ""},
		{
			name: "json",
			args: []any{
				"foo",
				1,
				map[string]any{"a": "b"},
			},
			want: `"foo", 1, {"a":"b"}`,
		},
		{
			name: "marshal-error",
			args: []any{
				badJSON{},
			},
			want: "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Args(tt.args); got != tt.want {
				t.Fatalf("Args(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestNumber(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{name: "plain", n: 999, want: "999"},
		{name: "kilo", n: 1000, want: "1.0K"},
		{name: "kilo-fraction", n: 1500, want: "1.5K"},
		{name: "mega", n: 1_000_000, want: "1.0M"},
		{name: "giga", n: 1_000_000_000, want: "1.0B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Number(tt.n); got != tt.want {
				t.Fatalf("Number(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestShortNumber(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{name: "plain", n: 999, want: "999"},
		{name: "kilo-decimal", n: 1000, want: "1.0K"},
		{name: "kilo-decimal-round", n: 9999, want: "10.0K"},
		{name: "kilo-whole", n: 10_000, want: "10K"},
		{name: "kilo-max", n: 999_999, want: "999K"},
		{name: "mega-decimal", n: 1_000_000, want: "1.0M"},
		{name: "mega-decimal-round", n: 9_999_999, want: "10.0M"},
		{name: "mega-whole", n: 10_000_000, want: "10M"},
		{name: "mega-max", n: 999_999_999, want: "999M"},
		{name: "giga-decimal", n: 1_000_000_000, want: "1.0B"},
		{name: "giga-whole", n: 12_345_678_901, want: "12B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortNumber(tt.n); got != tt.want {
				t.Fatalf("ShortNumber(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
