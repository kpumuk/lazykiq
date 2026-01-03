package sidekiq

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

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

func TestDetectVersion_Sidekiq8(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|250102|12:00", "some-data")

	version := client.DetectVersion(testContext(t))
	if version != Version8 {
		t.Errorf("DetectVersion() = %v, want Version8", version)
	}
}

func TestDetectVersion_Sidekiq7(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|20250102|12:00", "some-data")

	version := client.DetectVersion(testContext(t))
	if version != Version7 {
		t.Errorf("DetectVersion() = %v, want Version7", version)
	}
}

func TestDetectVersion_NoMetricsKeys(t *testing.T) {
	_, client := setupTestRedis(t)

	version := client.DetectVersion(testContext(t))
	if version != VersionUnknown {
		t.Errorf("DetectVersion() = %v, want VersionUnknown", version)
	}
}

func TestDetectVersion_InvalidKeys(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|invalid", "data")
	_ = mr.Set("j|123", "data")
	_ = mr.Set("j|", "data")

	version := client.DetectVersion(testContext(t))
	if version != VersionUnknown {
		t.Errorf("DetectVersion() = %v, want VersionUnknown", version)
	}
}

func TestDetectVersion_Caching(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	_ = mr.Set("j|250102|12:00", "data")

	version1 := client.DetectVersion(ctx)
	if version1 != Version8 {
		t.Errorf("First call: DetectVersion() = %v, want Version8", version1)
	}

	mr.Del("j|250102|12:00")
	_ = mr.Set("j|20250102|12:00", "data")

	version2 := client.DetectVersion(ctx)
	if version2 != Version8 {
		t.Errorf("Second call: DetectVersion() = %v, want Version8 (cached)", version2)
	}
}

func TestDetectVersion_MultipleKeys(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|250102|12:00", "data")
	_ = mr.Set("j|250102|13:00", "data")
	_ = mr.Set("j|250102|14:00", "data")

	version := client.DetectVersion(testContext(t))
	if version != Version8 {
		t.Errorf("DetectVersion() = %v, want Version8", version)
	}
}

func TestDetectVersion_MixedValidAndInvalid(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|bad", "data")
	_ = mr.Set("j|250102|12:00", "data")
	_ = mr.Set("j|also-bad", "data")

	version := client.DetectVersion(testContext(t))
	if version != Version8 {
		t.Errorf("DetectVersion() = %v, want Version8", version)
	}
}

func TestDetectVersion_ShortKeys(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|1", "data")
	_ = mr.Set("j|", "data")
	_ = mr.Set("j", "data")

	version := client.DetectVersion(testContext(t))
	if version != VersionUnknown {
		t.Errorf("DetectVersion() = %v, want VersionUnknown", version)
	}
}

func TestDetectVersion_MixedSidekiq7And8(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|20250102|12:00", "data")
	_ = mr.Set("j|250102|13:00", "data")

	version := client.DetectVersion(testContext(t))
	if version != Version8 {
		t.Errorf("DetectVersion() = %v, want Version8 (should prefer Version8 over Version7)", version)
	}
}

func TestMetricsPeriodOrder_Sidekiq8(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|250102|12:00", "data")

	periods := client.MetricsPeriodOrder(testContext(t))
	expected := []string{"1h", "2h", "4h", "8h", "24h", "48h", "72h"}

	if len(periods) != len(expected) {
		t.Fatalf("MetricsPeriodOrder() length = %d, want %d", len(periods), len(expected))
	}

	for i, period := range periods {
		if period != expected[i] {
			t.Errorf("MetricsPeriodOrder()[%d] = %q, want %q", i, period, expected[i])
		}
	}
}

func TestMetricsPeriodOrder_Sidekiq7(t *testing.T) {
	mr, client := setupTestRedis(t)

	_ = mr.Set("j|20250102|12:00", "data")

	periods := client.MetricsPeriodOrder(testContext(t))
	expected := []string{"1h", "2h", "4h", "8h"}

	if len(periods) != len(expected) {
		t.Fatalf("MetricsPeriodOrder() length = %d, want %d", len(periods), len(expected))
	}

	for i, period := range periods {
		if period != expected[i] {
			t.Errorf("MetricsPeriodOrder()[%d] = %q, want %q", i, period, expected[i])
		}
	}
}

func TestMetricsPeriodOrder_Unknown(t *testing.T) {
	_, client := setupTestRedis(t)

	periods := client.MetricsPeriodOrder(testContext(t))
	expected := []string{"1h", "2h", "4h", "8h", "24h", "48h", "72h"}

	if len(periods) != len(expected) {
		t.Fatalf("MetricsPeriodOrder() length = %d, want %d", len(periods), len(expected))
	}

	for i, period := range periods {
		if period != expected[i] {
			t.Errorf("MetricsPeriodOrder()[%d] = %q, want %q", i, period, expected[i])
		}
	}
}

// setupTestRedis starts a miniredis instance and creates a Sidekiq client.
// Cleanup is handled automatically via t.Cleanup().
//
// Example usage:
//
//	mr, client := setupTestRedis(t)
//	// Use mr for data setup
//	// Use client for Sidekiq operations
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *Client) {
	t.Helper()

	mr := miniredis.RunT(t)

	client := &Client{
		redis: redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		}),
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return mr, client
}
