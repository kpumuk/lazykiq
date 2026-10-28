package display

import (
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

type badJSON struct{}

func (badJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New("boom")
}

type countedJSON struct{ calls *int }

func (v countedJSON) MarshalJSON() ([]byte, error) {
	*v.calls++
	return []byte(`"custom"`), nil
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

func TestArgsPreview(t *testing.T) {
	for name, tc := range map[string]struct {
		args []any
		want string
	}{
		"empty":         {want: ""},
		"short":         {args: []any{"foo", 1, map[string]any{"a": "b"}}, want: `"foo", 1, {"a":"b"}`},
		"marshal error": {args: []any{badJSON{}}, want: "{}"},
		"at limit":      {args: []any{strings.Repeat("x", 198)}, want: `"` + strings.Repeat("x", 198) + `"`},
		"over limit":    {args: []any{strings.Repeat("x", 199)}, want: `"` + strings.Repeat("x", 198) + "…"},
		"large":         {args: []any{strings.Repeat("x", 32*1024)}, want: `"` + strings.Repeat("x", 198) + "…"},
		"many args":     {args: []any{strings.Repeat("x", 196), 123, "tail"}, want: `"` + strings.Repeat("x", 196) + `",…`},
		"wide unicode":  {args: []any{strings.Repeat("界", 1024)}, want: `"` + strings.Repeat("界", 99) + "…"},
		"graphemes":     {args: []any{strings.Repeat("e\u0301", 1024)}, want: `"` + strings.Repeat("e\u0301", 198) + "…"},
	} {
		t.Run(name, func(t *testing.T) {
			got := ArgsPreview(tc.args)
			if got != tc.want {
				t.Fatalf("ArgsPreview() = %q, want %q", got, tc.want)
			}
			if !utf8.ValidString(got) || ansi.StringWidth(got) > 200 {
				t.Fatalf("preview is invalid UTF-8 or wider than 200 columns: %q", got)
			}
		})
	}
}

func TestArgsPreviewCompatibility(t *testing.T) {
	var deep any = "leaf"
	for range 1100 {
		deep = []any{deep}
	}
	wide := strings.Repeat("payload", 1024)
	for name, args := range map[string][]any{
		"scalars":                     {nil, true, false, int64(-42), uint64(math.MaxUint64), 1e-9, 1e21, float32(1.25)},
		"nested":                      {map[string]any{"z": []any{true, nil, "tail"}, "a": map[string]any{"b": wide}}},
		"nil containers":              {[]any(nil), map[string]any(nil), []any{}, map[string]any{}},
		"many arguments":              {strings.Repeat("a", 196), "\n\t\"\\", "tail"},
		"sorted keys":                 {map[string]any{"z": "last", "a": wide, "b": "middle"}},
		"wide key":                    {map[string]any{wide: "value"}},
		"invalid UTF-8":               {string([]byte{'a', 0xff, 0xfe, 'b'}), strings.Repeat("\x80", 512)},
		"escapes":                     {strings.Repeat("\n\r\t\b\f\x00\x1b\"\\<>&\u2028\u2029", 512)},
		"combining":                   {strings.Repeat("e\u0301", 512)},
		"zero width":                  {strings.Repeat("\u200b", 512)},
		"long cluster":                {"e" + strings.Repeat("\u0301", 512)},
		"escaped cluster boundary":    {strings.Repeat("x", 254) + "\u2028\ufe0f\u20e3"},
		"typed slice":                 {[]string{wide, "tail"}},
		"typed map":                   {map[int]string{2: "two", 1: wide}},
		"raw JSON":                    {json.RawMessage(`{"nested":[1,"two"]}`)},
		"invalid raw JSON":            {json.RawMessage(`{"invalid"`)},
		"custom marshaler":            {badJSON{}, "tail"},
		"hidden marshaler":            {wide, badJSON{}},
		"hidden nested marshal error": {[]any{wide, badJSON{}}},
		"hidden unsupported value":    {map[string]any{"a": wide, "z": make(chan int)}},
		"hidden NaN":                  {[]any{wide, math.NaN()}},
		"hidden infinity":             {map[string]any{"a": wide, "z": math.Inf(1)}},
		"deep fallback":               {deep},
	} {
		t.Run(name, func(t *testing.T) {
			want := ansi.Truncate(Args(args), 200, "…")
			if got := ArgsPreview(args); got != want {
				t.Fatalf("ArgsPreview() = %q, want %q", got, want)
			}
		})
	}
}

func TestArgsPreviewUnicodeLimits(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want string
	}{
		"keycaps":            {strings.Repeat("1️⃣", 512), `"` + strings.Repeat("1️⃣", 99) + "…"},
		"emoji clusters":     {strings.Repeat("👩🏽‍💻🇨🇦1️⃣", 512), `"` + strings.Repeat("👩🏽‍💻🇨🇦1️⃣", 33) + "…"},
		"keycap at boundary": {strings.Repeat("x", 197) + "1️⃣tail", `"` + strings.Repeat("x", 197) + "…"},
		"escaped keycap":     {strings.Repeat("x", 150) + strings.Repeat("<\ufe0f\u20e3", 10), `"` + strings.Repeat("x", 150) + strings.Repeat(`\u003c`+"\ufe0f\u20e3", 6) + `\u003…`},
	} {
		t.Run(name, func(t *testing.T) {
			got := ArgsPreview([]any{tc.text})
			if got != tc.want || !utf8.ValidString(got) || ansi.StringWidth(got) > 200 {
				t.Fatalf("ArgsPreview() = %q (width %d), want %q", got, ansi.StringWidth(got), tc.want)
			}
		})
	}
}

func TestArgsPreviewPreservesHiddenCustomMarshaler(t *testing.T) {
	calls := 0
	args := []any{strings.Repeat("x", 1024), countedJSON{calls: &calls}}
	if got := ArgsPreview(args); got != `"`+strings.Repeat("x", 198)+"…" {
		t.Fatalf("unexpected preview: %q", got)
	}
	if calls != 1 {
		t.Fatalf("hidden custom marshaler called %d times, want once", calls)
	}
}

func TestArgsPreviewBoundedAllocations(t *testing.T) {
	const iterations = 32
	for name, unit := range map[string]string{"ascii": "x", "unicode": "界", "escaped": "\n", "emoji": "👩🏽‍💻"} {
		t.Run(name, func(t *testing.T) {
			args := []any{strings.Repeat(unit, 1024*1024/len(unit))}
			var before, after runtime.MemStats
			var preview string
			runtime.GC()
			runtime.ReadMemStats(&before)
			for range iterations {
				preview = ArgsPreview(args)
			}
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(preview)
			if bytes := (after.TotalAlloc - before.TotalAlloc) / iterations; bytes > 32*1024 {
				t.Fatalf("1 MiB preview allocates %d bytes/op, want at most 32 KiB", bytes)
			}
		})
	}
}

func FuzzArgsPreview(f *testing.F) {
	for _, raw := range []string{
		`[]`, `[null,true,1,1e-9,{"z":"last","a":["first"]}]`,
		`["\u2028\ufe0f\u20e3\n\"\\<>&","👩🏽‍💻é🇨🇦"]`,
		`["` + strings.Repeat("界", 200) + `"]`,
		`["` + strings.Repeat("x", 197) + `👩🏽‍💻"]`,
	} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var args []any
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return
		}
		full := Args(args)
		got := ArgsPreview(args)
		if !utf8.ValidString(got) || ansi.StringWidth(got) > 200 {
			t.Fatalf("invalid or oversized preview: %q", got)
		}
		if ansi.StringWidth(full) <= 200 {
			if got != full {
				t.Fatalf("short preview = %q, want %q", got, full)
			}
		} else if !strings.HasSuffix(got, "…") || !strings.HasPrefix(full, strings.TrimSuffix(got, "…")) {
			t.Fatalf("truncated preview is not a prefix with an ellipsis: %q", got)
		}
	})
}

func TestShortNumber(t *testing.T) {
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
			if got := ShortNumber(tt.n); got != tt.want {
				t.Fatalf("Number(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestCompactNumber(t *testing.T) {
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
			if got := CompactNumber(tt.n); got != tt.want {
				t.Fatalf("CompactNumber(%d) = %q, want %q", tt.n, got, tt.want)
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
		{name: "negative", n: -999, want: "-999"},
		{name: "kilo", n: 1000, want: "1,000"},
		{name: "kilo-fraction", n: 1500, want: "1,500"},
		{name: "mega", n: 1_000_000, want: "1,000,000"},
		{name: "giga", n: 1_000_000_000, want: "1,000,000,000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Number(tt.n); got != tt.want {
				t.Fatalf("Number(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestFloat(t *testing.T) {
	tests := []struct {
		name      string
		f         float64
		precision int
		want      string
	}{
		{name: "plain", f: 999, precision: 2, want: "999.00"},
		{name: "negative", f: -999, precision: 2, want: "-999.00"},
		{name: "negative-precision", f: -999, want: "-999.0", precision: 1},
		{name: "kilo", f: 1000.123, precision: 2, want: "1,000.12"},
		{name: "kilo-fraction", f: 1500.123, precision: 2, want: "1,500.12"},
		{name: "mega", f: 1_000_000.123, precision: 2, want: "1,000,000.12"},
		{name: "mega-fraction", f: 1_000_000.123, precision: 2, want: "1,000,000.12"},
		{name: "mega-precision", f: 1_000_000.123, precision: 0, want: "1,000,000"},
		{name: "giga", f: 1_000_000_000, precision: 2, want: "1,000,000,000.00"},
		{name: "giga-fraction", f: 1_000_000_000.123, precision: 2, want: "1,000,000,000.12"},
		{name: "giga-precision", f: 1_000_000_000.123, precision: 1, want: "1,000,000,000.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Float(tt.f, tt.precision); got != tt.want {
				t.Fatalf("Float(%f) = %q, want %q", tt.f, got, tt.want)
			}
		})
	}
}
