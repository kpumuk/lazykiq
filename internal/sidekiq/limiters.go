package sidekiq

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	limiterPageSize   = 50
	limiterScanBudget = 10
)

var (
	// ErrLimitersUnsupported means inspection was not explicitly enabled.
	ErrLimitersUnsupported = errors.New("rate limits require --enable=ent")
	// ErrLimiterTopologyChanged prevents continuing an incomplete cluster scan.
	ErrLimiterTopologyChanged = errors.New("limiter cluster primaries changed; reset discovery with ctrl+r")
)

// LimiterValue distinguishes an absent or malformed value from recorded zero.
type LimiterValue struct {
	Raw    string
	Number float64
	Valid  bool
}

// Limiter is a read-only observation, not an executable rate limiter.
type Limiter struct {
	Key, Name, Type string
	Values          map[string]LimiterValue
	Interval        string
	TTL             time.Duration
	Missing         bool
	Problems        []string
	History         []LimiterSample
	HistoryZone     string
	ObservedAt      time.Time
}

// LimiterSample contains one retained bucket counter. Absent counters are unknown.
type LimiterSample struct {
	At    time.Time
	Value LimiterValue
	Note  string
}

type limiterNodeCursor struct {
	Address string
	Cursor  uint64
	Done    bool
}

// LimiterCursor is an immutable continuation. Its zero value begins discovery.
// SCAN is not a snapshot; seen keys prevent duplicate rows during a traversal.
type LimiterCursor struct {
	nodes   []limiterNodeCursor
	pending []string
	seen    *limiterSeen
	filter  string
}

// Each continuation adds only its newly visited keys. Older immutable cursors
// share these blocks instead of retaining a full copied set for every page.
type limiterSeen struct {
	previous *limiterSeen
	keys     map[string]bool
}

func (s *limiterSeen) contains(key string) bool {
	for block := s; block != nil; block = block.previous {
		if block.keys[key] {
			return true
		}
	}
	return false
}

// LimiterPage is bounded even when Redis returns empty intermediate SCAN batches.
type LimiterPage struct {
	Items []Limiter
	Next  LimiterCursor
	More  bool
}

var limiterFields = []string{"size", "interval", "held", "held_ms", "immediate", "waited", "wait_ms", "overtime", "reclaimed", "drops", "points", "hit", "miss", "last"}

// ScanLimiters performs at most ten SCAN calls and returns at most fifty records.
// Filtering is a case-sensitive literal name substring, never a Redis glob.
func (c *Client) ScanLimiters(ctx context.Context, cursor LimiterCursor, filter string) (LimiterPage, error) {
	if !c.capabilities.SupportsRateLimits() {
		return LimiterPage{}, ErrLimitersUnsupported
	}
	nodes, err := c.limiterNodes(ctx)
	if err != nil {
		return LimiterPage{}, err
	}
	next := LimiterCursor{nodes: slices.Clone(cursor.nodes), pending: slices.Clone(cursor.pending), seen: cursor.seen, filter: filter}
	addresses := slices.Sorted(maps.Keys(nodes))
	if len(next.nodes) == 0 || cursor.filter != filter {
		next = LimiterCursor{filter: filter}
		for _, address := range addresses {
			next.nodes = append(next.nodes, limiterNodeCursor{Address: address})
		}
	} else {
		previous := make([]string, len(next.nodes))
		for i, node := range next.nodes {
			previous[i] = node.Address
		}
		if !slices.Equal(previous, addresses) {
			return LimiterPage{}, ErrLimiterTopologyChanged
		}
	}
	page := LimiterPage{}
	visited := make(map[string]bool)
	scans := 0
	for len(page.Items) < limiterPageSize {
		if err := ctx.Err(); err != nil {
			return LimiterPage{}, err
		}
		if len(next.pending) == 0 {
			index := slices.IndexFunc(next.nodes, func(n limiterNodeCursor) bool { return !n.Done })
			if index < 0 || scans == limiterScanBudget {
				break
			}
			node := &next.nodes[index]
			keys, continuation, scanErr := nodes[node.Address].Scan(ctx, node.Cursor, "lmtr-?-*", 100).Result()
			if scanErr != nil {
				return LimiterPage{}, scanErr
			}
			scans++
			node.Cursor, node.Done = continuation, continuation == 0
			next.pending = keys
			continue
		}
		key := next.pending[0]
		next.pending = next.pending[1:]
		_, name, valid := limiterIdentity(key)
		if !valid || !strings.Contains(name, filter) || visited[key] || next.seen.contains(key) {
			continue
		}
		visited[key] = true
		record, readErr := c.readLimiter(ctx, key, false, time.Now())
		if readErr != nil {
			return LimiterPage{}, readErr
		}
		if record != nil && !record.Missing {
			page.Items = append(page.Items, *record)
		}
	}
	if len(visited) > 0 {
		next.seen = &limiterSeen{previous: next.seen, keys: visited}
	}
	page.Next = next
	page.More = len(next.pending) > 0 || slices.ContainsFunc(next.nodes, func(n limiterNodeCursor) bool { return !n.Done })
	slices.SortFunc(page.Items, func(a, b Limiter) int { return strings.Compare(a.Key, b.Key) })
	return page, nil
}

func (c *Client) limiterNodes(ctx context.Context) (map[string]*redis.Client, error) {
	if cluster, ok := c.limiterRedis.(*redis.ClusterClient); ok {
		nodes := make(map[string]*redis.Client)
		var mu sync.Mutex
		err := cluster.ForEachMaster(ctx, func(_ context.Context, node *redis.Client) error {
			mu.Lock()
			nodes[node.Options().Addr] = node
			mu.Unlock()
			return nil
		})
		return nodes, err
	}
	return map[string]*redis.Client{"": c.redisForLimiters()}, nil
}

func (c *Client) redisForLimiters() *redis.Client {
	if node, ok := c.limiterRedis.(*redis.Client); ok {
		return node
	}
	return c.redis
}

// RefreshLimiters updates a page without changing its discovery cursor or order.
func (c *Client) RefreshLimiters(ctx context.Context, keys []string) ([]Limiter, error) {
	if !c.capabilities.SupportsRateLimits() {
		return nil, ErrLimitersUnsupported
	}
	if len(keys) > limiterPageSize {
		return nil, errors.New("limiter refresh exceeds page size")
	}
	items := make([]Limiter, 0, len(keys))
	for _, key := range keys {
		record, err := c.readLimiter(ctx, key, false, time.Now())
		if err != nil {
			return nil, err
		}
		if record != nil {
			items = append(items, *record)
		}
	}
	return items, nil
}

// GetLimiter reads current state and retained bucket counters. It never repairs,
// acquires, releases, resets, or extends the lifetime of a limiter.
func (c *Client) GetLimiter(ctx context.Context, key string) (*Limiter, error) {
	if !c.capabilities.SupportsRateLimits() {
		return nil, ErrLimitersUnsupported
	}
	return c.readLimiter(ctx, key, true, time.Now())
}

func limiterIdentity(key string) (string, string, bool) {
	if len(key) < 8 || !strings.HasPrefix(key, "lmtr-") || key[6] != '-' {
		return "", "", false
	}
	kind := map[byte]string{'c': "Concurrent", 'w': "Window", 'b': "Bucket", 'k': "Leaky bucket", 'p': "Points"}[key[5]]
	if kind == "" {
		kind = "Unsupported (" + key[5:6] + ")"
	}
	return kind, key[7:], true
}

func (c *Client) readLimiter(ctx context.Context, key string, history bool, now time.Time) (*Limiter, error) {
	kind, name, valid := limiterIdentity(key)
	if !valid {
		return nil, errors.New("invalid limiter key")
	}
	record := &Limiter{Key: key, Name: name, Type: kind, Values: make(map[string]LimiterValue), ObservedAt: now}
	pipe := c.limiterRedis.Pipeline()
	if kind == "Concurrent" {
		pipe = c.limiterRedis.TxPipeline()
	}
	props := pipe.HMGet(ctx, key, limiterFields...)
	ttl := pipe.PTTL(ctx, key)
	var free, used *redis.IntCmd
	if kind == "Concurrent" {
		free = pipe.LLen(ctx, "{"+key+"}-free")
		used = pipe.ZCard(ctx, "{"+key+"}-used")
	}
	_, err := pipe.Exec(ctx)
	if err != nil && !limiterDataError(err) {
		return nil, err
	}
	if ttl.Err() != nil {
		return nil, ttl.Err()
	}
	record.TTL = ttl.Val()
	if record.TTL == -2*time.Nanosecond {
		record.Missing = true
		return record, nil
	}
	if props.Err() != nil {
		record.Problems = append(record.Problems, "Record is not a Redis hash")
		return record, nil //nolint:nilerr // A wrong-type record is inspectable malformed data, not a connection failure.
	}
	for i, field := range limiterFields {
		if raw, ok := props.Val()[i].(string); ok {
			if field == "interval" {
				record.Interval = raw
				continue
			}
			value := parseLimiterValue(raw)
			record.Values[field] = value
			if !value.Valid {
				record.Problems = append(record.Problems, "Invalid "+field)
			}
		}
	}
	if kind == "Concurrent" {
		if err := readLimiterCapacity(record, free, used); err != nil {
			return nil, err
		}
	} else if !strings.HasPrefix(kind, "Unsupported") {
		if value := record.Values["size"]; !value.Valid || value.Number < 0 {
			record.Problems = append(record.Problems, "Size unavailable or invalid")
		}
		if seconds, ok := limiterIntervalSeconds(record.Interval); !ok || seconds <= 0 {
			record.Problems = append(record.Problems, "Interval unavailable or invalid")
		}
	}
	if history && kind == "Bucket" {
		if err := c.readLimiterHistory(ctx, record, now); err != nil {
			return nil, err
		}
	}
	if history && kind == "Window" {
		if err := c.readLimiterWindow(ctx, record); err != nil {
			return nil, err
		}
	}
	return record, nil
}

func (c *Client) readLimiterWindow(ctx context.Context, record *Limiter) error {
	seconds := parseLimiterValue(record.Interval)
	if !seconds.Valid || seconds.Number <= 0 {
		return nil
	}
	key := "lmtr-wdata-" + record.Name + "-" + record.Interval
	pipe := c.limiterRedis.Pipeline()
	value, ttl := pipe.Get(ctx, key), pipe.PTTL(ctx, key)
	_, err := pipe.Exec(ctx)
	if err != nil && !limiterDataError(err) {
		return err
	}
	if value.Err() == nil {
		record.Values["window_counter"] = parseLimiterValue(value.Val())
	}
	if value.Err() != nil && !errors.Is(value.Err(), redis.Nil) {
		record.Problems = append(record.Problems, "Invalid current window counter")
	}
	if ttl.Err() != nil {
		return ttl.Err()
	}
	if ttl.Val() >= 0 {
		record.Values["window_ttl_ms"] = parseLimiterValue(strconv.FormatInt(ttl.Val().Milliseconds(), 10))
	}
	return nil
}

func readLimiterCapacity(record *Limiter, free, used *redis.IntCmd) error {
	for _, field := range []struct {
		name   string
		result *redis.IntCmd
	}{{"free", free}, {"used", used}} {
		if field.result.Err() == nil {
			record.Values[field.name] = parseLimiterValue(strconv.FormatInt(field.result.Val(), 10))
			continue
		}
		if !limiterDataError(field.result.Err()) {
			return field.result.Err()
		}
		record.Problems = append(record.Problems, "Invalid "+field.name+" capacity key")
	}
	if free.Err() == nil && used.Err() == nil {
		record.Values["size"] = parseLimiterValue(strconv.FormatInt(free.Val()+used.Val(), 10))
	}
	return nil
}

func parseLimiterValue(raw string) LimiterValue {
	number, err := strconv.ParseFloat(raw, 64)
	return LimiterValue{Raw: raw, Number: number, Valid: err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)}
}

func limiterDataError(err error) bool {
	return errors.Is(err, redis.Nil) || strings.HasPrefix(err.Error(), "WRONGTYPE")
}

func limiterIntervalSeconds(interval string) (float64, bool) {
	if seconds, ok := map[string]float64{"second": 1, "minute": 60, "hour": 3600, "day": 86400}[interval]; ok {
		return seconds, true
	}
	value := parseLimiterValue(interval)
	return value.Number, value.Valid
}

func (c *Client) readLimiterHistory(ctx context.Context, record *Limiter, now time.Time) error {
	type bucketResolution struct {
		step   time.Duration
		count  int
		layout string
	}
	resolutionByInterval := map[string]bucketResolution{
		"second": {time.Second, 60, "04:05"}, "minute": {time.Minute, 60, "15:04"},
		"hour": {time.Hour, 24, "2006-01-02:15"}, "day": {24 * time.Hour, 30, "2006-01-02"},
	}
	resolution, ok := resolutionByInterval[record.Interval]
	if !ok {
		record.Problems = append(record.Problems, "History unavailable for this bucket interval")
		return nil
	}
	location := c.limiterLocation
	if location == nil {
		location = time.UTC
	}
	record.HistoryZone = location.String()
	pipe := c.limiterRedis.Pipeline()
	commands := make([]*redis.StringCmd, resolution.count)
	seen := make(map[string]bool, resolution.count)
	for i := range commands {
		at := now.In(location).Add(-time.Duration(i) * resolution.step)
		if record.Interval == "day" {
			at = now.In(location).AddDate(0, 0, -i)
		}
		record.History = append(record.History, LimiterSample{At: at})
		key := "{" + record.Key + "}-" + at.Format(resolution.layout)
		if seen[key] {
			record.History[i].Note = "Shared calendar bucket (DST)"
			continue
		}
		seen[key] = true
		commands[i] = pipe.Get(ctx, key)
	}
	_, err := pipe.Exec(ctx)
	if err != nil && !limiterDataError(err) {
		return err
	}
	for i, command := range commands {
		if command == nil {
			continue
		}
		switch {
		case command.Err() == nil:
			record.History[i].Value = parseLimiterValue(command.Val())
		case !limiterDataError(command.Err()):
			return command.Err()
		case !errors.Is(command.Err(), redis.Nil):
			record.History[i].Value.Raw = "wrong type"
		}
	}
	return nil
}

// Rate describes configuration with units. Concurrent capacity is observed.
func (l Limiter) Rate() string {
	size := l.Values["size"]
	if !size.Valid {
		return "Unavailable"
	}
	switch l.Type {
	case "Concurrent":
		return size.Raw + " slots"
	case "Leaky bucket", "Points":
		unit := "operations"
		if l.Type == "Points" {
			unit = "points"
		}
		return size.Raw + " " + unit + " / " + l.Interval + " s"
	default:
		interval := l.Interval
		if _, err := strconv.ParseFloat(interval, 64); err == nil {
			interval += " s"
		}
		return size.Raw + " / " + interval
	}
}

// UsageNote describes observed usage or a problem that needs attention.
func (l Limiter) UsageNote() string {
	if l.Missing {
		return "Disappeared"
	}
	if len(l.Problems) != 0 {
		return "Malformed: " + strings.Join(l.Problems, "; ")
	}
	if strings.HasPrefix(l.Type, "Unsupported") {
		return "Unsupported type"
	}
	if l.Type == "Concurrent" {
		return fmt.Sprintf("%s used / %s free", l.Values["used"].Raw, l.Values["free"].Raw)
	}
	return ""
}
