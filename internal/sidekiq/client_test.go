package sidekiq

import (
	"strings"
	"testing"
)

func TestSanitizeRedisURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain url",
			input:    "redis://localhost:6379/0",
			expected: "redis://localhost:6379/0",
		},
		{
			name:     "user with password",
			input:    "redis://user:secret@localhost:6379/0",
			expected: "redis://user@localhost:6379/0",
		},
		{
			name:     "password only",
			input:    "redis://:secret@localhost:6379/0",
			expected: "redis://localhost:6379/0",
		},
		{
			name:     "user without password",
			input:    "redis://user@localhost:6379/0",
			expected: "redis://user@localhost:6379/0",
		},
		{
			name:     "invalid url returns input",
			input:    "redis://%zz",
			expected: "redis://%zz",
		},
	}

	for i := range tests {
		test := tests[i]
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeRedisURL(test.input)
			if got != test.expected {
				t.Fatalf("sanitizeRedisURL(%q) = %q, want %q", test.input, got, test.expected)
			}
			if strings.Contains(got, "secret") || strings.Contains(got, "password=") || strings.Contains(got, "pass=") {
				t.Fatalf("sanitizeRedisURL(%q) leaked credentials: %q", test.input, got)
			}
		})
	}
}
