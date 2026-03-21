package sidekiq

import (
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestGetErrorSummaryExactAcrossScanBatches(t *testing.T) {
	ctx := testContext(t)
	client, mr := newErrorsTestClient(t)

	for i := range 130 {
		addSortedSetJob(t, mr, deadSetKey, float64(i+1), errorPayload(
			fmt.Sprintf("dead-cleanup-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("dead cleanup %03d", i),
			"",
		))
	}
	for i := range 95 {
		addSortedSetJob(t, mr, retrySetKey, float64(i+1), errorPayload(
			fmt.Sprintf("retry-cleanup-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("retry cleanup %03d", i),
			"",
		))
	}
	for i := range 40 {
		addSortedSetJob(t, mr, deadSetKey, float64(1000+i), errorPayload(
			fmt.Sprintf("dead-mail-%03d", i),
			"MailJob",
			"mailers",
			"TimeoutError",
			fmt.Sprintf("mail timeout %03d", i),
			"",
		))
	}

	rows, meta, err := client.GetErrorSummary(ctx, "")
	if err != nil {
		t.Fatalf("GetErrorSummary failed: %v", err)
	}

	if meta.DeadCount != 170 {
		t.Fatalf("meta.DeadCount = %d, want 170", meta.DeadCount)
	}
	if meta.RetryCount != 95 {
		t.Fatalf("meta.RetryCount = %d, want 95", meta.RetryCount)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	byKey := map[ErrorGroupKey]ErrorSummaryRow{}
	for _, row := range rows {
		byKey[ErrorGroupKey{
			DisplayClass: row.DisplayClass,
			ErrorClass:   row.ErrorClass,
			Queue:        row.Queue,
		}] = row
	}

	cleanup := byKey[ErrorGroupKey{DisplayClass: "CleanupJob", ErrorClass: "ArgumentError", Queue: "default"}]
	if cleanup.Count != 225 {
		t.Fatalf("cleanup.Count = %d, want 225", cleanup.Count)
	}
	if cleanup.ErrorMessage != "dead cleanup 129" {
		t.Fatalf("cleanup.ErrorMessage = %q, want %q", cleanup.ErrorMessage, "dead cleanup 129")
	}

	mail := byKey[ErrorGroupKey{DisplayClass: "MailJob", ErrorClass: "TimeoutError", Queue: "mailers"}]
	if mail.Count != 40 {
		t.Fatalf("mail.Count = %d, want 40", mail.Count)
	}
}

func TestGetErrorSummaryFilteredAcrossScanBatches(t *testing.T) {
	ctx := testContext(t)
	client, mr := newErrorsTestClient(t)

	for i := range 120 {
		marker := ""
		if i < 70 {
			marker = "needle"
		}
		addSortedSetJob(t, mr, deadSetKey, float64(i+1), errorPayload(
			fmt.Sprintf("dead-match-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("dead cleanup %03d", i),
			marker,
		))
	}
	for i := range 110 {
		marker := ""
		if i < 45 {
			marker = "needle"
		}
		addSortedSetJob(t, mr, retrySetKey, float64(i+1), errorPayload(
			fmt.Sprintf("retry-match-%03d", i),
			"CleanupJob",
			"default",
			"ArgumentError",
			fmt.Sprintf("retry cleanup %03d", i),
			marker,
		))
	}
	for i := range 10 {
		addSortedSetJob(t, mr, retrySetKey, float64(1000+i), errorPayload(
			fmt.Sprintf("retry-other-%03d", i),
			"OtherJob",
			"critical",
			"RuntimeError",
			fmt.Sprintf("other failure %03d", i),
			"needle",
		))
	}

	rows, meta, err := client.GetErrorSummary(ctx, "needle")
	if err != nil {
		t.Fatalf("GetErrorSummary failed: %v", err)
	}

	if meta.DeadCount != 70 {
		t.Fatalf("meta.DeadCount = %d, want 70", meta.DeadCount)
	}
	if meta.RetryCount != 55 {
		t.Fatalf("meta.RetryCount = %d, want 55", meta.RetryCount)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	byKey := map[ErrorGroupKey]int64{}
	for _, row := range rows {
		byKey[ErrorGroupKey{
			DisplayClass: row.DisplayClass,
			ErrorClass:   row.ErrorClass,
			Queue:        row.Queue,
		}] = row.Count
	}

	if byKey[ErrorGroupKey{DisplayClass: "CleanupJob", ErrorClass: "ArgumentError", Queue: "default"}] != 115 {
		t.Fatalf("cleanup filtered count = %d, want 115", byKey[ErrorGroupKey{DisplayClass: "CleanupJob", ErrorClass: "ArgumentError", Queue: "default"}])
	}
	if byKey[ErrorGroupKey{DisplayClass: "OtherJob", ErrorClass: "RuntimeError", Queue: "critical"}] != 10 {
		t.Fatalf("other filtered count = %d, want 10", byKey[ErrorGroupKey{DisplayClass: "OtherJob", ErrorClass: "RuntimeError", Queue: "critical"}])
	}
}

func TestGetErrorGroupWindowPagedAcrossDeadAndRetry(t *testing.T) {
	ctx := testContext(t)
	client, mr := newErrorsTestClient(t)

	groupKey := ErrorGroupKey{
		DisplayClass: "CleanupJob",
		ErrorClass:   "ArgumentError",
		Queue:        "default",
	}

	addSortedSetJob(t, mr, deadSetKey, 1, errorPayload("dead1", "CleanupJob", "default", "ArgumentError", "dead one", ""))
	addSortedSetJob(t, mr, deadSetKey, 2, errorPayload("dead2", "CleanupJob", "default", "ArgumentError", "dead two", ""))
	addSortedSetJob(t, mr, deadSetKey, 3, errorPayload("dead3", "CleanupJob", "default", "ArgumentError", "dead three", ""))
	addSortedSetJob(t, mr, retrySetKey, 10, errorPayload("retry1", "CleanupJob", "default", "ArgumentError", "retry one", ""))
	addSortedSetJob(t, mr, retrySetKey, 20, errorPayload("retry2", "CleanupJob", "default", "ArgumentError", "retry two", ""))
	addSortedSetJob(t, mr, retrySetKey, 30, errorPayload("retry3", "CleanupJob", "default", "ArgumentError", "retry three", ""))
	addSortedSetJob(t, mr, retrySetKey, 40, errorPayload("other", "OtherJob", "critical", "RuntimeError", "ignore", ""))

	window, err := client.GetErrorGroupWindow(ctx, groupKey, "", 2, 3)
	if err != nil {
		t.Fatalf("GetErrorGroupWindow failed: %v", err)
	}

	if window.Total != 6 {
		t.Fatalf("window.Total = %d, want 6", window.Total)
	}
	if window.WindowStart != 2 {
		t.Fatalf("window.WindowStart = %d, want 2", window.WindowStart)
	}
	if len(window.Entries) != 3 {
		t.Fatalf("len(window.Entries) = %d, want 3", len(window.Entries))
	}

	got := []string{
		window.Entries[0].Source + ":" + window.Entries[0].Entry.JID(),
		window.Entries[1].Source + ":" + window.Entries[1].Entry.JID(),
		window.Entries[2].Source + ":" + window.Entries[2].Entry.JID(),
	}
	want := []string{"dead:dead1", "retry:retry1", "retry:retry2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("window entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func newErrorsTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client, err := NewClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	return client, mr
}

func addSortedSetJob(t *testing.T, mr *miniredis.Miniredis, key string, score float64, payload string) {
	t.Helper()
	if _, err := mr.ZAdd(key, score, payload); err != nil {
		t.Fatalf("ZAdd(%s) failed: %v", key, err)
	}
}

func errorPayload(jid, class, queue, errorClass, errorMessage, marker string) string {
	return fmt.Sprintf(
		`{"jid":"%s","class":"%s","queue":"%s","args":["%s"],"error_class":"%s","error_message":"%s"}`,
		jid,
		class,
		queue,
		marker,
		errorClass,
		errorMessage,
	)
}
