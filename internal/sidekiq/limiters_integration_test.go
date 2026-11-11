package sidekiq

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// These opt-in tests require dedicated disposable services, never production.
func TestLimiterLiveRedis(t *testing.T) {
	for name, variable := range map[string]string{"standalone": "LAZYKIQ_TEST_LIMITER_REDIS", "cluster": "LAZYKIQ_TEST_LIMITER_CLUSTER"} {
		t.Run(name, func(t *testing.T) {
			address := os.Getenv(variable)
			if address == "" {
				t.Skip(variable + " is not set")
			}
			features, _ := ResolveFeatures([]string{"ent"})
			client, err := NewClient("redis://127.0.0.1:1", WithFeatures(features), WithLimiterConnection(LimiterConnection{URL: address, Cluster: name == "cluster"}))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = client.Close() }()
			fixture := loadLimiterFixture(t, client.limiterRedis)
			t.Cleanup(func() { // Close has already run, so use a separate disposable connection.
				cleanup, err := NewClient("redis://127.0.0.1:1", WithFeatures(features), WithLimiterConnection(LimiterConnection{URL: address, Cluster: name == "cluster"}))
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = cleanup.Close() }()
				for _, record := range fixture.Records {
					_ = cleanup.limiterRedis.Del(context.Background(), record.Key).Err()
				}
			})
			beforeCount := limiterNodeKeyCount(t, client)
			type snapshot struct {
				value string
				ttl   time.Duration
			}
			before := make(map[string]snapshot)
			for _, record := range fixture.Records {
				value, err := client.limiterRedis.Dump(t.Context(), record.Key).Result()
				if err != nil {
					t.Fatal(err)
				}
				before[record.Key] = snapshot{value, client.limiterRedis.PTTL(t.Context(), record.Key).Val()}
			}
			assertLimiterFixture(t, client, fixture)
			for key, previous := range before {
				value, err := client.limiterRedis.Dump(t.Context(), key).Result()
				if err != nil || value != previous.value {
					t.Fatalf("record changed: %s %v", key, err)
				}
				ttl := client.limiterRedis.PTTL(t.Context(), key).Val()
				if ttl > previous.ttl || (previous.ttl >= 0 && ttl < 0) {
					t.Fatalf("TTL extended: %s %s -> %s", key, previous.ttl, ttl)
				}
			}
			if after := limiterNodeKeyCount(t, client); after != beforeCount {
				t.Fatalf("inspection created keys: %d -> %d", beforeCount, after)
			}
			if name == "cluster" {
				assertLimiterClusterDiscovery(t, client)
			}
		})
	}
}

func limiterNodeKeyCount(t *testing.T, client *Client) int64 {
	t.Helper()
	nodes, err := client.limiterNodes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	for _, node := range nodes {
		n, err := node.DBSize(t.Context()).Result()
		if err != nil {
			t.Fatal(err)
		}
		count += n
	}
	return count
}

func assertLimiterClusterDiscovery(t *testing.T, client *Client) {
	t.Helper()
	keys := make([]string, 0, 301)
	for i := range 301 {
		key := fmt.Sprintf("lmtr-w-cluster_fixture_%03d", i)
		keys = append(keys, key)
		if err := client.limiterRedis.HSet(t.Context(), key, "size", 5, "interval", 60).Err(); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, key := range keys {
			_ = client.limiterRedis.Del(context.Background(), key).Err()
		}
	}()
	nodes, err := client.limiterNodes(t.Context())
	if err != nil || len(nodes) < 3 {
		t.Fatalf("expected a real cluster with at least 3 primaries: %d %v", len(nodes), err)
	}
	for _, node := range nodes {
		found := 0
		var cursor uint64
		for {
			batch, next, err := node.Scan(t.Context(), cursor, "lmtr-w-cluster_fixture_*", 100).Result()
			if err != nil {
				t.Fatal(err)
			}
			found += len(batch)
			cursor = next
			if next == 0 {
				break
			}
		}
		if found == 0 {
			t.Fatal("fixture failed to exercise every primary")
		}
	}
	seen := make(map[string]bool)
	var cursor LimiterCursor
	for {
		page, err := client.ScanLimiters(t.Context(), cursor, "cluster_fixture_")
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range page.Items {
			if seen[l.Key] || !strings.HasPrefix(l.Name, "cluster_fixture_") {
				t.Fatal("duplicate or wrong cluster record")
			}
			seen[l.Key] = true
		}
		if !page.More {
			break
		}
		cursor = page.Next
	}
	if len(seen) != len(keys) {
		t.Fatalf("only discovered %d/%d distributed limiters", len(seen), len(keys))
	}
}
