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

func TestGetProcesses(t *testing.T) {
	mr, client := setupTestRedis(t)

	_, _ = mr.SetAdd("processes", "host1:100:abc", "host2:200:def", "host3:300:ghi")

	processes, err := client.GetProcesses(testContext(t))
	if err != nil {
		t.Fatalf("GetProcesses failed: %v", err)
	}

	if len(processes) != 3 {
		t.Fatalf("len(processes) = %d, want 3", len(processes))
	}

	if processes[0].Identity != "host1:100:abc" {
		t.Errorf("processes[0].Identity = %q, want host1:100:abc (sorted)", processes[0].Identity)
	}
	if processes[1].Identity != "host2:200:def" {
		t.Errorf("processes[1].Identity = %q, want host2:200:def", processes[1].Identity)
	}
	if processes[2].Identity != "host3:300:ghi" {
		t.Errorf("processes[2].Identity = %q, want host3:300:ghi", processes[2].Identity)
	}
}

func TestGetProcesses_Empty(t *testing.T) {
	_, client := setupTestRedis(t)

	processes, err := client.GetProcesses(testContext(t))
	if err != nil {
		t.Fatalf("GetProcesses failed: %v", err)
	}

	if len(processes) != 0 {
		t.Errorf("len(processes) = %d, want 0", len(processes))
	}
}

func TestGetBusyData(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	_, _ = mr.SetAdd("processes", "host1:100:abc", "host2:200:def")

	info1 := map[string]any{
		"hostname":    "host1",
		"pid":         100,
		"concurrency": 5,
		"queues":      []any{"default"},
	}
	info2 := map[string]any{
		"hostname":    "host2",
		"pid":         200,
		"concurrency": 10,
		"queues":      []any{"critical"},
	}

	mr.HSet("host1:100:abc", "info", string(mustMarshalJSON(t, info1)))
	mr.HSet("host1:100:abc", "busy", "2")
	mr.HSet("host1:100:abc", "beat", "1234567890.5")

	mr.HSet("host2:200:def", "info", string(mustMarshalJSON(t, info2)))
	mr.HSet("host2:200:def", "busy", "3")
	mr.HSet("host2:200:def", "quiet", "true")

	work1 := map[string]any{
		"queue":   "default",
		"payload": `{"jid":"job1","class":"MyJob","args":[]}`,
		"run_at":  1234567800.0,
	}
	work2 := map[string]any{
		"queue":   "critical",
		"payload": `{"jid":"job2","class":"UrgentJob","args":[]}`,
		"run_at":  1234567900.0,
	}

	mr.HSet("host1:100:abc:work", "tid1", string(mustMarshalJSON(t, work1)))
	mr.HSet("host2:200:def:work", "tid2", string(mustMarshalJSON(t, work2)))

	data, err := client.GetBusyData(ctx)
	if err != nil {
		t.Fatalf("GetBusyData failed: %v", err)
	}

	if len(data.Processes) != 2 {
		t.Fatalf("len(Processes) = %d, want 2", len(data.Processes))
	}

	p1 := data.Processes[0]
	if p1.Hostname != "host1" {
		t.Errorf("p1.Hostname = %q, want host1", p1.Hostname)
	}
	if p1.PID != 100 {
		t.Errorf("p1.PID = %d, want 100", p1.PID)
	}
	if p1.Busy != 2 {
		t.Errorf("p1.Busy = %d, want 2", p1.Busy)
	}
	if p1.Concurrency != 5 {
		t.Errorf("p1.Concurrency = %d, want 5", p1.Concurrency)
	}

	p2 := data.Processes[1]
	if p2.Hostname != "host2" {
		t.Errorf("p2.Hostname = %q, want host2", p2.Hostname)
	}
	if p2.Busy != 3 {
		t.Errorf("p2.Busy = %d, want 3", p2.Busy)
	}
	if !p2.Quiet {
		t.Errorf("p2.Quiet = false, want true")
	}

	if len(data.Jobs) != 2 {
		t.Fatalf("len(Jobs) = %d, want 2", len(data.Jobs))
	}

	j1 := data.Jobs[0]
	if j1.JID() != "job1" {
		t.Errorf("j1.JID() = %q, want job1", j1.JID())
	}
	if j1.ProcessIdentity != "host1:100:abc" {
		t.Errorf("j1.ProcessIdentity = %q, want host1:100:abc", j1.ProcessIdentity)
	}
	if j1.ThreadID != "tid1" {
		t.Errorf("j1.ThreadID = %q, want tid1", j1.ThreadID)
	}
}

func TestGetBusyData_Empty(t *testing.T) {
	_, client := setupTestRedis(t)

	data, err := client.GetBusyData(testContext(t))
	if err != nil {
		t.Fatalf("GetBusyData failed: %v", err)
	}

	if len(data.Processes) != 0 {
		t.Errorf("len(Processes) = %d, want 0", len(data.Processes))
	}
	if len(data.Jobs) != 0 {
		t.Errorf("len(Jobs) = %d, want 0", len(data.Jobs))
	}
}

func TestProcessRefresh(t *testing.T) {
	mr, client := setupTestRedis(t)

	info := map[string]any{
		"hostname":    "myhost",
		"pid":         999,
		"tag":         "production",
		"concurrency": 25,
		"started_at":  1700000000.0,
		"queues":      []any{"high", "default"},
	}

	mr.HSet("myhost:999:xyz", "info", string(mustMarshalJSON(t, info)))
	mr.HSet("myhost:999:xyz", "busy", "10")
	mr.HSet("myhost:999:xyz", "beat", "1234567890.5")
	mr.HSet("myhost:999:xyz", "quiet", "false")
	mr.HSet("myhost:999:xyz", "rss", "512")
	mr.HSet("myhost:999:xyz", "rtt_us", "1500")

	process := client.NewProcess("myhost:999:xyz")
	err := process.Refresh(testContext(t))
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if process.Hostname != "myhost" {
		t.Errorf("Hostname = %q, want myhost", process.Hostname)
	}
	if process.PID != 999 {
		t.Errorf("PID = %d, want 999", process.PID)
	}
	if process.Tag != "production" {
		t.Errorf("Tag = %q, want production", process.Tag)
	}
	if process.Concurrency != 25 {
		t.Errorf("Concurrency = %d, want 25", process.Concurrency)
	}
	if process.Busy != 10 {
		t.Errorf("Busy = %d, want 10", process.Busy)
	}
	if process.Quiet {
		t.Errorf("Quiet = true, want false")
	}
	if process.RSS != 512*1024 {
		t.Errorf("RSS = %d, want %d (KB to bytes)", process.RSS, 512*1024)
	}
	if process.RTTUS != 1500 {
		t.Errorf("RTTUS = %d, want 1500", process.RTTUS)
	}
}

func TestProcessRefresh_IdentityParsing(t *testing.T) {
	mr, client := setupTestRedis(t)

	mr.HSet("fallback:123:abc", "busy", "5")

	process := client.NewProcess("fallback:123:abc")
	err := process.Refresh(testContext(t))
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if process.Hostname != "fallback" {
		t.Errorf("Hostname = %q, want fallback (parsed from identity)", process.Hostname)
	}
	if process.PID != 123 {
		t.Errorf("PID = %d, want 123 (parsed from identity)", process.PID)
	}
	if process.Busy != 5 {
		t.Errorf("Busy = %d, want 5", process.Busy)
	}
}

func TestProcessGetJobs(t *testing.T) {
	mr, client := setupTestRedis(t)

	work1 := map[string]any{
		"queue":   "default",
		"payload": `{"jid":"job1","class":"MyJob","args":[1,2,3]}`,
		"run_at":  1234567890.5,
	}
	work2 := map[string]any{
		"queue":   "critical",
		"payload": `{"jid":"job2","class":"UrgentJob","args":[]}`,
		"run_at":  1234567900.0,
	}

	mr.HSet("test:100:xyz:work", "thread1", string(mustMarshalJSON(t, work1)))
	mr.HSet("test:100:xyz:work", "thread2", string(mustMarshalJSON(t, work2)))

	process := client.NewProcess("test:100:xyz")
	jobs, err := process.GetJobs(testContext(t), "")
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	if jobs[0].ProcessIdentity != "test:100:xyz" {
		t.Errorf("jobs[0].ProcessIdentity = %q, want test:100:xyz", jobs[0].ProcessIdentity)
	}
	if jobs[0].ThreadID == "" {
		t.Errorf("jobs[0].ThreadID is empty")
	}
	if jobs[0].JID() == "" {
		t.Errorf("jobs[0].JID() is empty")
	}
}

func TestProcessGetJobs_Filter(t *testing.T) {
	mr, client := setupTestRedis(t)

	work1 := map[string]any{
		"queue":   "default",
		"payload": `{"jid":"abc123","class":"MyJob","args":[]}`,
		"run_at":  1234567890.0,
	}
	work2 := map[string]any{
		"queue":   "default",
		"payload": `{"jid":"xyz456","class":"OtherJob","args":[]}`,
		"run_at":  1234567900.0,
	}

	mr.HSet("test:100:xyz:work", "t1", string(mustMarshalJSON(t, work1)))
	mr.HSet("test:100:xyz:work", "t2", string(mustMarshalJSON(t, work2)))

	process := client.NewProcess("test:100:xyz")
	jobs, err := process.GetJobs(testContext(t), "abc")
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (filtered)", len(jobs))
	}

	if jobs[0].JID() != "abc123" {
		t.Errorf("jobs[0].JID() = %q, want abc123", jobs[0].JID())
	}
}

func TestProcessGetJobs_Empty(t *testing.T) {
	_, client := setupTestRedis(t)

	process := client.NewProcess("test:100:xyz")
	jobs, err := process.GetJobs(testContext(t), "")
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0", len(jobs))
	}
}

func TestProcessGetJobs_InvalidJSON(t *testing.T) {
	mr, client := setupTestRedis(t)

	mr.HSet("test:100:xyz:work", "t1", "invalid json")
	mr.HSet("test:100:xyz:work", "t2", `{"queue":"default","payload":"{\"jid\":\"good\"}","run_at":1234567890.0}`)

	process := client.NewProcess("test:100:xyz")
	jobs, err := process.GetJobs(testContext(t), "")
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (invalid JSON skipped)", len(jobs))
	}

	if jobs[0].JID() != "good" {
		t.Errorf("jobs[0].JID() = %q, want good", jobs[0].JID())
	}
}

func TestParseProcessInfoEmptyCapsules(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"queues":   []any{"default"},
		"capsules": map[string]any{},
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	if len(process.Capsules) != 1 {
		t.Fatalf("Capsules len = %d, want 1 (default from queues)", len(process.Capsules))
	}
	if _, ok := process.Capsules[DefaultCapsuleName]; !ok {
		t.Errorf("Capsules[default] missing, should be created from queues")
	}
}

func TestParseProcessInfoAllWeightsOne(t *testing.T) {
	t.Parallel()

	info := map[string]any{
		"queues":      []any{"q1", "q2", "q3"},
		"concurrency": 10,
		"weights":     []any{map[string]any{"q1": 1, "q2": 1, "q3": 1}},
	}

	data := mustMarshalJSON(t, info)
	var process Process
	parseProcessInfo(string(data), &process)

	if process.Capsules[DefaultCapsuleName].Mode != "random" {
		t.Errorf("Capsules[default].Mode = %q, want random (all weights are 1)", process.Capsules[DefaultCapsuleName].Mode)
	}

	expectedWeights := map[string]int{"q1": 1, "q2": 1, "q3": 1}
	if !reflect.DeepEqual(process.Capsules[DefaultCapsuleName].Weights, expectedWeights) {
		t.Errorf("Capsules[default].Weights = %#v, want %#v", process.Capsules[DefaultCapsuleName].Weights, expectedWeights)
	}
}

func TestGetBusyData_SkipInvalidProcesses(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	_, _ = mr.SetAdd("processes", "good:100:abc", ":200:bad1", "bad2:0:def", "invalid")

	goodInfo := map[string]any{
		"hostname": "good",
		"pid":      100,
		"queues":   []any{"default"},
	}
	emptyHostnameInfo := map[string]any{
		"hostname": "",
		"pid":      200,
		"queues":   []any{"default"},
	}
	zeroPIDInfo := map[string]any{
		"hostname": "bad2",
		"pid":      0,
		"queues":   []any{"default"},
	}

	mr.HSet("good:100:abc", "info", string(mustMarshalJSON(t, goodInfo)))
	mr.HSet("good:100:abc", "busy", "1")

	mr.HSet(":200:bad1", "info", string(mustMarshalJSON(t, emptyHostnameInfo)))
	mr.HSet(":200:bad1", "busy", "2")

	mr.HSet("bad2:0:def", "info", string(mustMarshalJSON(t, zeroPIDInfo)))
	mr.HSet("bad2:0:def", "busy", "3")

	mr.HSet("invalid", "busy", "4")

	data, err := client.GetBusyData(ctx)
	if err != nil {
		t.Fatalf("GetBusyData failed: %v", err)
	}

	if len(data.Processes) != 1 {
		t.Fatalf("len(Processes) = %d, want 1 (invalid processes skipped: empty hostname, zero PID, invalid identity)", len(data.Processes))
	}

	if data.Processes[0].Hostname != "good" {
		t.Errorf("Processes[0].Hostname = %q, want good", data.Processes[0].Hostname)
	}
	if data.Processes[0].PID != 100 {
		t.Errorf("Processes[0].PID = %d, want 100", data.Processes[0].PID)
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
