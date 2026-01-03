package sidekiq

import (
	"context"
	"testing"
)

func TestGetStats(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Set up test data
	_ = mr.Set("stat:processed", "1234")
	_ = mr.Set("stat:failed", "56")

	// Add jobs to sorted sets
	_, _ = mr.ZAdd("retry", 1.0, `{"jid":"retry1"}`)
	_, _ = mr.ZAdd("retry", 2.0, `{"jid":"retry2"}`)
	_, _ = mr.ZAdd("retry", 3.0, `{"jid":"retry3"}`)

	_, _ = mr.ZAdd("schedule", 1.0, `{"jid":"sched1"}`)
	_, _ = mr.ZAdd("schedule", 2.0, `{"jid":"sched2"}`)

	_, _ = mr.ZAdd("dead", 1.0, `{"jid":"dead1"}`)

	// Add queues
	_, _ = mr.SetAdd("queues", "default", "critical", "low")
	_, _ = mr.Push("queue:default", "job1")
	_, _ = mr.Push("queue:default", "job2")
	_, _ = mr.Push("queue:default", "job3")
	_, _ = mr.Push("queue:critical", "job4")
	_, _ = mr.Push("queue:critical", "job5")
	_, _ = mr.Push("queue:low", "job6")

	// Add processes with busy counts
	_, _ = mr.SetAdd("processes", "hostname1:pid1:workers", "hostname2:pid2:workers")
	mr.HSet("hostname1:pid1:workers", "busy", "4")
	mr.HSet("hostname2:pid2:workers", "busy", "7")

	// Get stats
	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	// Verify all stats
	tests := []struct {
		name     string
		got      int64
		expected int64
	}{
		{"Processed", stats.Processed, 1234},
		{"Failed", stats.Failed, 56},
		{"Retries", stats.Retries, 3},
		{"Scheduled", stats.Scheduled, 2},
		{"Dead", stats.Dead, 1},
		{"Busy", stats.Busy, 11},
		{"Enqueued", stats.Enqueued, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s: got %d, want %d", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestGetStats_Empty(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := context.Background()

	// Get stats from empty Redis
	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	// All stats should be zero
	if stats.Processed != 0 {
		t.Errorf("Processed: got %d, want 0", stats.Processed)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed: got %d, want 0", stats.Failed)
	}
	if stats.Retries != 0 {
		t.Errorf("Retries: got %d, want 0", stats.Retries)
	}
	if stats.Scheduled != 0 {
		t.Errorf("Scheduled: got %d, want 0", stats.Scheduled)
	}
	if stats.Dead != 0 {
		t.Errorf("Dead: got %d, want 0", stats.Dead)
	}
	if stats.Busy != 0 {
		t.Errorf("Busy: got %d, want 0", stats.Busy)
	}
	if stats.Enqueued != 0 {
		t.Errorf("Enqueued: got %d, want 0", stats.Enqueued)
	}
}

func TestGetStats_PartialData(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Set only some data
	_ = mr.Set("stat:processed", "999")
	_, _ = mr.ZAdd("dead", 1.0, `{"jid":"dead1"}`)
	_, _ = mr.ZAdd("dead", 2.0, `{"jid":"dead2"}`)

	// Get stats
	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	// Verify what we set
	if stats.Processed != 999 {
		t.Errorf("Processed: got %d, want 999", stats.Processed)
	}
	if stats.Dead != 2 {
		t.Errorf("Dead: got %d, want 2", stats.Dead)
	}

	// Others should be zero
	if stats.Failed != 0 {
		t.Errorf("Failed: got %d, want 0", stats.Failed)
	}
	if stats.Retries != 0 {
		t.Errorf("Retries: got %d, want 0", stats.Retries)
	}
}

func TestGetStats_MultipleQueues(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Add many queues with different lengths
	queues := map[string]int{
		"default":  10,
		"critical": 5,
		"low":      2,
		"mailers":  8,
		"reports":  3,
	}

	for queue, count := range queues {
		_, _ = mr.SetAdd("queues", queue)
		for range count {
			_, _ = mr.Push("queue:"+queue, "job")
		}
	}

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	expectedEnqueued := int64(10 + 5 + 2 + 8 + 3)
	if stats.Enqueued != expectedEnqueued {
		t.Errorf("Enqueued: got %d, want %d", stats.Enqueued, expectedEnqueued)
	}
}

func TestGetStats_MultipleProcesses(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Add multiple processes with different busy counts
	processes := map[string]string{
		"server1:1234:workers": "5",
		"server2:5678:workers": "3",
		"server3:9012:workers": "8",
		"server4:3456:workers": "0",
	}

	for proc, busy := range processes {
		_, _ = mr.SetAdd("processes", proc)
		mr.HSet(proc, "busy", busy)
	}

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	expectedBusy := int64(5 + 3 + 8 + 0)
	if stats.Busy != expectedBusy {
		t.Errorf("Busy: got %d, want %d", stats.Busy, expectedBusy)
	}
}

func TestGetStats_DeadProcess(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Add process to set but don't create the hash (simulates dead process)
	_, _ = mr.SetAdd("processes", "dead:process:workers")

	// Add a live process for comparison
	_, _ = mr.SetAdd("processes", "live:process:workers")
	mr.HSet("live:process:workers", "busy", "3")

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	// Should only count the live process
	if stats.Busy != 3 {
		t.Errorf("Busy: got %d, want 3 (dead process should be ignored)", stats.Busy)
	}
}

func TestGetStats_ProcessWithMissingBusy(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Add process with hash but no busy field
	_, _ = mr.SetAdd("processes", "incomplete:process:workers")
	mr.HSet("incomplete:process:workers", "info", `{"hostname":"test"}`)
	// No busy field set

	// Add normal process
	_, _ = mr.SetAdd("processes", "normal:process:workers")
	mr.HSet("normal:process:workers", "busy", "5")

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	// Should only count the normal process
	if stats.Busy != 5 {
		t.Errorf("Busy: got %d, want 5 (process without busy should be ignored)", stats.Busy)
	}
}

func TestGetStats_QueueInSetButNoList(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Add queue names but don't create all the lists
	_, _ = mr.SetAdd("queues", "existing", "ghost")
	_, _ = mr.Push("queue:existing", "job1")
	_, _ = mr.Push("queue:existing", "job2")
	// queue:ghost list intentionally not created

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	// Should only count jobs from existing queue
	if stats.Enqueued != 2 {
		t.Errorf("Enqueued: got %d, want 2 (ghost queue should count as 0)", stats.Enqueued)
	}
}

func TestGetStats_InvalidBusyValue(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Add process with invalid busy value
	_, _ = mr.SetAdd("processes", "broken:process:workers")
	mr.HSet("broken:process:workers", "busy", "not-a-number")

	// Add normal process
	_, _ = mr.SetAdd("processes", "normal:process:workers")
	mr.HSet("normal:process:workers", "busy", "7")

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	// Should only count the valid busy value
	if stats.Busy != 7 {
		t.Errorf("Busy: got %d, want 7 (invalid busy should be ignored)", stats.Busy)
	}
}

func TestGetStats_LargeNumbers(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()

	// Set large numbers
	_ = mr.Set("stat:processed", "999999999")
	_ = mr.Set("stat:failed", "123456789")

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.Processed != 999999999 {
		t.Errorf("Processed: got %d, want 999999999", stats.Processed)
	}
	if stats.Failed != 123456789 {
		t.Errorf("Failed: got %d, want 123456789", stats.Failed)
	}
}
