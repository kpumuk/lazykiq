package sidekiq

import (
	"testing"
	"time"
)

func TestParseOptionalInt64(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		want   int64
		wantOk bool
	}{
		{"nil", nil, 0, false},
		{"string number", "123", 123, true},
		{"string negative", "-456", -456, true},
		{"string zero", "0", 0, true},
		{"string invalid", "not-a-number", 0, false},
		{"string empty", "", 0, false},
		{"int64", int64(999), 999, true},
		{"int64 negative", int64(-111), -111, true},
		{"int", int(42), 42, true},
		{"float64", float64(3.14), 3, true},
		{"float64 negative", float64(-2.71), -2, true},
		{"unknown type", struct{}{}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseOptionalInt64(tt.input)
			if got != tt.want {
				t.Errorf("parseOptionalInt64(%v) value = %d, want %d", tt.input, got, tt.want)
			}
			if ok != tt.wantOk {
				t.Errorf("parseOptionalInt64(%v) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
		})
	}
}

func TestParseOptionalFloat64(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		want   float64
		wantOk bool
	}{
		{"nil", nil, 0, false},
		{"string number", "3.14", 3.14, true},
		{"string negative", "-2.71", -2.71, true},
		{"string zero", "0", 0, true},
		{"string invalid", "not-a-number", 0, false},
		{"string empty", "", 0, false},
		{"float64", float64(1.5), 1.5, true},
		{"float64 negative", float64(-9.99), -9.99, true},
		{"int64", int64(42), 42.0, true},
		{"int", int(7), 7.0, true},
		{"unknown type", true, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseOptionalFloat64(tt.input)
			if got != tt.want {
				t.Errorf("parseOptionalFloat64(%v) value = %f, want %f", tt.input, got, tt.want)
			}
			if ok != tt.wantOk {
				t.Errorf("parseOptionalFloat64(%v) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
		})
	}
}

func TestParseOptionalBool(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		want   bool
		wantOk bool
	}{
		{"nil", nil, false, false},
		{"string true", "true", true, true},
		{"string false", "false", false, true},
		{"string 1", "1", true, true},
		{"string 0", "0", false, true},
		{"string invalid", "invalid", false, false},
		{"string empty", "", false, false},
		{"bool true", true, true, true},
		{"bool false", false, false, true},
		{"unknown type", 42, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseOptionalBool(tt.input)
			if got != tt.want {
				t.Errorf("parseOptionalBool(%v) value = %v, want %v", tt.input, got, tt.want)
			}
			if ok != tt.wantOk {
				t.Errorf("parseOptionalBool(%v) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected time.Time
	}{
		{
			name:     "float seconds",
			input:    1234.5,
			expected: time.Unix(1234, 500*int64(time.Millisecond)),
		},
		{
			name:     "milliseconds int",
			input:    int64(1568305717946),
			expected: time.UnixMilli(1568305717946),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestamp(tt.input)
			if !got.Equal(tt.expected) {
				t.Fatalf("parseTimestamp(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseTimestamp_Invalid(t *testing.T) {
	if got := parseTimestamp("not a timestamp"); !got.IsZero() {
		t.Fatalf("parseTimestamp(invalid) = %v, want zero", got)
	}
}
