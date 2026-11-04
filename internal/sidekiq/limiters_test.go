package sidekiq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type limiterFixture struct {
	ObservedAt time.Time `json:"observed_at"`
	Records    []struct {
		Key   string          `json:"key"`
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
		TTL   int64           `json:"ttl_ms"`
	} `json:"records"`
}

func loadLimiterFixture(t *testing.T, rdb redis.UniversalClient) limiterFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/limiters/enterprise-8.1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture limiterFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	decode := func(raw json.RawMessage, value any) {
		t.Helper()
		if err := json.Unmarshal(raw, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range fixture.Records {
		if err := rdb.Del(t.Context(), record.Key).Err(); err != nil {
			t.Fatal(err)
		}
		var err error
		switch record.Type {
		case "hash":
			var value map[string]string
			decode(record.Value, &value)
			err = rdb.HSet(t.Context(), record.Key, value).Err()
		case "string":
			var value string
			decode(record.Value, &value)
			err = rdb.Set(t.Context(), record.Key, value, 0).Err()
		case "list":
			var value []string
			decode(record.Value, &value)
			for _, entry := range value {
				if err == nil {
					err = rdb.RPush(t.Context(), record.Key, entry).Err()
				}
			}
		case "zset":
			var value [][]json.RawMessage
			decode(record.Value, &value)
			for _, pair := range value {
				var member string
				var score float64
				decode(pair[0], &member)
				decode(pair[1], &score)
				if err == nil {
					err = rdb.ZAdd(t.Context(), record.Key, redis.Z{Member: member, Score: score}).Err()
				}
			}
		default:
			t.Fatalf("unknown fixture type %s", record.Type)
		}
		if err != nil {
			t.Fatal(err)
		}
		if record.TTL >= 0 {
			if err := rdb.PExpire(t.Context(), record.Key, time.Duration(record.TTL)*time.Millisecond).Err(); err != nil {
				t.Fatal(err)
			}
		}
	}
	return fixture
}

func limiterTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	features, _ := ResolveFeatures([]string{"ent"})
	client, err := NewClient("redis://"+mr.Addr(), WithFeatures(features))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func assertLimiterFixture(t *testing.T, client *Client, fixture limiterFixture) {
	t.Helper()
	ctx := t.Context()
	page, err := client.ScanLimiters(ctx, LimiterCursor{}, "fixture_")
	if err != nil || len(page.Items) != 12 || page.More {
		t.Fatalf("list: %d records, more=%v: %v", len(page.Items), page.More, err)
	}
	for name, test := range map[string]struct {
		kind, size, interval, field, value string
		history                            int
	}{
		"concurrent":         {kind: "Concurrent", size: "2", field: "used", value: "1"},
		"concurrent_metrics": {kind: "Concurrent", size: "1", field: "reclaimed", value: "1"},
		"window_symbol":      {kind: "Window", size: "8", interval: "1"},
		"window_numeric":     {kind: "Window", size: "8", interval: "30"},
		"bucket_second":      {kind: "Bucket", size: "10", interval: "second", history: 60},
		"bucket_minute":      {kind: "Bucket", size: "10", interval: "minute", history: 60},
		"bucket_hour":        {kind: "Bucket", size: "10", interval: "hour", history: 24},
		"bucket_day":         {kind: "Bucket", size: "10", interval: "day", history: 30},
		"leaky":              {kind: "Leaky bucket", size: "2", interval: "20", field: "miss", value: "1"},
		"points":             {kind: "Points", size: "1000", interval: "20", field: "points", value: "850"},
		"leaky_waited":       {kind: "Leaky bucket", size: "1", interval: "1", field: "hit", value: "2"},
		"points_waited":      {kind: "Points", size: "100", interval: "1", field: "hit", value: "2"},
	} {
		t.Run(name, func(t *testing.T) {
			index := slices.IndexFunc(page.Items, func(l Limiter) bool { return l.Name == "fixture_"+name })
			if index < 0 {
				t.Fatal("missing runtime fixture")
			}
			l, err := client.readLimiter(ctx, page.Items[index].Key, true, fixture.ObservedAt)
			if err != nil || l == nil {
				t.Fatalf("detail: %v", err)
			}
			if l.Type != test.kind || l.Values["size"].Raw != test.size || l.Interval != test.interval || len(l.Problems) != 0 {
				t.Fatalf("wrong runtime observation: %+v", l)
			}
			if test.field != "" && l.Values[test.field].Raw != test.value {
				t.Fatalf("%s = %+v", test.field, l.Values[test.field])
			}
			if len(l.History) != test.history {
				t.Fatalf("history length %d, want %d", len(l.History), test.history)
			}
			if test.history > 0 {
				assertRuntimeBucketSamples(t, l.History)
			}
			if name == "concurrent" && l.Values["waited"].Valid {
				t.Fatal("absent runtime metric became zero")
			}
			if name == "concurrent_metrics" {
				for _, field := range []string{"held", "held_ms", "immediate", "waited", "wait_ms", "overtime", "reclaimed"} {
					if !l.Values[field].Valid || l.Values[field].Number <= 0 {
						t.Fatalf("missing real runtime metric %s", field)
					}
				}
			}
			if name == "leaky_waited" || name == "points_waited" {
				if !l.Values["waited"].Valid || l.Values["waited"].Number < 0.9 || l.Values["waited"].Number > 3 {
					t.Fatal("runtime wait metric is not seconds")
				}
			}
			if name == "window_numeric" && l.Values["window_counter"].Raw != "2" {
				t.Fatal("runtime window counter unavailable")
			}
		})
	}
}

func assertRuntimeBucketSamples(t *testing.T, samples []LimiterSample) {
	t.Helper()
	recorded := 0
	for _, sample := range samples {
		if !sample.Value.Valid {
			if sample.Value.Raw != "" {
				t.Fatalf("invalid runtime history: %+v", sample)
			}
			continue
		}
		if sample.Value.Raw != "3" {
			t.Fatalf("history manufactured data: %+v", sample)
		}
		recorded++
	}
	if recorded != 1 {
		t.Fatalf("expected one retained counter, got %d", recorded)
	}
}

func TestLimiterRuntimeFixtures(t *testing.T) {
	client, mr := limiterTestClient(t)
	fixture := loadLimiterFixture(t, client.limiterRedis)
	before := mr.Dump()
	assertLimiterFixture(t, client, fixture)
	keys := []string{"lmtr-c-fixture_concurrent", "lmtr-p-fixture_points"}
	if _, err := client.RefreshLimiters(t.Context(), keys); err != nil {
		t.Fatal(err)
	}
	if after := mr.Dump(); after != before {
		t.Fatal("inspection changed data or TTLs")
	}
}

func TestLimiterMalformedExpiredAndMetrics(t *testing.T) {
	client, mr := limiterTestClient(t)
	ctx := t.Context()
	mr.HSet("lmtr-c-counts", "held", "0", "held_ms", "1234", "immediate", "7", "waited", "2", "wait_ms", "250", "overtime", "3", "reclaimed", "1")
	for key, test := range map[string]struct {
		seed    func()
		missing bool
		problem string
	}{
		"lmtr-c-counts":  {},
		"lmtr-b-missing": {missing: true},
		"lmtr-w-nan":     {seed: func() { mr.HSet("lmtr-w-nan", "size", "NaN", "interval", "junk") }, problem: "Invalid size"},
		"lmtr-b-wrong":   {seed: func() { _ = mr.Set("lmtr-b-wrong", "x") }, problem: "not a Redis hash"},
		"lmtr-k-empty":   {seed: func() { mr.HSet("lmtr-k-empty", "hit", "0") }, problem: "Size unavailable"},
		"lmtr-z-new":     {seed: func() { mr.HSet("lmtr-z-new", "size", "5") }},
	} {
		t.Run(key, func(t *testing.T) {
			if test.seed != nil {
				test.seed()
			}
			l, err := client.GetLimiter(ctx, key)
			if err != nil || l == nil || l.Missing != test.missing {
				t.Fatalf("observation %+v: %v", l, err)
			}
			if test.problem != "" && !strings.Contains(strings.Join(l.Problems, ";"), test.problem) {
				t.Fatalf("problems: %v", l.Problems)
			}
			if key == "lmtr-c-counts" && (!l.Values["held"].Valid || l.Values["held"].Number != 0 || l.Values["held_ms"].Number != 1234 || l.Values["size"].Raw != "0") {
				t.Fatalf("lost recorded zero/duration: %+v", l)
			}
		})
	}
	mr.HSet("lmtr-c-bad-capacity", "held", "1")
	_ = mr.Set("{lmtr-c-bad-capacity}-free", "wrong type")
	l, err := client.GetLimiter(ctx, "lmtr-c-bad-capacity")
	if err != nil || l.Values["free"].Valid || l.Values["size"].Valid || len(l.Problems) == 0 {
		t.Fatalf("wrong capacity: %+v %v", l, err)
	}
	if _, err := client.GetLimiter(ctx, "other"); err == nil {
		t.Fatal("accepted non-limiter key")
	}
	mr.HSet("lmtr-w-changing", "size", "10", "interval", "60")
	if _, err := client.ScanLimiters(ctx, LimiterCursor{}, "changing"); err != nil {
		t.Fatal(err)
	}
	mr.Del("lmtr-w-changing")
	mr.HSet("lmtr-p-changing", "size", "100", "interval", "20")
	l, err = client.GetLimiter(ctx, "lmtr-w-changing")
	if err != nil || !l.Missing {
		t.Fatal("type change resurrected stale selection")
	}
	page, err := client.ScanLimiters(ctx, LimiterCursor{}, "")
	if err != nil || len(page.Items) < 5 {
		t.Fatalf("bad record invalidated listing: %+v %v", page, err)
	}
}

type limiterScanHook struct {
	scan func(context.Context, *redis.ScanCmd) error
}

func (h limiterScanHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h limiterScanHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h limiterScanHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if scan, ok := cmd.(*redis.ScanCmd); ok {
			return h.scan(ctx, scan)
		}
		return next(ctx, cmd)
	}
}

func TestLimiterCursorBudgetContinuationAndDeduplication(t *testing.T) {
	client, mr := limiterTestClient(t)
	for i := range 137 {
		mr.HSet(fmt.Sprintf("lmtr-w-fixture-%03d", i), "size", "10", "interval", "60")
	}
	calls := 0
	client.redis.AddHook(limiterScanHook{scan: func(_ context.Context, cmd *redis.ScanCmd) error {
		calls++
		cursor := cmd.Args()[1].(uint64)
		if cursor < 12 {
			cmd.SetVal(nil, cursor+1)
			return nil
		}
		var keys []string
		for i := range 137 {
			keys = append(keys, fmt.Sprintf("lmtr-w-fixture-%03d", i))
		}
		keys = append(keys, keys[0], keys[42], "invalid", "lmtr-w-expired")
		cmd.SetVal(keys, 0)
		return nil
	}})
	page, err := client.ScanLimiters(t.Context(), LimiterCursor{}, "fixture")
	if err != nil || len(page.Items) != 0 || !page.More || calls != limiterScanBudget {
		t.Fatalf("unbounded scan: %d calls, %+v %v", calls, page, err)
	}
	seen := make(map[string]bool)
	for page.More {
		page, err = client.ScanLimiters(t.Context(), page.Next, "fixture")
		if err != nil || len(page.Items) > limiterPageSize {
			t.Fatalf("page bound: %v", err)
		}
		for _, l := range page.Items {
			if seen[l.Key] {
				t.Fatalf("duplicate key: %s", l.Key)
			}
			seen[l.Key] = true
		}
	}
	if len(seen) != 137 || calls != 13 {
		t.Fatalf("lost records or rescan: %d keys, %d calls", len(seen), calls)
	}
}

func TestLimiterLiteralSearchAndCancellation(t *testing.T) {
	for _, filter := range []string{"*", "?", "[", "]", "\\", "APP", "app"} {
		t.Run(filter, func(t *testing.T) {
			client, mr := limiterTestClient(t)
			mr.HSet("lmtr-w-app"+filter+"tail", "size", "5", "interval", "60")
			mr.HSet("lmtr-w-unrelated", "size", "5", "interval", "60")
			page, err := client.ScanLimiters(t.Context(), LimiterCursor{}, filter)
			if err != nil || len(page.Items) != 1 || !strings.Contains(page.Items[0].Name, filter) {
				t.Fatalf("literal filter: %+v %v", page, err)
			}
		})
	}
	client, _ := limiterTestClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	client.redis.AddHook(limiterScanHook{scan: func(_ context.Context, cmd *redis.ScanCmd) error {
		cmd.SetVal([]string{"lmtr-w-fixture"}, 0)
		cancel()
		return nil
	}})
	if _, err := client.ScanLimiters(ctx, LimiterCursor{}, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestLimiterGuardsAndConnectionFailure(t *testing.T) {
	client, mr := limiterTestClient(t)
	client.capabilities = Capabilities{}
	if _, err := client.ScanLimiters(t.Context(), LimiterCursor{}, ""); !errors.Is(err, ErrLimitersUnsupported) {
		t.Fatal(err)
	}
	if _, err := client.GetLimiter(t.Context(), "lmtr-w-x"); !errors.Is(err, ErrLimitersUnsupported) {
		t.Fatal(err)
	}
	if _, err := client.RefreshLimiters(t.Context(), nil); !errors.Is(err, ErrLimitersUnsupported) {
		t.Fatal(err)
	}
	client.capabilities.rateLimits = true
	mr.Close()
	if _, err := client.GetLimiter(t.Context(), "lmtr-w-x"); err == nil {
		t.Fatal("connection failure swallowed")
	}
}

func TestLimiterHistoryCalendarAndWrongCounters(t *testing.T) {
	client, mr := limiterTestClient(t)
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	client.limiterLocation = location
	now := time.Date(2026, 11, 3, 22, 30, 0, 0, location)
	mr.HSet("lmtr-b-calendar", "size", "20", "interval", "day")
	_ = mr.Set("{lmtr-b-calendar}-2026-11-03", "0")
	mr.HSet("{lmtr-b-calendar}-2026-11-02", "wrong", "type")
	l, err := client.readLimiter(t.Context(), "lmtr-b-calendar", true, now)
	if err != nil || len(l.History) != 30 || l.History[0].Value.Raw != "0" || !l.History[0].Value.Valid || l.History[1].Value.Valid || l.History[1].Value.Raw != "wrong type" {
		t.Fatalf("history: %+v %v", l, err)
	}
	for i, sample := range l.History {
		if sample.At.Day() != now.AddDate(0, 0, -i).Day() {
			t.Fatal("DST shifted calendar bucket")
		}
	}
}

func TestLimiterHistoryRepeatedHourSharesCounter(t *testing.T) {
	client, mr := limiterTestClient(t)
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	client.limiterLocation = location
	// 06:30 UTC is the second 01:30 when Toronto leaves daylight saving time.
	now := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	mr.HSet("lmtr-b-dst", "size", "10", "interval", "hour")
	_ = mr.Set("{lmtr-b-dst}-2026-11-01:01", "7")
	l, err := client.readLimiter(t.Context(), "lmtr-b-dst", true, now)
	if err != nil {
		t.Fatal(err)
	}
	if l.History[0].Value.Raw != "7" || l.History[1].Value.Valid || l.History[1].Note != "Shared calendar bucket (DST)" {
		t.Fatalf("double-counted repeated hour: %+v", l.History[:2])
	}
}

func TestLimiterCursorIsReusable(t *testing.T) {
	client, mr := limiterTestClient(t)
	for i := range 120 {
		mr.HSet(fmt.Sprintf("lmtr-w-immutable-%03d", i), "size", "10", "interval", "60")
	}
	first, err := client.ScanLimiters(t.Context(), LimiterCursor{}, "immutable")
	if err != nil || !first.More {
		t.Fatalf("first page: %v", err)
	}
	second, err := client.ScanLimiters(t.Context(), first.Next, "immutable")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := client.ScanLimiters(t.Context(), first.Next, "immutable")
	if err != nil || len(second.Items) != len(repeated.Items) {
		t.Fatalf("cursor mutated after use: %v", err)
	}
	for i, item := range second.Items {
		if item.Key != repeated.Items[i].Key {
			t.Fatal("continuation was mutated by a later read")
		}
	}
}

func TestLimiterSeparateConnectionAndOptions(t *testing.T) {
	features, _ := ResolveFeatures([]string{"ent"})
	job, limiter := miniredis.RunT(t), miniredis.RunT(t)
	limiter.RequireAuth("synthetic-secret")
	client, err := NewClient("redis://"+job.Addr(), WithFeatures(features), WithLimiterConnection(LimiterConnection{URL: "redis://:synthetic-secret@" + limiter.Addr() + "/2", Timezone: "America/Toronto"}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if err := client.limiterRedis.HSet(t.Context(), "lmtr-w-separate", "size", 5, "interval", 60).Err(); err != nil {
		t.Fatal(err)
	}
	page, err := client.ScanLimiters(t.Context(), LimiterCursor{}, "")
	if err != nil || len(page.Items) != 1 || len(job.Keys()) != 0 {
		t.Fatalf("wrong pool: %+v %v", page, err)
	}
	if strings.Contains(client.DisplayLimiterRedis(), "secret") || !strings.Contains(client.DisplayLimiterRedis(), "DB 2") {
		t.Fatal("unsafe/wrong connection label")
	}
	owned := client.limiterRedis
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owned.Ping(t.Context()).Err(); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("owned pool not closed: %v", err)
	}
	for name, config := range map[string]LimiterConnection{
		"tls":     {URL: "rediss://user:" + "synthetic-secret@localhost:6380/3?pool_fifo=true"},
		"cluster": {URL: "rediss://user:" + "synthetic-secret@localhost:6380/0?addr=localhost:6381", Cluster: true},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := NewClient("redis://localhost", WithFeatures(features), WithLimiterConnection(config))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = c.Close() }()
			if strings.Contains(c.DisplayLimiterRedis(), "secret") || strings.Contains(c.DisplayLimiterRedis(), "user") {
				t.Fatal("credential disclosure")
			}
			if cluster, ok := c.limiterRedis.(*redis.ClusterClient); ok {
				o := cluster.Options()
				if o.TLSConfig == nil || o.Username != "user" || o.Password != "synthetic-secret" || len(o.Addrs) != 2 {
					t.Fatal("cluster options lost")
				}
			} else {
				o := c.limiterRedis.(*redis.Client).Options()
				if o.TLSConfig == nil || o.DB != 3 || o.Password != "synthetic-secret" || !o.PoolFIFO {
					t.Fatal("standalone options lost")
				}
			}
		})
	}
	for _, config := range []LimiterConnection{{Cluster: true}, {URL: "redis://user:" + "synthetic-secret@localhost/%zz"}, {URL: "redis://user:" + "synthetic-secret@localhost/2", Cluster: true}, {Timezone: "bad/zone"}, {URL: "rediss://user:" + "synthetic-secret@localhost?addr=synthetic-secret@localhost:123", Cluster: true}} {
		c, err := NewClient("redis://localhost", WithFeatures(features), WithLimiterConnection(config))
		if c != nil {
			_ = c.Close()
		}
		if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatalf("invalid config accepted or disclosed: %v", err)
		}
	}
}
