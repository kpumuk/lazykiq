package sidekiq

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

type scanCancelHook struct {
	cancel context.CancelFunc
}

func (h scanCancelHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h scanCancelHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		switch cmd := cmd.(type) {
		case *redis.IntCmd:
			cmd.SetVal(1000)
		case *redis.ScanCmd:
			cmd.SetVal([]string{`{"jid":"match"}`, "1"}, 0)
			h.cancel()
		case *redis.StringSliceCmd:
			cmd.SetVal([]string{`{"jid":"match"}`})
			h.cancel()
		}
		return nil
	}
}

func (h scanCancelHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestScansHonorCancellationAfterRedisReply(t *testing.T) {
	for name, scan := range map[string]func(context.Context, *Client) error{
		"queue": func(ctx context.Context, c *Client) error {
			_, err := c.NewQueue("default").ScanJobsWindow(ctx, "match", 0, 10)
			return err
		},
		"sorted": func(ctx context.Context, c *Client) error {
			_, err := c.ScanSortedEntriesWindow(ctx, SortedSetRetry, "match", 0, 10)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rdb := redis.NewClient(&redis.Options{})
			defer func() { _ = rdb.Close() }()
			rdb.AddHook(scanCancelHook{cancel: cancel})
			if err := scan(ctx, &Client{redis: rdb}); !errors.Is(err, context.Canceled) {
				t.Fatalf("scan error = %v, want cancellation", err)
			}
		})
	}
}
