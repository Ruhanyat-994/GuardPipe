package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis keys for GitHub live scanning (BUILD_GUIDE.md Phase 17 Part B). All
// of it is disposable coordination state — losing it on a Redis restart
// costs at most one missed or duplicated webhook scan, never data
// (documentation/03-architecture-overview.md §5: Redis is explicitly
// losable).
const (
	webhookEventsKey      = "gp:webhook:events"
	webhookSeenPrefix     = "gp:webhook:seen:"
	webhookPendingKey     = "gp:webhook:pending"      // sorted set: member = debounce key, score = fire-at (unix ms)
	webhookPendingDataKey = "gp:webhook:pending:data" // hash: debounce key -> latest payload
	webhookCountPrefix    = "gp:webhook:count:"
)

// armPendingScript stores the latest payload for key and, only if no timer
// is running for key yet, starts one. Later events inside the window just
// overwrite the payload (so the scan uses the newest push) without pushing
// the fire time back — a steady stream of pushes still produces a scan.
var armPendingScript = redis.NewScript(`
redis.call('HSET', KEYS[2], ARGV[1], ARGV[2])
redis.call('ZADD', KEYS[1], 'NX', ARGV[3], ARGV[1])
return 1
`)

// popPendingScript atomically claims one due key: removes its timer and
// returns + deletes its payload. Returns nil if another worker already
// claimed it, so two worker replicas never fire the same debounced scan.
var popPendingScript = redis.NewScript(`
if redis.call('ZREM', KEYS[1], ARGV[1]) == 0 then return false end
local v = redis.call('HGET', KEYS[2], ARGV[1])
redis.call('HDEL', KEYS[2], ARGV[1])
return v
`)

// LiveScanStore is the Redis half of live scanning: the webhook event queue,
// delivery de-duplication, push debouncing, and the per-project trigger
// counter behind the hourly cap and circuit breaker.
type LiveScanStore struct {
	client *redis.Client
}

func NewLiveScanStore(client *redis.Client) *LiveScanStore {
	return &LiveScanStore{client: client}
}

// MarkDeliverySeen records a GitHub delivery ID and reports whether this is
// the first time it's been seen. GitHub redelivers on timeouts and lets a
// user click "Redeliver" — both must not produce a second scan.
func (s *LiveScanStore) MarkDeliverySeen(ctx context.Context, deliveryID string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, webhookSeenPrefix+deliveryID, 1, ttl).Result()
}

// PushEvent queues a verified webhook event for the worker.
func (s *LiveScanStore) PushEvent(ctx context.Context, payload []byte) error {
	return s.client.LPush(ctx, webhookEventsKey, payload).Err()
}

// PopEvent blocks up to timeout for the next queued event. Returns nil, nil
// when nothing arrived.
func (s *LiveScanStore) PopEvent(ctx context.Context, timeout time.Duration) ([]byte, error) {
	res, err := s.client.BRPop(ctx, timeout, webhookEventsKey).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(res[1]), nil // res[0] is the key name
}

// ArmPending starts (or joins) a debounce window for key — see
// armPendingScript.
func (s *LiveScanStore) ArmPending(ctx context.Context, key string, payload []byte, fireAt time.Time) error {
	return armPendingScript.Run(ctx, s.client, []string{webhookPendingKey, webhookPendingDataKey},
		key, payload, fireAt.UnixMilli()).Err()
}

// PopDuePending claims and returns the payload of every debounce window
// that has ended by now.
func (s *LiveScanStore) PopDuePending(ctx context.Context, now time.Time) ([][]byte, error) {
	keys, err := s.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: webhookPendingKey, ByScore: true,
		Start: "-inf", Stop: strconv.FormatInt(now.UnixMilli(), 10),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: list due pending: %w", err)
	}
	var out [][]byte
	for _, key := range keys {
		v, err := popPendingScript.Run(ctx, s.client, []string{webhookPendingKey, webhookPendingDataKey}, key).Text()
		if errors.Is(err, redis.Nil) {
			continue // claimed by another worker, or its payload was already gone
		}
		if err != nil {
			return out, fmt.Errorf("queue: pop pending %s: %w", key, err)
		}
		out = append(out, []byte(v))
	}
	return out, nil
}

// IncrWindowCount increments key's counter and returns the new value. The
// counter expires window after its first increment (a fixed window).
func (s *LiveScanStore) IncrWindowCount(ctx context.Context, key string, window time.Duration) (int64, error) {
	full := webhookCountPrefix + key
	n, err := s.client.Incr(ctx, full).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		if err := s.client.Expire(ctx, full, window).Err(); err != nil {
			return n, err
		}
	}
	return n, nil
}
