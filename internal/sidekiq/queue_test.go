package sidekiq

import (
	"strconv"
	"testing"
	"time"
)

func TestGetQueues(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := testContext(t)

	queues, err := client.GetQueues(ctx)
	if err != nil {
		t.Fatalf("GetQueues failed: %v", err)
	}

	if len(queues) != 0 {
		t.Fatalf("len(queues) = %d, want 0", len(queues))
	}
}

func TestGetQueues_SingleQueue(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	_, _ = mr.SetAdd("queues", "default")

	queues, err := client.GetQueues(ctx)
	if err != nil {
		t.Fatalf("GetQueues failed: %v", err)
	}

	if len(queues) != 1 {
		t.Fatalf("len(queues) = %d, want 1", len(queues))
	}

	if queues[0].Name() != "default" {
		t.Errorf("queues[0].Name() = %q, want default", queues[0].Name())
	}
}

func TestGetQueues_MultipleSorted(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	_, _ = mr.SetAdd("queues", "critical", "default", "low")

	queues, err := client.GetQueues(ctx)
	if err != nil {
		t.Fatalf("GetQueues failed: %v", err)
	}

	if len(queues) != 3 {
		t.Fatalf("len(queues) = %d, want 3", len(queues))
	}

	// Should be sorted alphabetically
	expected := []string{"critical", "default", "low"}
	for i, name := range expected {
		if queues[i].Name() != name {
			t.Errorf("queues[%d].Name() = %q, want %q", i, queues[i].Name(), name)
		}
	}
}

func TestNewQueue(t *testing.T) {
	client := &Client{}

	q := client.NewQueue("test")

	if q.Name() != "test" {
		t.Errorf("q.Name() = %q, want test", q.Name())
	}

	if q.client == nil {
		t.Error("q.client is nil")
	}
}

func TestQueueSize(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	size, err := q.Size(ctx)
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}

	if size != 0 {
		t.Errorf("Size() = %d, want 0", size)
	}
}

func TestQueueSize_WithJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	job1 := map[string]any{"jid": "job1", "class": "TestJob"}
	job2 := map[string]any{"jid": "job2", "class": "TestJob"}

	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job1)))
	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job2)))

	size, err := q.Size(ctx)
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}

	if size != 2 {
		t.Errorf("Size() = %d, want 2", size)
	}
}

func TestQueueLatency_Empty(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	latency, err := q.Latency(ctx)
	if err != nil {
		t.Fatalf("Latency failed: %v", err)
	}

	if latency != 0.0 {
		t.Errorf("Latency() = %f, want 0.0", latency)
	}
}

func TestQueueLatency_OldFormat(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// Old format: float seconds (e.g., 1703000000.123)
	enqueuedAt := float64(time.Now().Unix() - 10) // 10 seconds ago

	job := map[string]any{
		"jid":         "job1",
		"class":       "TestJob",
		"enqueued_at": enqueuedAt,
	}

	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job)))

	latency, err := q.Latency(ctx)
	if err != nil {
		t.Fatalf("Latency failed: %v", err)
	}

	// Should be around 10 seconds (allow some margin for test execution time)
	if latency < 9.0 || latency > 11.0 {
		t.Errorf("Latency() = %f, want ~10.0", latency)
	}
}

func TestQueueLatency_NewFormat(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// New format: integer milliseconds (e.g., 1703000000123)
	enqueuedAt := float64(time.Now().UnixMilli() - 10000) // 10 seconds ago in millis

	job := map[string]any{
		"jid":         "job1",
		"class":       "TestJob",
		"enqueued_at": enqueuedAt,
	}

	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job)))

	latency, err := q.Latency(ctx)
	if err != nil {
		t.Fatalf("Latency failed: %v", err)
	}

	// Should be around 10 seconds (allow some margin for test execution time)
	if latency < 9.0 || latency > 11.0 {
		t.Errorf("Latency() = %f, want ~10.0", latency)
	}
}

func TestQueueLatency_NegativeClampedToZero(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// Future timestamp (negative latency)
	enqueuedAt := float64(time.Now().Unix() + 100)

	job := map[string]any{
		"jid":         "job1",
		"class":       "TestJob",
		"enqueued_at": enqueuedAt,
	}

	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job)))

	latency, err := q.Latency(ctx)
	if err != nil {
		t.Fatalf("Latency failed: %v", err)
	}

	if latency != 0.0 {
		t.Errorf("Latency() = %f, want 0.0 (negative clamped)", latency)
	}
}

func TestQueueLatency_InvalidJSON(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	_, _ = mr.Lpush("queue:default", "invalid json{")

	_, err := q.Latency(ctx)
	if err == nil {
		t.Fatal("Latency should fail with invalid JSON")
	}
}

func TestQueueLatency_MissingEnqueuedAt(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	job := map[string]any{
		"jid":   "job1",
		"class": "TestJob",
		// no enqueued_at
	}

	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job)))

	latency, err := q.Latency(ctx)
	if err != nil {
		t.Fatalf("Latency failed: %v", err)
	}

	if latency != 0.0 {
		t.Errorf("Latency() = %f, want 0.0 (missing enqueued_at)", latency)
	}
}

func TestQueueLatency_InvalidEnqueuedAtType(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// Manually construct JSON with string enqueued_at
	jobJSON := `{"jid":"job1","class":"TestJob","enqueued_at":"not a number"}`
	_, _ = mr.Lpush("queue:default", jobJSON)

	latency, err := q.Latency(ctx)
	if err != nil {
		t.Fatalf("Latency failed: %v", err)
	}

	if latency != 0.0 {
		t.Errorf("Latency() = %f, want 0.0 (invalid type)", latency)
	}
}

func TestQueueGetJobs_Empty(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	jobs, size, err := q.GetJobs(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if size != 0 {
		t.Errorf("size = %d, want 0", size)
	}

	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0", len(jobs))
	}
}

func TestQueueGetJobs_Basic(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// Add jobs (newest first in Redis list)
	job1 := map[string]any{"jid": "job1", "class": "TestJob"}
	job2 := map[string]any{"jid": "job2", "class": "TestJob"}
	job3 := map[string]any{"jid": "job3", "class": "TestJob"}

	// LPUSH adds to the front, so job3 is newest
	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job1)))
	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job2)))
	_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job3)))

	jobs, size, err := q.GetJobs(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}

	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}

	// Verify position calculation (descending: 3, 2, 1)
	if jobs[0].Position != 3 {
		t.Errorf("jobs[0].Position = %d, want 3", jobs[0].Position)
	}
	if jobs[1].Position != 2 {
		t.Errorf("jobs[1].Position = %d, want 2", jobs[1].Position)
	}
	if jobs[2].Position != 1 {
		t.Errorf("jobs[2].Position = %d, want 1", jobs[2].Position)
	}

	// Verify JIDs (newest first: job3, job2, job1)
	if jobs[0].JID() != "job3" {
		t.Errorf("jobs[0].JID() = %q, want job3", jobs[0].JID())
	}
	if jobs[1].JID() != "job2" {
		t.Errorf("jobs[1].JID() = %q, want job2", jobs[1].JID())
	}
	if jobs[2].JID() != "job1" {
		t.Errorf("jobs[2].JID() = %q, want job1", jobs[2].JID())
	}
}

func TestQueueGetJobs_Pagination(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// Add 10 jobs
	for i := range 10 {
		job := map[string]any{
			"jid":   "job" + strconv.Itoa(i),
			"class": "TestJob",
		}
		_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job)))
	}

	// Get first page (5 jobs)
	jobs, size, err := q.GetJobs(ctx, 0, 5)
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if size != 10 {
		t.Errorf("size = %d, want 10", size)
	}

	if len(jobs) != 5 {
		t.Fatalf("len(jobs) = %d, want 5", len(jobs))
	}

	// Positions should be 10, 9, 8, 7, 6
	expectedPositions := []int{10, 9, 8, 7, 6}
	for i, expectedPos := range expectedPositions {
		if jobs[i].Position != expectedPos {
			t.Errorf("jobs[%d].Position = %d, want %d", i, jobs[i].Position, expectedPos)
		}
	}

	// Get second page (next 5 jobs)
	jobs, _, err = q.GetJobs(ctx, 5, 5)
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if len(jobs) != 5 {
		t.Fatalf("len(jobs) = %d, want 5", len(jobs))
	}

	// Positions should be 5, 4, 3, 2, 1
	expectedPositions = []int{5, 4, 3, 2, 1}
	for i, expectedPos := range expectedPositions {
		if jobs[i].Position != expectedPos {
			t.Errorf("jobs[%d].Position = %d, want %d", i, jobs[i].Position, expectedPos)
		}
	}
}

func TestQueueGetJobs_PartialPage(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// Add 3 jobs
	for i := range 3 {
		job := map[string]any{
			"jid":   "job" + strconv.Itoa(i),
			"class": "TestJob",
		}
		_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job)))
	}

	// Request 10 jobs, but only 3 exist
	jobs, size, err := q.GetJobs(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}

	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
}

func TestQueueGetJobs_BeyondEnd(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	// Add 3 jobs
	for i := range 3 {
		job := map[string]any{
			"jid":   "job" + strconv.Itoa(i),
			"class": "TestJob",
		}
		_, _ = mr.Lpush("queue:default", string(mustMarshalJSON(t, job)))
	}

	// Request beyond end
	jobs, size, err := q.GetJobs(ctx, 10, 5)
	if err != nil {
		t.Fatalf("GetJobs failed: %v", err)
	}

	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}

	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0", len(jobs))
	}
}

func TestQueueClear_RemovesJobsAndQueueSet(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	q := client.NewQueue("default")

	_, _ = mr.SetAdd("queues", "default")
	_, _ = mr.Lpush("queue:default", "job1")
	_, _ = mr.Lpush("queue:default", "job2")

	if err := q.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	exists, err := client.redis.Exists(ctx, "queue:default").Result()
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists != 0 {
		t.Errorf("queue key exists = %d, want 0", exists)
	}

	queues, err := client.redis.SMembers(ctx, "queues").Result()
	if err != nil {
		t.Fatalf("SMembers failed: %v", err)
	}
	for _, name := range queues {
		if name == "default" {
			t.Errorf("queues set still contains %q", name)
		}
	}
}

func TestQueueClear_NilClient(t *testing.T) {
	q := &Queue{name: "default"}

	err := q.Clear(testContext(t))
	if err == nil {
		t.Fatal("Clear should fail with nil client")
	}
	if err.Error() != "queue client is nil" {
		t.Fatalf("Clear error = %q, want %q", err.Error(), "queue client is nil")
	}
}
