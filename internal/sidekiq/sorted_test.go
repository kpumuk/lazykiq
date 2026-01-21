package sidekiq

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

const (
	testScoreA    = 1767906476.252612
	testScoreB    = 1767910073.166937
	testScoreC    = 1767913660.050862
	testScoreBase = 1767900000.123456
)

func timeFromScore(score float64) time.Time {
	return time.Unix(0, int64(score*float64(time.Second)))
}

func TestGetDeadJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"dead1","class":"MyJob","args":[],"error_message":"boom"}`
	job2 := `{"jid":"dead2","class":"MyJob","args":[],"error_message":"crash"}`
	job3 := `{"jid":"dead3","class":"MyJob","args":[],"error_message":"fail"}`

	_, _ = mr.ZAdd("dead", testScoreA, job1)
	_, _ = mr.ZAdd("dead", testScoreB, job2)
	_, _ = mr.ZAdd("dead", testScoreC, job3)

	entries, size, err := client.GetDeadJobs(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetDeadJobs failed: %v", err)
	}

	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}

	if entries[0].JID() != "dead3" {
		t.Errorf("entries[0].JID() = %q, want %q (newest first)", entries[0].JID(), "dead3")
	}
	if entries[1].JID() != "dead2" {
		t.Errorf("entries[1].JID() = %q, want %q", entries[1].JID(), "dead2")
	}
	if entries[2].JID() != "dead1" {
		t.Errorf("entries[2].JID() = %q, want %q (oldest last)", entries[2].JID(), "dead1")
	}

	if entries[0].Score != testScoreC {
		t.Errorf("entries[0].Score = %f, want %f", entries[0].Score, testScoreC)
	}
	if got := entries[0].At(); !got.Equal(timeFromScore(testScoreC)) {
		t.Errorf("entries[0].At() = %v, want %v", got, timeFromScore(testScoreC))
	}
}

func TestGetDeadJobs_Empty(t *testing.T) {
	_, client := setupTestRedis(t)

	entries, size, err := client.GetDeadJobs(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("GetDeadJobs failed: %v", err)
	}

	if size != 0 {
		t.Errorf("size = %d, want 0", size)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0", len(entries))
	}
}

func TestGetDeadJobs_Pagination(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		job := fmt.Sprintf(`{"jid":"dead%d","class":"MyJob","args":[]}`, i)
		_, _ = mr.ZAdd("dead", testScoreBase+float64(i)*60, job)
	}

	entries, size, err := client.GetDeadJobs(ctx, 2, 3)
	if err != nil {
		t.Fatalf("GetDeadJobs failed: %v", err)
	}

	if size != 10 {
		t.Errorf("size = %d, want 10", size)
	}
	if len(entries) != 3 {
		t.Errorf("len(entries) = %d, want 3", len(entries))
	}

	if entries[0].JID() != "dead8" {
		t.Errorf("entries[0].JID() = %q, want dead8 (skip 2, newest first)", entries[0].JID())
	}
	if entries[1].JID() != "dead7" {
		t.Errorf("entries[1].JID() = %q, want dead7", entries[1].JID())
	}
	if entries[2].JID() != "dead6" {
		t.Errorf("entries[2].JID() = %q, want dead6", entries[2].JID())
	}
}

func TestGetDeadJobs_CountZero(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		job := fmt.Sprintf(`{"jid":"dead%d","class":"MyJob","args":[]}`, i)
		_, _ = mr.ZAdd("dead", testScoreBase+float64(i)*60, job)
	}

	entries, size, err := client.GetDeadJobs(ctx, 0, 0)
	if err != nil {
		t.Fatalf("GetDeadJobs failed: %v", err)
	}

	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
	if len(entries) != 5 {
		t.Errorf("len(entries) = %d, want 5 (count=0 means all)", len(entries))
	}
}

func TestGetRetryJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"retry1","class":"MyJob","args":[],"retry_count":1}`
	job2 := `{"jid":"retry2","class":"MyJob","args":[],"retry_count":2}`
	job3 := `{"jid":"retry3","class":"MyJob","args":[],"retry_count":3}`

	_, _ = mr.ZAdd("retry", testScoreA, job1)
	_, _ = mr.ZAdd("retry", testScoreB, job2)
	_, _ = mr.ZAdd("retry", testScoreC, job3)

	entries, size, err := client.GetRetryJobs(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetRetryJobs failed: %v", err)
	}

	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}

	if entries[0].JID() != "retry1" {
		t.Errorf("entries[0].JID() = %q, want %q (earliest first)", entries[0].JID(), "retry1")
	}
	if entries[1].JID() != "retry2" {
		t.Errorf("entries[1].JID() = %q, want %q", entries[1].JID(), "retry2")
	}
	if entries[2].JID() != "retry3" {
		t.Errorf("entries[2].JID() = %q, want %q (latest last)", entries[2].JID(), "retry3")
	}
}

func TestGetScheduledJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"sched1","class":"MyJob","args":[]}`
	job2 := `{"jid":"sched2","class":"MyJob","args":[]}`
	job3 := `{"jid":"sched3","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("schedule", testScoreA, job1)
	_, _ = mr.ZAdd("schedule", testScoreB, job2)
	_, _ = mr.ZAdd("schedule", testScoreC, job3)

	entries, size, err := client.GetScheduledJobs(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetScheduledJobs failed: %v", err)
	}

	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}

	if entries[0].JID() != "sched1" {
		t.Errorf("entries[0].JID() = %q, want %q (earliest first)", entries[0].JID(), "sched1")
	}
	if entries[1].JID() != "sched2" {
		t.Errorf("entries[1].JID() = %q, want %q", entries[1].JID(), "sched2")
	}
	if entries[2].JID() != "sched3" {
		t.Errorf("entries[2].JID() = %q, want %q", entries[2].JID(), "sched3")
	}
}

func TestGetDeadBounds(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"dead1","class":"MyJob","args":[]}`
	job2 := `{"jid":"dead2","class":"MyJob","args":[]}`
	job3 := `{"jid":"dead3","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("dead", testScoreA, job1)
	_, _ = mr.ZAdd("dead", testScoreC, job3)
	_, _ = mr.ZAdd("dead", testScoreB, job2)

	oldest, newest, err := client.GetDeadBounds(ctx)
	if err != nil {
		t.Fatalf("GetDeadBounds failed: %v", err)
	}
	if oldest == nil || newest == nil {
		t.Fatalf("GetDeadBounds returned nil entries")
	}
	if oldest.JID() != "dead1" {
		t.Errorf("oldest.JID() = %q, want dead1", oldest.JID())
	}
	if newest.JID() != "dead3" {
		t.Errorf("newest.JID() = %q, want dead3", newest.JID())
	}
}

func TestGetRetryBounds(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"retry1","class":"MyJob","args":[]}`
	job2 := `{"jid":"retry2","class":"MyJob","args":[]}`
	job3 := `{"jid":"retry3","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("retry", testScoreC, job3)
	_, _ = mr.ZAdd("retry", testScoreA, job1)
	_, _ = mr.ZAdd("retry", testScoreB, job2)

	first, last, err := client.GetRetryBounds(ctx)
	if err != nil {
		t.Fatalf("GetRetryBounds failed: %v", err)
	}
	if first == nil || last == nil {
		t.Fatalf("GetRetryBounds returned nil entries")
	}
	if first.JID() != "retry1" {
		t.Errorf("first.JID() = %q, want retry1", first.JID())
	}
	if last.JID() != "retry3" {
		t.Errorf("last.JID() = %q, want retry3", last.JID())
	}
}

func TestGetScheduledBounds(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"sched1","class":"MyJob","args":[]}`
	job2 := `{"jid":"sched2","class":"MyJob","args":[]}`
	job3 := `{"jid":"sched3","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("schedule", testScoreB, job2)
	_, _ = mr.ZAdd("schedule", testScoreC, job3)
	_, _ = mr.ZAdd("schedule", testScoreA, job1)

	first, last, err := client.GetScheduledBounds(ctx)
	if err != nil {
		t.Fatalf("GetScheduledBounds failed: %v", err)
	}
	if first == nil || last == nil {
		t.Fatalf("GetScheduledBounds returned nil entries")
	}
	if first.JID() != "sched1" {
		t.Errorf("first.JID() = %q, want sched1", first.JID())
	}
	if last.JID() != "sched3" {
		t.Errorf("last.JID() = %q, want sched3", last.JID())
	}
}

func TestGetBounds_Empty(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := context.Background()

	first, last, err := client.GetRetryBounds(ctx)
	if err != nil {
		t.Fatalf("GetRetryBounds failed: %v", err)
	}
	if first != nil || last != nil {
		t.Errorf("GetRetryBounds returned entries for empty set")
	}
}

func TestScanDeadJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"abc123","class":"MyJob","args":[]}`
	job2 := `{"jid":"xyz456","class":"OtherJob","args":[]}`
	job3 := `{"jid":"abc789","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("dead", testScoreA, job1)
	_, _ = mr.ZAdd("dead", testScoreB, job2)
	_, _ = mr.ZAdd("dead", testScoreC, job3)

	entries, err := client.ScanDeadJobs(ctx, "abc")
	if err != nil {
		t.Fatalf("ScanDeadJobs failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (matching 'abc')", len(entries))
	}

	if entries[0].JID() != "abc789" {
		t.Errorf("entries[0].JID() = %q, want abc789 (newest first)", entries[0].JID())
	}
	if entries[1].JID() != "abc123" {
		t.Errorf("entries[1].JID() = %q, want abc123", entries[1].JID())
	}
}

func TestScanDeadJobs_ExactMatch(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"test123","class":"MyJob","args":[]}`
	job2 := `{"jid":"test456","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("dead", testScoreA, job1)
	_, _ = mr.ZAdd("dead", testScoreB, job2)

	entries, err := client.ScanDeadJobs(ctx, "test123")
	if err != nil {
		t.Fatalf("ScanDeadJobs failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (exact match)", len(entries))
	}

	if entries[0].JID() != "test123" {
		t.Errorf("entries[0].JID() = %q, want test123", entries[0].JID())
	}
}

func TestScanDeadJobs_Wildcard(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"prefix_abc","class":"MyJob","args":[]}`
	job2 := `{"jid":"prefix_xyz","class":"MyJob","args":[]}`
	job3 := `{"jid":"other_job","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("dead", testScoreA, job1)
	_, _ = mr.ZAdd("dead", testScoreB, job2)
	_, _ = mr.ZAdd("dead", testScoreC, job3)

	entries, err := client.ScanDeadJobs(ctx, "*prefix*")
	if err != nil {
		t.Fatalf("ScanDeadJobs failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (wildcard pattern)", len(entries))
	}
}

func TestScanRetryJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"retry_early","class":"MyJob","args":[]}`
	job2 := `{"jid":"retry_late","class":"MyJob","args":[]}`
	job3 := `{"jid":"other_job","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("retry", testScoreC, job2)
	_, _ = mr.ZAdd("retry", testScoreA, job1)
	_, _ = mr.ZAdd("retry", testScoreB, job3)

	entries, err := client.ScanRetryJobs(ctx, "retry")
	if err != nil {
		t.Fatalf("ScanRetryJobs failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (matching 'retry')", len(entries))
	}

	if entries[0].JID() != "retry_early" {
		t.Errorf("entries[0].JID() = %q, want retry_early (earliest first)", entries[0].JID())
	}
	if entries[1].JID() != "retry_late" {
		t.Errorf("entries[1].JID() = %q, want retry_late", entries[1].JID())
	}
}

func TestScanScheduledJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"sched_123","class":"MyJob","args":[]}`
	job2 := `{"jid":"sched_456","class":"MyJob","args":[]}`
	job3 := `{"jid":"other_789","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("schedule", testScoreB, job2)
	_, _ = mr.ZAdd("schedule", testScoreC, job3)
	_, _ = mr.ZAdd("schedule", testScoreA, job1)

	entries, err := client.ScanScheduledJobs(ctx, "sched")
	if err != nil {
		t.Fatalf("ScanScheduledJobs failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (matching 'sched')", len(entries))
	}

	if entries[0].JID() != "sched_123" {
		t.Errorf("entries[0].JID() = %q, want sched_123 (earliest first)", entries[0].JID())
	}
	if entries[1].JID() != "sched_456" {
		t.Errorf("entries[1].JID() = %q, want sched_456", entries[1].JID())
	}
}

func TestScanJobs_Empty(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := context.Background()

	entries, err := client.ScanDeadJobs(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ScanDeadJobs failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0 (no matches)", len(entries))
	}
}

func TestScanJobs_NoPattern(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"job1","class":"MyJob","args":[]}`
	job2 := `{"jid":"job2","class":"MyJob","args":[]}`

	_, _ = mr.ZAdd("dead", testScoreA, job1)
	_, _ = mr.ZAdd("dead", testScoreB, job2)

	entries, err := client.ScanDeadJobs(ctx, "")
	if err != nil {
		t.Fatalf("ScanDeadJobs failed: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2 (empty pattern matches all)", len(entries))
	}
}

func TestNewSortedEntry(t *testing.T) {
	jobJSON := `{"jid":"test123","class":"MyJob","args":[1,2,3],"queue":"default"}`
	score := testScoreA

	entry := NewSortedEntry(jobJSON, score)

	if entry.JID() != "test123" {
		t.Errorf("JID() = %q, want test123", entry.JID())
	}
	if entry.Score != score {
		t.Errorf("Score = %f, want %f", entry.Score, score)
	}
	if got := entry.At(); !got.Equal(timeFromScore(score)) {
		t.Errorf("At() = %v, want %v", got, timeFromScore(score))
	}
	if entry.DisplayClass() != "MyJob" {
		t.Errorf("DisplayClass() = %q, want MyJob", entry.DisplayClass())
	}
}

func TestSortedEntry_ActiveJob(t *testing.T) {
	// ActiveJob wrapper
	jobJSON := `{"jid":"aj123","class":"ActiveJob::QueueAdapters::SidekiqAdapter::JobWrapper","wrapped":"MyActiveJob","args":[{"job_class":"MyActiveJob","arguments":[1,2,3]}]}`
	entry := NewSortedEntry(jobJSON, testScoreA)

	if entry.DisplayClass() != "MyActiveJob" {
		t.Errorf("DisplayClass() = %q, want MyActiveJob (unwrapped)", entry.DisplayClass())
	}
}

func TestDeleteRetryJob_RemovesOnly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	jobJSON := `{"jid":"retry_delete","class":"MyJob","queue":"default"}`
	_, _ = mr.ZAdd("retry", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.DeleteRetryJob(ctx, entry); err != nil {
		t.Fatalf("DeleteRetryJob failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "retry").Result(); size != 0 {
		t.Fatalf("retry size = %d, want 0", size)
	}
	if size, _ := client.redis.LLen(ctx, "queue:default").Result(); size != 0 {
		t.Fatalf("queue length = %d, want 0", size)
	}
}

func TestKillRetryJob_MovesToDead(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	jobJSON := `{"jid":"retry_kill","class":"MyJob","queue":"default"}`
	_, _ = mr.ZAdd("retry", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	start := time.Now()
	if err := client.KillRetryJob(ctx, entry); err != nil {
		t.Fatalf("KillRetryJob failed: %v", err)
	}
	end := time.Now()

	if size, _ := client.redis.ZCard(ctx, "retry").Result(); size != 0 {
		t.Fatalf("retry size = %d, want 0", size)
	}
	results, err := client.redis.ZRangeWithScores(ctx, "dead", 0, -1).Result()
	if err != nil {
		t.Fatalf("dead zrange failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("dead entries = %v, want 1 entry", results)
	}
	value, _ := results[0].Member.(string)
	if value != jobJSON {
		t.Fatalf("dead entries = %v, want %q", results, jobJSON)
	}
	deadAt := timeFromScore(results[0].Score)
	// Allow minor float64 precision loss when converting scores back to time.
	const scoreWindow = 2 * time.Millisecond
	windowStart := start.Add(-scoreWindow)
	windowEnd := end.Add(scoreWindow)
	if deadAt.Before(windowStart) || deadAt.After(windowEnd) {
		t.Fatalf("dead score time = %v, want between %v and %v", deadAt, windowStart, windowEnd)
	}
}

func TestRetryNowRetryJob_Sidekiq7(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	originalNow := nowFuncSidekiq
	nowFuncSidekiq = func() time.Time {
		return time.Unix(1700000000, 123456000)
	}
	t.Cleanup(func() { nowFuncSidekiq = originalNow })

	jobJSON := `{"jid":"retry_now","class":"MyJob","queue":"default","args":[1,2],"retry_count":2,"created_at":1700000000.5}`
	_, _ = mr.ZAdd("retry", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.RetryNowRetryJob(ctx, entry); err != nil {
		t.Fatalf("RetryNowRetryJob failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "retry").Result(); size != 0 {
		t.Fatalf("retry size = %d, want 0", size)
	}

	if ok, _ := client.redis.SIsMember(ctx, "queues", "default").Result(); !ok {
		t.Fatalf("queue set missing default queue")
	}

	values, err := client.redis.LRange(ctx, "queue:default", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue size = %d, want 1", len(values))
	}

	var payload map[string]any
	if err := safeParseJSON([]byte(values[0]), &payload); err != nil {
		t.Fatalf("safeParseJSON queued payload: %v", err)
	}

	retryCount, ok := payload["retry_count"].(json.Number)
	if !ok || retryCount.String() != "1" {
		t.Fatalf("retry_count = %v, want 1", payload["retry_count"])
	}

	enqueuedAt, ok := payload["enqueued_at"].(json.Number)
	if !ok {
		t.Fatalf("enqueued_at type = %T, want json.Number", payload["enqueued_at"])
	}
	if enqueuedAt.String() != "1700000000.123456" {
		t.Fatalf("enqueued_at = %q, want %q", enqueuedAt.String(), "1700000000.123456")
	}
}

func TestRetryNowRetryJob_Sidekiq8(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	originalNow := nowFuncSidekiq
	nowFuncSidekiq = func() time.Time {
		return time.Unix(1700000000, 123456000)
	}
	t.Cleanup(func() { nowFuncSidekiq = originalNow })

	jobJSON := `{"jid":"retry_now8","class":"MyJob","queue":"default","args":[1,2],"retry_count":1,"created_at":1700000000123}`
	_, _ = mr.ZAdd("retry", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.RetryNowRetryJob(ctx, entry); err != nil {
		t.Fatalf("RetryNowRetryJob failed: %v", err)
	}

	values, err := client.redis.LRange(ctx, "queue:default", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue size = %d, want 1", len(values))
	}

	var payload map[string]any
	if err := safeParseJSON([]byte(values[0]), &payload); err != nil {
		t.Fatalf("safeParseJSON queued payload: %v", err)
	}

	enqueuedAt, ok := payload["enqueued_at"].(json.Number)
	if !ok {
		t.Fatalf("enqueued_at type = %T, want json.Number", payload["enqueued_at"])
	}
	if enqueuedAt.String() != "1700000000123" {
		t.Fatalf("enqueued_at = %q, want %q", enqueuedAt.String(), "1700000000123")
	}
}

func TestAddScheduledJobToQueue_RemovesAt(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	originalNow := nowFuncSidekiq
	nowFuncSidekiq = func() time.Time {
		return time.Unix(1700000000, 0)
	}
	t.Cleanup(func() { nowFuncSidekiq = originalNow })

	jobJSON := `{"jid":"sched_now","class":"MyJob","queue":"default","args":[],"created_at":1700000000.0,"at":1700000100.0}`
	_, _ = mr.ZAdd("schedule", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.AddScheduledJobToQueue(ctx, entry); err != nil {
		t.Fatalf("AddScheduledJobToQueue failed: %v", err)
	}

	values, err := client.redis.LRange(ctx, "queue:default", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue size = %d, want 1", len(values))
	}

	var payload map[string]any
	if err := safeParseJSON([]byte(values[0]), &payload); err != nil {
		t.Fatalf("safeParseJSON queued payload: %v", err)
	}
	if _, ok := payload["at"]; ok {
		t.Fatalf("expected \"at\" to be removed")
	}
	if _, ok := payload["enqueued_at"]; !ok {
		t.Fatalf("expected enqueued_at to be set")
	}
}

func TestRetryNowDeadJob_MissingQueue(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	jobJSON := `{"jid":"dead_now","class":"MyJob","args":[],"created_at":1700000000.5}`
	_, _ = mr.ZAdd("dead", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.RetryNowDeadJob(ctx, entry); err == nil {
		t.Fatalf("RetryNowDeadJob should fail without queue")
	}

	if size, _ := client.redis.ZCard(ctx, "dead").Result(); size != 1 {
		t.Fatalf("dead size = %d, want 1", size)
	}
}

func TestRetryNowRetryJob_InvalidJSON(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	jobJSON := `{"jid":"bad-json"`
	_, _ = mr.ZAdd("retry", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.RetryNowRetryJob(ctx, entry); err == nil {
		t.Fatalf("RetryNowRetryJob should fail for invalid JSON")
	}

	if size, _ := client.redis.ZCard(ctx, "retry").Result(); size != 1 {
		t.Fatalf("retry size = %d, want 1", size)
	}
}

func TestDeleteScheduledJob_RemovesOnly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	jobJSON := `{"jid":"sched_delete","class":"MyJob","queue":"default"}`
	_, _ = mr.ZAdd("schedule", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.DeleteScheduledJob(ctx, entry); err != nil {
		t.Fatalf("DeleteScheduledJob failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "schedule").Result(); size != 0 {
		t.Fatalf("schedule size = %d, want 0", size)
	}
}

func TestDeleteDeadJob_RemovesOnly(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	jobJSON := `{"jid":"dead_delete","class":"MyJob","queue":"default"}`
	_, _ = mr.ZAdd("dead", testScoreA, jobJSON)

	entry := NewSortedEntry(jobJSON, testScoreA)
	if err := client.DeleteDeadJob(ctx, entry); err != nil {
		t.Fatalf("DeleteDeadJob failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "dead").Result(); size != 0 {
		t.Fatalf("dead size = %d, want 0", size)
	}
}

func TestDeleteAllRetryJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	_, _ = mr.ZAdd("retry", testScoreA, `{"jid":"retry_all_delete1","class":"MyJob","queue":"default"}`)
	_, _ = mr.ZAdd("retry", testScoreB, `{"jid":"retry_all_delete2","class":"MyJob","queue":"critical"}`)

	if err := client.DeleteAllRetryJobs(ctx); err != nil {
		t.Fatalf("DeleteAllRetryJobs failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "retry").Result(); size != 0 {
		t.Fatalf("retry size = %d, want 0", size)
	}
}

func TestRetryAllRetryJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	originalNow := nowFuncSidekiq
	nowFuncSidekiq = func() time.Time {
		return time.Unix(1700000000, 123456000)
	}
	t.Cleanup(func() { nowFuncSidekiq = originalNow })

	_, _ = mr.ZAdd("retry", testScoreA, `{"jid":"retry_all1","class":"MyJob","queue":"default","retry_count":2,"created_at":1700000000.5}`)
	_, _ = mr.ZAdd("retry", testScoreB, `{"jid":"retry_all2","class":"MyJob","queue":"critical","retry_count":1,"created_at":1700000000.5}`)

	if err := client.RetryAllRetryJobs(ctx); err != nil {
		t.Fatalf("RetryAllRetryJobs failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "retry").Result(); size != 0 {
		t.Fatalf("retry size = %d, want 0", size)
	}

	if ok, _ := client.redis.SIsMember(ctx, "queues", "default").Result(); !ok {
		t.Fatalf("queue set missing default queue")
	}
	if ok, _ := client.redis.SIsMember(ctx, "queues", "critical").Result(); !ok {
		t.Fatalf("queue set missing critical queue")
	}

	values, err := client.redis.LRange(ctx, "queue:default", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue default lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue default size = %d, want 1", len(values))
	}

	var payload map[string]any
	if err := safeParseJSON([]byte(values[0]), &payload); err != nil {
		t.Fatalf("safeParseJSON queued payload: %v", err)
	}
	retryCount, ok := payload["retry_count"].(json.Number)
	if !ok || retryCount.String() != "1" {
		t.Fatalf("retry_count = %v, want 1", payload["retry_count"])
	}
	enqueuedAt, ok := payload["enqueued_at"].(json.Number)
	if !ok || enqueuedAt.String() != "1700000000.123456" {
		t.Fatalf("enqueued_at = %v, want 1700000000.123456", payload["enqueued_at"])
	}

	values, err = client.redis.LRange(ctx, "queue:critical", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue critical lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue critical size = %d, want 1", len(values))
	}

	if err := safeParseJSON([]byte(values[0]), &payload); err != nil {
		t.Fatalf("safeParseJSON queued payload: %v", err)
	}
	retryCount, ok = payload["retry_count"].(json.Number)
	if !ok || retryCount.String() != "0" {
		t.Fatalf("retry_count = %v, want 0", payload["retry_count"])
	}
}

func TestKillAllRetryJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	job1 := `{"jid":"retry_kill1","class":"MyJob","queue":"default"}`
	job2 := `{"jid":"retry_kill2","class":"MyJob","queue":"critical"}`
	_, _ = mr.ZAdd("retry", testScoreA, job1)
	_, _ = mr.ZAdd("retry", testScoreB, job2)

	start := time.Now()
	if err := client.KillAllRetryJobs(ctx); err != nil {
		t.Fatalf("KillAllRetryJobs failed: %v", err)
	}
	end := time.Now()

	if size, _ := client.redis.ZCard(ctx, "retry").Result(); size != 0 {
		t.Fatalf("retry size = %d, want 0", size)
	}
	results, err := client.redis.ZRangeWithScores(ctx, "dead", 0, -1).Result()
	if err != nil {
		t.Fatalf("dead zrange failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("dead entries = %d, want 2", len(results))
	}

	windowStart := start.Add(-2 * time.Millisecond)
	windowEnd := end.Add(2 * time.Millisecond)
	for _, entry := range results {
		value, _ := entry.Member.(string)
		if value != job1 && value != job2 {
			t.Fatalf("unexpected dead payload: %v", value)
		}
		deadAt := timeFromScore(entry.Score)
		if deadAt.Before(windowStart) || deadAt.After(windowEnd) {
			t.Fatalf("dead score time = %v, want between %v and %v", deadAt, windowStart, windowEnd)
		}
	}
}

func TestDeleteAllScheduledJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	_, _ = mr.ZAdd("schedule", testScoreA, `{"jid":"sched_all_delete1","class":"MyJob","queue":"default"}`)
	_, _ = mr.ZAdd("schedule", testScoreB, `{"jid":"sched_all_delete2","class":"MyJob","queue":"critical"}`)

	if err := client.DeleteAllScheduledJobs(ctx); err != nil {
		t.Fatalf("DeleteAllScheduledJobs failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "schedule").Result(); size != 0 {
		t.Fatalf("schedule size = %d, want 0", size)
	}
}

func TestAddAllScheduledJobsToQueue(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	originalNow := nowFuncSidekiq
	nowFuncSidekiq = func() time.Time {
		return time.Unix(1700000000, 0)
	}
	t.Cleanup(func() { nowFuncSidekiq = originalNow })

	_, _ = mr.ZAdd("schedule", testScoreA, `{"jid":"sched_all1","class":"MyJob","queue":"default","created_at":1700000000.0,"at":1700000100.0}`)
	_, _ = mr.ZAdd("schedule", testScoreB, `{"jid":"sched_all2","class":"MyJob","queue":"critical","created_at":1700000000.0,"at":1700000200.0}`)

	if err := client.AddAllScheduledJobsToQueue(ctx); err != nil {
		t.Fatalf("AddAllScheduledJobsToQueue failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "schedule").Result(); size != 0 {
		t.Fatalf("schedule size = %d, want 0", size)
	}

	values, err := client.redis.LRange(ctx, "queue:default", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue default lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue default size = %d, want 1", len(values))
	}

	var payload map[string]any
	if err := safeParseJSON([]byte(values[0]), &payload); err != nil {
		t.Fatalf("safeParseJSON queued payload: %v", err)
	}
	if _, ok := payload["at"]; ok {
		t.Fatalf("expected \"at\" to be removed")
	}
	if _, ok := payload["enqueued_at"]; !ok {
		t.Fatalf("expected enqueued_at to be set")
	}

	values, err = client.redis.LRange(ctx, "queue:critical", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue critical lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue critical size = %d, want 1", len(values))
	}
}

func TestDeleteAllDeadJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	_, _ = mr.ZAdd("dead", testScoreA, `{"jid":"dead_all_delete1","class":"MyJob","queue":"default"}`)
	_, _ = mr.ZAdd("dead", testScoreB, `{"jid":"dead_all_delete2","class":"MyJob","queue":"critical"}`)

	if err := client.DeleteAllDeadJobs(ctx); err != nil {
		t.Fatalf("DeleteAllDeadJobs failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "dead").Result(); size != 0 {
		t.Fatalf("dead size = %d, want 0", size)
	}
}

func TestRetryAllDeadJobs(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	originalNow := nowFuncSidekiq
	nowFuncSidekiq = func() time.Time {
		return time.Unix(1700000000, 123456000)
	}
	t.Cleanup(func() { nowFuncSidekiq = originalNow })

	_, _ = mr.ZAdd("dead", testScoreA, `{"jid":"dead_retry1","class":"MyJob","queue":"default","retry_count":1,"created_at":1700000000.5}`)
	_, _ = mr.ZAdd("dead", testScoreB, `{"jid":"dead_retry2","class":"MyJob","queue":"critical","retry_count":2,"created_at":1700000000.5}`)

	if err := client.RetryAllDeadJobs(ctx); err != nil {
		t.Fatalf("RetryAllDeadJobs failed: %v", err)
	}

	if size, _ := client.redis.ZCard(ctx, "dead").Result(); size != 0 {
		t.Fatalf("dead size = %d, want 0", size)
	}

	values, err := client.redis.LRange(ctx, "queue:default", 0, -1).Result()
	if err != nil {
		t.Fatalf("queue default lrange failed: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("queue default size = %d, want 1", len(values))
	}

	var payload map[string]any
	if err := safeParseJSON([]byte(values[0]), &payload); err != nil {
		t.Fatalf("safeParseJSON queued payload: %v", err)
	}
	retryCount, ok := payload["retry_count"].(json.Number)
	if !ok || retryCount.String() != "0" {
		t.Fatalf("retry_count = %v, want 0", payload["retry_count"])
	}
	enqueuedAt, ok := payload["enqueued_at"].(json.Number)
	if !ok || enqueuedAt.String() != "1700000000.123456" {
		t.Fatalf("enqueued_at = %v, want 1700000000.123456", payload["enqueued_at"])
	}
}
