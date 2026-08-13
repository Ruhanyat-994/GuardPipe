package advisory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is the real Cache implementation, built on adapters/queue's
// connection (documentation/06-database-design.md §11's `gp:cache:osv:*`
// entry). A nil-valued JSON payload ("null", written when a dependency has
// zero advisories) round-trips as an empty, non-nil slice — a cache hit
// with no advisories must stay distinguishable from a cache miss.
type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

var _ Cache = (*RedisCache)(nil)

func (c *RedisCache) Get(ctx context.Context, key string) ([]Advisory, bool, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("advisory: redis get: %w", err)
	}

	var advisories []Advisory
	if err := json.Unmarshal(raw, &advisories); err != nil {
		return nil, false, fmt.Errorf("advisory: decode cached advisories: %w", err)
	}
	if advisories == nil {
		advisories = []Advisory{}
	}
	return advisories, true, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, advisories []Advisory, ttl time.Duration) error {
	payload, err := json.Marshal(advisories)
	if err != nil {
		return fmt.Errorf("advisory: encode advisories: %w", err)
	}
	if err := c.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("advisory: redis set: %w", err)
	}
	return nil
}
