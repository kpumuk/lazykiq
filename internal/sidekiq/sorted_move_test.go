package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestEnqueueAllSortedEntriesInvalidBatchPreservesSource(t *testing.T) {
	mr, client := setupTestRedis(t)
	good := `{"jid":"good","class":"Good","queue":"default"}`
	bad := `{"jid":"bad","class":"Bad"}`
	_, _ = mr.ZAdd("retry", 1, good)
	_, _ = mr.ZAdd("retry", 2, bad)
	if err := client.EnqueueAllSortedEntries(context.Background(), SortedSetRetry); err == nil {
		t.Fatal("expected missing queue error")
	}
	if size := client.redis.ZCard(context.Background(), "retry").Val(); size != 2 {
		t.Fatalf("source size = %d, want 2", size)
	}
	if size := client.redis.LLen(context.Background(), "queue:default").Val(); size != 0 {
		t.Fatalf("destination size = %d, want 0", size)
	}
}

func TestEnqueueSortedEntryWrongTypePreservesSource(t *testing.T) {
	mr, client := setupTestRedis(t)
	job := `{"jid":"good","class":"Good","queue":"default"}`
	_, _ = mr.ZAdd("retry", 1, job)
	_ = mr.Set("queue:default", "wrong type")
	if err := client.EnqueueSortedEntry(context.Background(), SortedSetRetry, NewSortedEntry(job, 1)); err == nil {
		t.Fatal("expected destination type error")
	}
	if size := client.redis.ZCard(context.Background(), "retry").Val(); size != 1 {
		t.Fatalf("source size = %d, want 1", size)
	}
}

func TestSortedMovesDestinationPreflight(t *testing.T) {
	for name, test := range map[string]struct {
		bulk bool
		dead bool
		key  string
	}{
		"single registry": {key: "queues"},
		"bulk registry":   {bulk: true, key: "queues"},
		"bulk queue":      {bulk: true, key: "queue:second"},
		"bulk dead":       {bulk: true, dead: true, key: "dead"},
	} {
		t.Run(name, func(t *testing.T) {
			mr, client := setupTestRedis(t)
			ctx := context.Background()
			jobs := []string{
				`{"jid":"first","queue":"default"}`,
				`{"jid":"second","queue":"second"}`,
			}
			for i, job := range jobs {
				_, _ = mr.ZAdd("retry", float64(i+1), job)
			}
			_ = mr.Set(test.key, "wrong type")
			var err error
			switch {
			case test.dead:
				err = client.MoveAllSortedEntriesToDead(ctx, SortedSetRetry)
			case test.bulk:
				err = client.EnqueueAllSortedEntries(ctx, SortedSetRetry)
			default:
				err = client.EnqueueSortedEntry(ctx, SortedSetRetry, NewSortedEntry(jobs[0], 1))
			}
			if err == nil {
				t.Fatal("expected type error")
			}
			for i, job := range jobs {
				if score := client.redis.ZScore(ctx, "retry", job).Val(); score != float64(i+1) {
					t.Fatalf("source job %d score = %v", i, score)
				}
			}
			if size := client.redis.LLen(ctx, "queue:default").Val(); size != 0 {
				t.Fatalf("destination size = %d, want 0", size)
			}
		})
	}
}

func TestEnqueueAllSortedEntriesPartialSuccess(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()
	for i := range int(sortedSetMoveBatch) {
		job := fmt.Sprintf(`{"jid":"job%d","queue":"default","retry_count":2}`, i)
		_, _ = mr.ZAdd("retry", float64(i), job)
	}
	bad := `{"jid":"bad"}`
	good := `{"jid":"good","queue":"default"}`
	_, _ = mr.ZAdd("retry", float64(sortedSetMoveBatch), bad)
	_, _ = mr.ZAdd("retry", float64(sortedSetMoveBatch+1), good)
	if err := client.EnqueueAllSortedEntries(ctx, SortedSetRetry); err == nil {
		t.Fatal("expected invalid second batch error")
	}
	if got := client.redis.ZRange(ctx, "retry", 0, -1).Val(); !slices.Equal(got, []string{bad, good}) {
		t.Fatalf("remaining source = %v", got)
	}
	queued := client.redis.LRange(ctx, "queue:default", 0, -1).Val()
	if len(queued) != int(sortedSetMoveBatch) {
		t.Fatalf("queued = %d, want %d", len(queued), sortedSetMoveBatch)
	}
	for i, raw := range queued {
		var payload struct {
			JID        string `json:"jid"`
			RetryCount int    `json:"retry_count"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.JID != fmt.Sprintf("job%d", len(queued)-i-1) || payload.RetryCount != 1 {
			t.Fatalf("queue order or retry count changed: %s", raw)
		}
	}
}

func TestMoveAllSortedEntriesToDeadEmptyPayload(t *testing.T) {
	mr, client := setupTestRedis(t)
	_, _ = mr.ZAdd("retry", 1, `{"jid":"good"}`)
	_, _ = mr.ZAdd("retry", 2, "")
	ctx := context.Background()
	if err := client.MoveAllSortedEntriesToDead(ctx, SortedSetRetry); err == nil {
		t.Fatal("expected empty payload error")
	}
	if size := client.redis.ZCard(ctx, "retry").Val(); size != 2 {
		t.Fatalf("source size = %d, want 2", size)
	}
	if size := client.redis.ZCard(ctx, "dead").Val(); size != 0 {
		t.Fatalf("dead size = %d, want 0", size)
	}
}

// Interrupt at the script boundary, including an executed script whose reply was
// lost. A retry must not enqueue an already moved member again.
type sortedMoveFailureHook struct {
	after bool
	err   error
	fired bool
}

func (h *sortedMoveFailureHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *sortedMoveFailureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h *sortedMoveFailureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.fired || (cmd.Name() != "eval" && cmd.Name() != "evalsha") {
			return next(ctx, cmd)
		}
		if h.after {
			if err := next(ctx, cmd); err != nil {
				return err
			}
		}
		h.fired = true
		return h.err
	}
}

func TestSortedMovesInterrupted(t *testing.T) {
	for name, move := range map[string]func(*Client, context.Context, string) error{
		"single": func(c *Client, ctx context.Context, job string) error {
			return c.EnqueueSortedEntry(ctx, SortedSetRetry, NewSortedEntry(job, 1))
		},
		"bulk queue": func(c *Client, ctx context.Context, _ string) error {
			return c.EnqueueAllSortedEntries(ctx, SortedSetRetry)
		},
		"bulk dead": func(c *Client, ctx context.Context, _ string) error {
			return c.MoveAllSortedEntriesToDead(ctx, SortedSetRetry)
		},
	} {
		for _, after := range []bool{false, true} {
			for _, failure := range []error{context.Canceled, io.ErrUnexpectedEOF} {
				t.Run(fmt.Sprintf("%s/after=%t/%v", name, after, failure), func(t *testing.T) {
					mr, client := setupTestRedis(t)
					ctx := context.Background()
					job := `{"jid":"good","queue":"default"}`
					_, _ = mr.ZAdd("retry", 1, job)
					hook := &sortedMoveFailureHook{after: after, err: failure}
					client.redis.AddHook(hook)
					if err := move(client, ctx, job); !errors.Is(err, failure) {
						t.Fatalf("error = %v, want %v", err, failure)
					}
					wantSource := int64(1)
					if after {
						wantSource = 0
					}
					if size := client.redis.ZCard(ctx, "retry").Val(); size != wantSource {
						t.Fatalf("source size = %d, want %d", size, wantSource)
					}
					// Bulk retries complete; a stale single move returns job not found.
					err := move(client, ctx, job)
					if err != nil && (!after || name != "single") {
						t.Fatal(err)
					}
					if size := client.redis.ZCard(ctx, "retry").Val(); size != 0 {
						t.Fatalf("source size = %d", size)
					}
					destination := client.redis.LLen(ctx, "queue:default").Val() + client.redis.ZCard(ctx, "dead").Val()
					if destination != 1 {
						t.Fatalf("destination count = %d, want 1", destination)
					}
				})
			}
		}
	}
}

func TestSortedMovesConcurrent(t *testing.T) {
	for _, dead := range []bool{false, true} {
		t.Run(fmt.Sprintf("dead=%t", dead), func(t *testing.T) {
			mr, client := setupTestRedis(t)
			ctx := context.Background()
			const count = 225
			for i := range count {
				_, _ = mr.ZAdd("retry", float64(i), fmt.Sprintf(`{"jid":"job%d","queue":"default"}`, i))
			}
			var group sync.WaitGroup
			for range 4 {
				group.Go(func() {
					var err error
					if dead {
						err = client.MoveAllSortedEntriesToDead(ctx, SortedSetRetry)
					} else {
						err = client.EnqueueAllSortedEntries(ctx, SortedSetRetry)
					}
					if err != nil {
						t.Errorf("move: %v", err)
					}
				})
			}
			group.Wait()
			if size := client.redis.ZCard(ctx, "retry").Val(); size != 0 {
				t.Fatalf("source size = %d", size)
			}
			if dead {
				if size := client.redis.ZCard(ctx, "dead").Val(); size != count {
					t.Fatalf("dead size = %d", size)
				}
				return
			}
			jobs := client.redis.LRange(ctx, "queue:default", 0, -1).Val()
			seen := make(map[string]bool)
			for _, raw := range jobs {
				var payload struct {
					JID string `json:"jid"`
				}
				if err := json.Unmarshal([]byte(raw), &payload); err != nil {
					t.Fatal(err)
				}
				if seen[payload.JID] {
					t.Fatalf("duplicate job %s", payload.JID)
				}
				seen[payload.JID] = true
			}
			if len(seen) != count {
				t.Fatalf("queued = %d, want %d", len(seen), count)
			}
		})
	}
}
