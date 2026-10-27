package sidekiq

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

// ErrQueuePauseUnsupported means queue pause support was not enabled.
var ErrQueuePauseUnsupported = errors.New("queue pausing requires --enable=pro or --enable=ent and Sidekiq Pro 7.3.5+ or compatible Enterprise")

// Paused reports the persistent queue pause flag used by Sidekiq Pro.
func (q *Queue) Paused(ctx context.Context) (bool, error) {
	capabilities := q.client.GetCapabilities()
	if !capabilities.SupportsQueuePause() {
		return false, ErrQueuePauseUnsupported
	}
	return q.client.redis.SIsMember(ctx, "paused", q.name).Result()
}

// A script prevents a failed set update from publishing a configuration event.
// Publish even an idempotent request, matching Sidekiq and re-notifying workers.
var queuePauseScript = redis.NewScript(`
redis.call(ARGV[1], KEYS[1], ARGV[2])
return redis.call("PUBLISH", ARGV[3], ARGV[4])
`)

// SetPaused updates the pause flag and notifies running commercial workers.
// Existing jobs may finish; newly enqueued jobs remain in the queue.
func (q *Queue) SetPaused(ctx context.Context, paused bool) error {
	capabilities := q.client.GetCapabilities()
	if !capabilities.SupportsQueuePause() {
		return ErrQueuePauseUnsupported
	}
	command, verb := "SREM", "unpause"
	if paused {
		command, verb = "SADD", "pause"
	}
	payload := queueConfigMessage(verb, q.name, q.client.redis.Options().DB)
	return queuePauseScript.Run(ctx, q.client.redis, []string{"paused"}, command, q.name, "sidekiq:config", payload).Err()
}

// queueConfigMessage encodes only Sidekiq::Pro::Message(verb, UTF-8 name, db)
// in Ruby Marshal 4.8 format. Modern Pro requires the Redis DB field because
// Redis pub/sub channels are shared by all database indexes.
func queueConfigMessage(verb, name string, db int) []byte {
	message := []byte("\x04\x08S:\x1aSidekiq::Pro::Message\x08:\x09verb:")
	message = appendMarshalInteger(message, len(verb))
	message = append(message, verb...)
	message = append(message, []byte(":\x0cpayloadI\"")...)
	message = appendMarshalInteger(message, len(name))
	message = append(message, name...)
	message = append(message, []byte("\x06:\x06ET:\x07dbi")...)
	return appendMarshalInteger(message, db)
}

// Marshal lengths and Redis DB indexes are nonnegative integers.
func appendMarshalInteger(dst []byte, value int) []byte {
	if value == 0 {
		return append(dst, 0)
	}
	if value > 0 && value < 123 {
		return append(dst, byte(value+5))
	}
	var bytes [8]byte
	count := 0
	for value > 0 {
		bytes[count] = byte(value)
		value >>= 8
		count++
	}
	dst = append(dst, byte(count))
	return append(dst, bytes[:count]...)
}

// GetPausedQueues reads one snapshot of the pause flags.
func (c *Client) GetPausedQueues(ctx context.Context) (map[string]bool, error) {
	names, err := c.redis.SMembers(ctx, "paused").Result()
	if err != nil {
		return nil, err
	}
	paused := make(map[string]bool, len(names))
	for _, name := range names {
		paused[name] = true
	}
	return paused, nil
}
