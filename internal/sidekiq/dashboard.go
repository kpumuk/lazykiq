package sidekiq

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisInfo holds Redis INFO fields needed for the dashboard.
type RedisInfo struct {
	Version        string
	UptimeDays     int64
	Connections    int64
	UsedMemory     string
	UsedMemoryPeak string
}

// DashboardRealtime combines stats and Redis info for the realtime dashboard pane.
type DashboardRealtime struct {
	Stats     Stats
	RedisInfo RedisInfo
	FetchedAt time.Time
}

// StatsHistory holds daily processed and failed counts.
type StatsHistory struct {
	// Use parallel slices to match chart data sets without extra struct mapping.
	Dates     []time.Time
	Processed []int64
	Failed    []int64
}

// GetRedisInfo fetches Redis INFO and extracts fields used on the dashboard.
func (c *Client) GetRedisInfo(ctx context.Context) (RedisInfo, error) {
	info := RedisInfo{}
	raw, err := c.redis.Info(ctx, "server", "clients", "memory").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return info, err
	}

	parsed := parseInfo(raw)
	info.Version = parsed["redis_version"]
	info.UsedMemory = parsed["used_memory_human"]
	info.UsedMemoryPeak = parsed["used_memory_peak_human"]

	if v, ok := parsed["uptime_in_days"]; ok {
		info.UptimeDays, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := parsed["connected_clients"]; ok {
		info.Connections, _ = strconv.ParseInt(v, 10, 64)
	}

	return info, nil
}

// GetDashboardRealtime fetches realtime dashboard data in one call.
func (c *Client) GetDashboardRealtime(ctx context.Context) (DashboardRealtime, error) {
	stats, err := c.GetStats(ctx)
	if err != nil {
		return DashboardRealtime{}, err
	}
	redisInfo, err := c.GetRedisInfo(ctx)
	if err != nil {
		return DashboardRealtime{}, err
	}
	return DashboardRealtime{
		Stats:     stats,
		RedisInfo: redisInfo,
		FetchedAt: time.Now(),
	}, nil
}

// GetStatsHistory fetches per-day processed and failed stats for the last N days.
func (c *Client) GetStatsHistory(ctx context.Context, days int) (StatsHistory, error) {
	if days < 1 {
		days = 1
	}

	endDate := time.Now().UTC()
	dates := make([]time.Time, 0, days)
	processedKeys := make([]string, 0, days)
	failedKeys := make([]string, 0, days)

	for i := days - 1; i >= 0; i-- {
		date := endDate.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		dates = append(dates, date)
		processedKeys = append(processedKeys, "stat:processed:"+dateStr)
		failedKeys = append(failedKeys, "stat:failed:"+dateStr)
	}

	processed, err := c.redis.MGet(ctx, processedKeys...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return StatsHistory{}, err
	}
	failed, err := c.redis.MGet(ctx, failedKeys...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return StatsHistory{}, err
	}

	history := StatsHistory{
		Dates:     dates,
		Processed: make([]int64, len(dates)),
		Failed:    make([]int64, len(dates)),
	}

	for i := range dates {
		history.Processed[i] = parseInt64(processed[i])
		history.Failed[i] = parseInt64(failed[i])
	}

	return history, nil
}

func parseInfo(raw string) map[string]string {
	values := make(map[string]string)
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		values[parts[0]] = strings.TrimSpace(parts[1])
	}
	return values
}

func parseInt64(value any) int64 {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	case int64:
		return v
	default:
		return 0
	}
}
