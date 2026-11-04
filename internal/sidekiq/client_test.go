package sidekiq

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestNewClient_InvalidURLDoesNotDiscloseCredentials(t *testing.T) {
	const secret = "synthetic-credential"
	for name, test := range map[string]struct {
		input string
		want  string
	}{
		"path escape":              {"redis://user:" + secret + "@localhost/%zz", "invalid URL escape"},
		"password escape":          {"redis://user:" + secret + "%zz@localhost", "invalid URL escape"},
		"encoded password":         {"rediss://user:synthetic%2Dcredential@localhost/%zz", "invalid URL escape"},
		"port":                     {"redis://user:" + secret + "@localhost:bad", "invalid URL syntax"},
		"control character":        {"redis://user:" + secret + "@localhost/\n", "invalid URL syntax"},
		"database":                 {"redis://user:" + secret + "@localhost/" + secret, "invalid database number"},
		"path":                     {"redis://user:" + secret + "@localhost/a/" + secret, "invalid URL path"},
		"scheme":                   {secret + "://user:" + secret + "@localhost", "invalid URL scheme"},
		"option value":             {"redis://user:" + secret + "@localhost?pool_size=" + secret, "invalid Redis URL option value"},
		"option name":              {"redis://user:" + secret + "@localhost?" + secret + "=1", "unexpected option"},
		"password option rejected": {"redis://localhost?password=" + secret, "unexpected option"},
	} {
		t.Run(name, func(t *testing.T) {
			client, err := NewClient(test.input)
			if client != nil {
				_ = client.Close()
				t.Fatal("invalid URL created a client")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected diagnostic %q, got %v", test.want, err)
			}
			if strings.Contains(fmt.Sprintf("%+v", err), secret) || strings.Contains(err.Error(), test.input) {
				t.Fatal("parse error disclosed sensitive URL input")
			}
		})
	}
}

func TestRedisURLParseError_WrappedURL(t *testing.T) {
	const secret = "synthetic-credential"
	inner := &url.Error{Op: "parse", URL: "redis://user:" + secret + "@localhost/%zz", Err: url.EscapeError("%zz")}
	err := redisURLParseError(fmt.Errorf("outer: %w", inner))
	if !strings.Contains(err.Error(), "invalid URL escape") || strings.Contains(err.Error(), "synthetic-credential") {
		t.Fatalf("unsafe or missing diagnostic: %v", err)
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("sanitized error retains unsafe cause")
	}
}

func TestNewClient_ValidURLPreservesOptions(t *testing.T) {
	const encodedSecret = "synthetic%2Dcredential"
	for name, input := range map[string]string{
		"tcp":  "redis://user:" + encodedSecret + "@[::1]:6380/2?pool_fifo=true",
		"tls":  "rediss://user:" + encodedSecret + "@localhost:6380/2?pool_fifo=true",
		"unix": "unix://user:" + encodedSecret + "@/tmp/redis.sock?db=2&pool_fifo=true",
	} {
		t.Run(name, func(t *testing.T) {
			client, err := NewClient(input)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			opts := client.Redis().Options()
			if opts.Username != "user" || opts.Password != "synthetic-credential" || opts.DB != 2 || !opts.PoolFIFO {
				t.Fatal("valid URL options changed")
			}
			if strings.Contains(client.DisplayRedisURL(), "credential") {
				t.Fatal("display URL disclosed password")
			}
		})
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func redisTestURL(user, pass string) string {
	u := &url.URL{
		Scheme: "redis",
		Host:   "localhost:6379",
		Path:   "/0",
	}
	switch {
	case user != "" && pass != "":
		u.User = url.UserPassword(user, pass)
	case user != "":
		u.User = url.User(user)
	case pass != "":
		u.User = url.UserPassword("", pass)
	}
	return u.String()
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
			input:    redisTestURL("user", "credential"),
			expected: "redis://user@localhost:6379/0",
		},
		{
			name:     "password only",
			input:    redisTestURL("", "credential"),
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
			if strings.Contains(got, "credential") || strings.Contains(got, "password=") || strings.Contains(got, "pass=") {
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

func TestDetectVersion_CachesUnknown(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	version1 := client.DetectVersion(ctx)
	if version1 != VersionUnknown {
		t.Errorf("First call: DetectVersion() = %v, want VersionUnknown", version1)
	}

	_ = mr.Set("j|250102|12:00", "data")

	version2 := client.DetectVersion(ctx)
	if version2 != VersionUnknown {
		t.Errorf("Second call: DetectVersion() = %v, want VersionUnknown (cached)", version2)
	}
}

func TestDetectVersion_RetriesFailedScan(t *testing.T) {
	for name, failure := range map[string]struct {
		context func() context.Context
		err     string
	}{
		"cancelled": {context: func() context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx
		}},
		"deadline": {context: func() context.Context {
			ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			cancel()
			return ctx
		}},
		"redis error": {context: t.Context, err: "ERR temporary scan failure"},
	} {
		t.Run(name, func(t *testing.T) {
			for key, version := range map[string]Version{
				"j|20250102|12:00": Version7,
				"j|250102|12:00":   Version8,
			} {
				t.Run(key, func(t *testing.T) {
					mr, client := setupTestRedis(t)
					_ = mr.Set(key, "data")
					mr.SetError(failure.err)
					if got := client.DetectVersion(failure.context()); got != VersionUnknown {
						t.Fatalf("failed detection = %v, want VersionUnknown", got)
					}
					mr.SetError("")
					periods := client.MetricsPeriodOrder(t.Context())
					wantPeriods := MetricsPeriodOrder
					if version == Version7 {
						wantPeriods = MetricsPeriodOrderSidekiq7
					}
					if !slices.Equal(periods, wantPeriods) {
						t.Errorf("recovered periods = %v, want %v", periods, wantPeriods)
					}
					if got := client.DetectVersion(t.Context()); got != version {
						t.Errorf("recovered detection = %v, want %v", got, version)
					}
				})
			}
		})
	}
}

func TestDetectVersion_RetriesAfterPartialScanFailure(t *testing.T) {
	calls := 0
	client := newSelectionHookClient(t, func(_ context.Context, cmd *redis.ScanCmd) error {
		calls++
		switch calls {
		case 1:
			cmd.SetVal([]string{"j|20250102|12:00"}, 1)
		case 2:
			return errors.New("temporary scan failure")
		default:
			cmd.SetVal([]string{"j|250102|12:00"}, 0)
		}
		return nil
	})
	if got := client.DetectVersion(t.Context()); got != VersionUnknown {
		t.Fatalf("partial scan failure = %v, want VersionUnknown", got)
	}
	if got := client.DetectVersion(t.Context()); got != Version8 {
		t.Fatalf("recovered detection = %v, want Version8", got)
	}
	if calls != 3 {
		t.Errorf("SCAN calls = %d, want 3", calls)
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

func TestNewClient_ConfiguresRequestBackpressure(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := NewClient("redis://" + mr.Addr() + "/0")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	opts := client.Redis().Options()
	if !opts.ContextTimeoutEnabled {
		t.Fatal("ContextTimeoutEnabled = false, want true")
	}
	if opts.PoolSize != uiRedisPoolSize {
		t.Fatalf("PoolSize = %d, want %d", opts.PoolSize, uiRedisPoolSize)
	}
	if opts.MaxActiveConns != uiRedisPoolSize {
		t.Fatalf("MaxActiveConns = %d, want %d", opts.MaxActiveConns, uiRedisPoolSize)
	}
	if opts.PipelinePoolSize != -1 {
		t.Fatalf("PipelinePoolSize = %d, want -1", opts.PipelinePoolSize)
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
