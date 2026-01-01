package sidekiq

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestParseProcessInfoQueuesAndWeights(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"hostname":    "host",
		"started_at":  1700000000.5,
		"pid":         1234,
		"tag":         "app",
		"concurrency": 10,
		"queues":      []any{"high", "default", "low", "single"},
		"weights": []any{
			map[string]any{"high": 3, "default": 2, "low": 1},
			map[string]any{"single": 0},
		},
		"labels":   []any{"alpha"},
		"identity": "host:1234:abc",
		"version":  "7.0.0",
		"embedded": false,
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	if process.Hostname != "host" {
		t.Fatalf("Hostname = %q, want %q", process.Hostname, "host")
	}
	if process.PID != 1234 {
		t.Fatalf("PID = %d, want %d", process.PID, 1234)
	}
	if process.Tag != "app" {
		t.Fatalf("Tag = %q, want %q", process.Tag, "app")
	}
	if process.Concurrency != 10 {
		t.Fatalf("Concurrency = %d, want %d", process.Concurrency, 10)
	}
	expectedStartedAt := time.Unix(0, int64(1700000000.5*float64(time.Second)))
	if !process.StartedAt.Equal(expectedStartedAt) {
		t.Fatalf("StartedAt = %v, want %v", process.StartedAt, expectedStartedAt)
	}

	expectedWeights := map[string]int{
		"high":    3,
		"default": 2,
		"low":     1,
		"single":  0,
	}
	if len(process.Capsules) != 1 {
		t.Fatalf("Capsules len = %d, want %d", len(process.Capsules), 1)
	}
	if !reflect.DeepEqual(process.Capsules[DefaultCapsuleName].Weights, expectedWeights) {
		t.Fatalf("Capsules[default].Weights = %#v, want %#v", process.Capsules[DefaultCapsuleName].Weights, expectedWeights)
	}
}

func TestParseProcessInfoQueueWeightsInline(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"queues":  []any{"high,1", "default,2", "low,3"},
		"weights": []any{map[string]any{"high": 4}},
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	expectedWeights := map[string]int{
		"high":    4,
		"default": 2,
		"low":     3,
	}
	if !reflect.DeepEqual(process.Capsules[DefaultCapsuleName].Weights, expectedWeights) {
		t.Fatalf("Capsules[default].Weights = %#v, want %#v", process.Capsules[DefaultCapsuleName].Weights, expectedWeights)
	}
}

func TestParseProcessInfoLegacyWeights(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"queues":  []any{"alpha", "beta"},
		"weights": map[string]any{"alpha": 2, "beta": 1},
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	expectedWeights := map[string]int{
		"alpha": 2,
		"beta":  1,
	}
	if !reflect.DeepEqual(process.Capsules[DefaultCapsuleName].Weights, expectedWeights) {
		t.Fatalf("Capsules[default].Weights = %#v, want %#v", process.Capsules[DefaultCapsuleName].Weights, expectedWeights)
	}
}

func TestParseProcessInfoCapsulesOverride(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"queues":  []any{"default", "low"},
		"weights": []any{map[string]any{"default": 1, "low": 1}},
		"capsules": map[string]any{
			"default": map[string]any{
				"concurrency": 8,
				"mode":        "weighted",
				"weights":     map[string]any{"default": 5, "low": 1},
			},
			"unsafe": map[string]any{
				"concurrency": 1,
				"mode":        "strict",
				"weights":     map[string]any{"unsafe": 0},
			},
		},
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	if len(process.Capsules) != 2 {
		t.Fatalf("Capsules len = %d, want %d", len(process.Capsules), 2)
	}
	if process.Capsules["unsafe"].Concurrency != 1 {
		t.Fatalf("Capsules[unsafe].Concurrency = %d, want %d", process.Capsules["unsafe"].Concurrency, 1)
	}

	expectedDefaultWeights := map[string]int{
		"default": 5,
		"low":     1,
	}
	if !reflect.DeepEqual(process.Capsules[DefaultCapsuleName].Weights, expectedDefaultWeights) {
		t.Fatalf("Capsules[default].Weights = %#v, want %#v", process.Capsules[DefaultCapsuleName].Weights, expectedDefaultWeights)
	}
	expectedUnsafeWeights := map[string]int{"unsafe": 0}
	if !reflect.DeepEqual(process.Capsules["unsafe"].Weights, expectedUnsafeWeights) {
		t.Fatalf("Capsules[unsafe].Weights = %#v, want %#v", process.Capsules["unsafe"].Weights, expectedUnsafeWeights)
	}
}

func TestParseProcessInfoDefaultCapsuleFromQueues(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"queues": []any{"default", "low"},
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	expectedWeights := map[string]int{
		"default": 0,
		"low":     0,
	}
	if !reflect.DeepEqual(process.Capsules[DefaultCapsuleName].Weights, expectedWeights) {
		t.Fatalf("Capsules[default].Weights = %#v, want %#v", process.Capsules[DefaultCapsuleName].Weights, expectedWeights)
	}
	if process.Capsules[DefaultCapsuleName].Mode != "strict" {
		t.Fatalf("Capsules[default].Mode = %q, want %q", process.Capsules[DefaultCapsuleName].Mode, "strict")
	}
}

func TestParseProcessInfoCapsulesDuplicateQueues(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"capsules": map[string]any{
			"alpha": map[string]any{
				"concurrency": 5,
				"mode":        "weighted",
				"weights":     map[string]any{"default": 5, "low": 1},
			},
			"beta": map[string]any{
				"concurrency": 3,
				"mode":        "weighted",
				"weights":     map[string]any{"default": 2, "critical": 4},
			},
		},
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	expectedAlphaWeights := map[string]int{"default": 5, "low": 1}
	if !reflect.DeepEqual(process.Capsules["alpha"].Weights, expectedAlphaWeights) {
		t.Fatalf("Capsules[alpha].Weights = %#v, want %#v", process.Capsules["alpha"].Weights, expectedAlphaWeights)
	}
	expectedBetaWeights := map[string]int{"default": 2, "critical": 4}
	if !reflect.DeepEqual(process.Capsules["beta"].Weights, expectedBetaWeights) {
		t.Fatalf("Capsules[beta].Weights = %#v, want %#v", process.Capsules["beta"].Weights, expectedBetaWeights)
	}
}

func TestParseOptionalBeatAndQuiet(t *testing.T) {
	t.Parallel()

	if beat, ok := parseOptionalFloat64("123.45"); !ok || beat != 123.45 {
		t.Fatalf("parseOptionalFloat64(\"123.45\") = %v, %v", beat, ok)
	}
	if quiet, ok := parseOptionalBool("true"); !ok || !quiet {
		t.Fatalf("parseOptionalBool(\"true\") = %v, %v", quiet, ok)
	}
	if quiet, ok := parseOptionalBool("false"); !ok || quiet {
		t.Fatalf("parseOptionalBool(\"false\") = %v, %v", quiet, ok)
	}
}

func TestParseProcessInfoMissingInfo(t *testing.T) {
	t.Parallel()

	process := Process{
		Hostname: "existing",
		PID:      42,
	}

	parseProcessInfo("{", &process)

	if process.Hostname != "existing" {
		t.Fatalf("Hostname = %q, want %q", process.Hostname, "existing")
	}
	if process.PID != 42 {
		t.Fatalf("PID = %d, want %d", process.PID, 42)
	}
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	return data
}
