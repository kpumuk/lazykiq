package sidekiq

import (
	"context"
	"errors"
	"strconv"
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
	data, err := c.redis.InfoMap(ctx, "server", "clients", "memory").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return info, err
	}

	if server, ok := data["Server"]; ok {
		info.Version = server["redis_version"]
		if v, ok := server["uptime_in_days"]; ok {
			info.UptimeDays, _ = strconv.ParseInt(v, 10, 64)
		}
	}

	if clients, ok := data["Clients"]; ok {
		if v, ok := clients["connected_clients"]; ok {
			info.Connections, _ = strconv.ParseInt(v, 10, 64)
		}
	}

	if memory, ok := data["Memory"]; ok {
		info.UsedMemory = memory["used_memory_human"]
		info.UsedMemoryPeak = memory["used_memory_peak_human"]
	}

	return info, nil
}

// GetStatsHistory fetches per-day processed and failed stats for the last N days.
func (c *Client) GetStatsHistory(ctx context.Context, days int) (StatsHistory, error) {
	if days < 1 {
		days = 1
	}

	endDate := time.Now().UTC()
	dates := make([]time.Time, 0, days)
	allKeys := make([]string, 0, days*2)

	// Build processed keys first, then failed keys
	for i := days - 1; i >= 0; i-- {
		date := endDate.AddDate(0, 0, -i)
		dates = append(dates, date)
		allKeys = append(allKeys, "stat:processed:"+date.Format("2006-01-02"))
	}
	for i := days - 1; i >= 0; i-- {
		date := endDate.AddDate(0, 0, -i)
		allKeys = append(allKeys, "stat:failed:"+date.Format("2006-01-02"))
	}

	// Single MGET for all keys
	results, err := c.redis.MGet(ctx, allKeys...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return StatsHistory{}, err
	}

	history := StatsHistory{
		Dates:     dates,
		Processed: make([]int64, days),
		Failed:    make([]int64, days),
	}

	for i := range days {
		history.Processed[i], _ = parseOptionalInt64(results[i])
		history.Failed[i], _ = parseOptionalInt64(results[days+i])
	}

	return history, nil
}
