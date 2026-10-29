package sidekiq

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestDetectVersion_ConcurrentDiscovery(t *testing.T) {
	for name, fixture := range map[string]struct {
		keys []string
		want Version
	}{
		"Sidekiq 7": {keys: []string{"j|20250102|12:00"}, want: Version7},
		"Sidekiq 8": {keys: []string{"j|250102|12:00"}, want: Version8},
		"unknown":   {keys: []string{"j|invalid"}, want: VersionUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				release := make(chan struct{})
				var scans atomic.Int32
				client := newSelectionHookClient(t, func(_ context.Context, cmd *redis.ScanCmd) error {
					scans.Add(1)
					<-release
					cmd.SetVal(fixture.keys, 0)
					return nil
				})
				results := concurrentVersionCalls(t.Context(), client, 32)
				synctest.Wait()
				before := scans.Load()
				close(release)
				for range 32 {
					if got := <-results; got != fixture.want {
						t.Errorf("DetectVersion = %v, want %v", got, fixture.want)
					}
				}
				if before != 1 {
					t.Errorf("concurrent discoveries = %d, want 1", before)
				}
				cached := concurrentVersionCalls(t.Context(), client, 32)
				for range 32 {
					if got := <-cached; got != fixture.want {
						t.Errorf("cached version = %v, want %v", got, fixture.want)
					}
				}
				if got := scans.Load(); got != 1 {
					t.Errorf("SCAN calls = %d, want 1", got)
				}
			})
		})
	}
}

func TestDetectVersion_ConcurrentFailureRecovery(t *testing.T) {
	for name, failure := range map[string]bool{"cancelled": true, "Redis error": false} {
		t.Run(name, func(t *testing.T) {
			for key, want := range map[string]Version{
				"j|20250102|12:00": Version7,
				"j|250102|12:00":   Version8,
				"j|invalid":        VersionUnknown,
			} {
				t.Run(key, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						release := make(chan struct{})
						var scans atomic.Int32
						client := newSelectionHookClient(t, func(ctx context.Context, cmd *redis.ScanCmd) error {
							if scans.Add(1) == 1 {
								<-release
								if failure {
									return ctx.Err()
								}
								return errors.New("ERR temporary scan failure")
							}
							cmd.SetVal([]string{key}, 0)
							return nil
						})
						ctx, cancel := context.WithCancel(t.Context())
						defer cancel()
						first := concurrentVersionCalls(ctx, client, 1)
						synctest.Wait()
						waiters := concurrentVersionCalls(t.Context(), client, 32)
						synctest.Wait()
						before := scans.Load()
						if failure {
							cancel()
						}
						close(release)
						if got := <-first; got != VersionUnknown {
							t.Errorf("failed caller = %v, want VersionUnknown", got)
						}
						for range 32 {
							if got := <-waiters; got != want {
								t.Errorf("recovered waiter = %v, want %v", got, want)
							}
						}
						if before != 1 {
							t.Errorf("discoveries before failure = %d, want 1", before)
						}
						if got := client.DetectVersion(t.Context()); got != want {
							t.Errorf("cached recovery = %v, want %v", got, want)
						}
						if got := scans.Load(); got != 2 {
							t.Errorf("SCAN calls = %d, want failed scan + one successful retry", got)
						}
					})
				})
			}
		})
	}
}

func TestDetectVersion_WaiterCancellation(t *testing.T) {
	for name, deadline := range map[string]bool{"cancelled": false, "deadline": true} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				release := make(chan struct{})
				defer close(release)
				var scans atomic.Int32
				client := newSelectionHookClient(t, func(_ context.Context, cmd *redis.ScanCmd) error {
					scans.Add(1)
					<-release
					cmd.SetVal([]string{"j|20250102|12:00"}, 0)
					return nil
				})
				owner := concurrentVersionCalls(t.Context(), client, 1)
				synctest.Wait()
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				waiters := concurrentVersionCalls(ctx, client, 32)
				synctest.Wait()
				if deadline {
					time.Sleep(time.Second)
				} else {
					cancel()
				}
				synctest.Wait()
				for range 32 {
					select {
					case got := <-waiters:
						if got != VersionUnknown {
							t.Errorf("cancelled waiter = %v, want VersionUnknown", got)
						}
					default:
						t.Fatal("waiter did not return while discovery was blocked")
					}
				}
				select {
				case <-owner:
					t.Fatal("waiter cancellation interrupted the discovery owner")
				default:
				}
				if got := scans.Load(); got != 1 {
					t.Errorf("SCAN calls = %d, want 1", got)
				}
			})
		})
	}
}

func TestDetectVersion_DoesNotBlockOtherRedisCommands(t *testing.T) {
	_, client := setupTestRedis(t)
	release := make(chan struct{})
	started := make(chan struct{})
	client.AddHook(versionCacheScanHook(func(_ context.Context, cmd *redis.ScanCmd) error {
		close(started)
		<-release
		cmd.SetVal([]string{"j|250102|12:00"}, 0)
		return nil
	}))
	result := concurrentVersionCalls(t.Context(), client, 1)
	t.Cleanup(func() { close(release); <-result })
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if got, err := client.Do(ctx, "PING"); err != nil || got != "PONG" {
		t.Fatalf("PING during discovery = %v, %v, want PONG", got, err)
	}
}

func concurrentVersionCalls(ctx context.Context, client *Client, count int) <-chan Version {
	start := make(chan struct{})
	results := make(chan Version, count)
	for range count {
		go func() {
			<-start
			results <- client.DetectVersion(ctx)
		}()
	}
	close(start)
	return results
}

type versionCacheScanHook func(context.Context, *redis.ScanCmd) error

func (h versionCacheScanHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h versionCacheScanHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if scan, ok := cmd.(*redis.ScanCmd); ok {
			return h(ctx, scan)
		}
		return next(ctx, cmd)
	}
}

func (h versionCacheScanHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
