package sidekiq

import (
	"encoding/json"
	"testing"
)

func TestSafeParseJSONPreservesNumbers(t *testing.T) {
	input := `{"int":1,"float":1.5,"floatIntLike":1.0,"exp":1e3,"neg":-2.25}`

	var data map[string]any
	if err := safeParseJSON([]byte(input), &data); err != nil {
		t.Fatalf("safeParseJSON: %v", err)
	}

	assertNumber := func(key, want string) {
		t.Helper()
		raw, ok := data[key]
		if !ok {
			t.Fatalf("missing key %q", key)
		}
		num, ok := raw.(json.Number)
		if !ok {
			t.Fatalf("key %q type = %T, want json.Number", key, raw)
		}
		if num.String() != want {
			t.Fatalf("key %q value = %q, want %q", key, num.String(), want)
		}
	}

	assertNumber("int", "1")
	assertNumber("float", "1.5")
	assertNumber("floatIntLike", "1.0")
	assertNumber("exp", "1e3")
	assertNumber("neg", "-2.25")

	round, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var redecoded map[string]any
	if err := safeParseJSON(round, &redecoded); err != nil {
		t.Fatalf("safeParseJSON roundtrip: %v", err)
	}

	for key, want := range map[string]string{
		"int":          "1",
		"float":        "1.5",
		"floatIntLike": "1.0",
		"exp":          "1e3",
		"neg":          "-2.25",
	} {
		raw := redecoded[key]
		num, ok := raw.(json.Number)
		if !ok {
			t.Fatalf("roundtrip key %q type = %T, want json.Number", key, raw)
		}
		if num.String() != want {
			t.Fatalf("roundtrip key %q value = %q, want %q", key, num.String(), want)
		}
	}
}

func TestDetectTimestampFormat(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		version Version
		want    timestampFormat
	}{
		{
			name: "seconds float",
			payload: map[string]any{
				"enqueued_at": 1700000000.5,
			},
			version: Version8,
			want:    timestampSecondsFloat,
		},
		{
			name: "milliseconds int64",
			payload: map[string]any{
				"created_at": int64(1700000000123),
			},
			version: Version7,
			want:    timestampMilliseconds,
		},
		{
			name: "seconds json number",
			payload: map[string]any{
				"retried_at": json.Number("1700000000.123"),
			},
			version: VersionUnknown,
			want:    timestampSecondsFloat,
		},
		{
			name: "milliseconds json number",
			payload: map[string]any{
				"failed_at": json.Number("1700000000123"),
			},
			version: VersionUnknown,
			want:    timestampMilliseconds,
		},
		{
			name:    "fallback version7",
			payload: map[string]any{},
			version: Version7,
			want:    timestampSecondsFloat,
		},
		{
			name:    "fallback version8",
			payload: map[string]any{},
			version: Version8,
			want:    timestampMilliseconds,
		},
		{
			name:    "fallback unknown",
			payload: map[string]any{},
			version: VersionUnknown,
			want:    timestampMilliseconds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectTimestampFormat(tt.payload, tt.version); got != tt.want {
				t.Fatalf("detectTimestampFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}
