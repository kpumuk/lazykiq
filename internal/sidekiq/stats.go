package sidekiq

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// Stats holds Sidekiq statistics.
type Stats struct {
	Processed int64
	Failed    int64
	Busy      int64
	Enqueued  int64
	Retries   int64
	Scheduled int64
	Dead      int64
}

// getStatsScript fetches all stats in a single round-trip using Lua.
var getStatsScript = redis.NewScript(`
local processed = tonumber(redis.call('GET', 'stat:processed')) or 0
local failed = tonumber(redis.call('GET', 'stat:failed')) or 0
local retries = redis.call('ZCARD', 'retry')
local scheduled = redis.call('ZCARD', 'schedule')
local dead = redis.call('ZCARD', 'dead')

local processes = redis.call('SMEMBERS', 'processes')
local queues = redis.call('SMEMBERS', 'queues')

local busy = 0
for _, proc in ipairs(processes) do
    local b = redis.call('HGET', proc, 'busy')
    if b then
        busy = busy + tonumber(b)
    end
end

local enqueued = 0
for _, q in ipairs(queues) do
    enqueued = enqueued + redis.call('LLEN', 'queue:' .. q)
end

return {processed, failed, retries, scheduled, dead, busy, enqueued}
`)

// GetStats fetches current Sidekiq statistics from Redis.
// Uses a Lua script for single round-trip execution.
func (c *Client) GetStats(ctx context.Context) (Stats, error) {
	result, err := getStatsScript.Run(ctx, c.redis, nil).Slice()
	if err != nil {
		return Stats{}, err
	}

	if len(result) < 7 {
		return Stats{}, nil
	}

	return Stats{
		Processed: result[0].(int64),
		Failed:    result[1].(int64),
		Retries:   result[2].(int64),
		Scheduled: result[3].(int64),
		Dead:      result[4].(int64),
		Busy:      result[5].(int64),
		Enqueued:  result[6].(int64),
	}, nil
}
