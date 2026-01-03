package sidekiq

import (
	"testing"
	"time"
)

func TestGetStatsHistory(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	twoDaysAgo := now.AddDate(0, 0, -2).Format("2006-01-02")

	_ = mr.Set("stat:processed:"+today, "1000")
	_ = mr.Set("stat:failed:"+today, "50")
	_ = mr.Set("stat:processed:"+yesterday, "800")
	_ = mr.Set("stat:failed:"+yesterday, "40")
	_ = mr.Set("stat:processed:"+twoDaysAgo, "600")
	_ = mr.Set("stat:failed:"+twoDaysAgo, "30")

	history, err := client.GetStatsHistory(ctx, 3)
	if err != nil {
		t.Fatalf("GetStatsHistory failed: %v", err)
	}

	if len(history.Dates) != 3 {
		t.Errorf("len(Dates) = %d, want 3", len(history.Dates))
	}
	if len(history.Processed) != 3 {
		t.Errorf("len(Processed) = %d, want 3", len(history.Processed))
	}
	if len(history.Failed) != 3 {
		t.Errorf("len(Failed) = %d, want 3", len(history.Failed))
	}

	if history.Processed[0] != 600 {
		t.Errorf("Processed[0] = %d, want 600 (two days ago)", history.Processed[0])
	}
	if history.Processed[1] != 800 {
		t.Errorf("Processed[1] = %d, want 800 (yesterday)", history.Processed[1])
	}
	if history.Processed[2] != 1000 {
		t.Errorf("Processed[2] = %d, want 1000 (today)", history.Processed[2])
	}

	if history.Failed[0] != 30 {
		t.Errorf("Failed[0] = %d, want 30 (two days ago)", history.Failed[0])
	}
	if history.Failed[1] != 40 {
		t.Errorf("Failed[1] = %d, want 40 (yesterday)", history.Failed[1])
	}
	if history.Failed[2] != 50 {
		t.Errorf("Failed[2] = %d, want 50 (today)", history.Failed[2])
	}
}

func TestGetStatsHistory_MissingDays(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := testContext(t)

	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	_ = mr.Set("stat:processed:"+today, "1000")
	_ = mr.Set("stat:failed:"+today, "50")

	history, err := client.GetStatsHistory(ctx, 3)
	if err != nil {
		t.Fatalf("GetStatsHistory failed: %v", err)
	}

	if history.Processed[0] != 0 {
		t.Errorf("Processed[0] = %d, want 0 (missing day)", history.Processed[0])
	}
	if history.Processed[1] != 0 {
		t.Errorf("Processed[1] = %d, want 0 (missing day)", history.Processed[1])
	}
	if history.Processed[2] != 1000 {
		t.Errorf("Processed[2] = %d, want 1000 (today)", history.Processed[2])
	}

	if history.Failed[2] != 50 {
		t.Errorf("Failed[2] = %d, want 50 (today)", history.Failed[2])
	}
}

func TestGetStatsHistory_InvalidDays(t *testing.T) {
	_, client := setupTestRedis(t)

	history, err := client.GetStatsHistory(testContext(t), 0)
	if err != nil {
		t.Fatalf("GetStatsHistory failed: %v", err)
	}

	if len(history.Dates) != 1 {
		t.Errorf("len(Dates) = %d, want 1 (minimum)", len(history.Dates))
	}

	history, err = client.GetStatsHistory(testContext(t), -5)
	if err != nil {
		t.Fatalf("GetStatsHistory failed: %v", err)
	}

	if len(history.Dates) != 1 {
		t.Errorf("len(Dates) = %d, want 1 (minimum)", len(history.Dates))
	}
}
