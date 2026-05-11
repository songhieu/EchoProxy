package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is a tiny JSON-serialized cache used by hot dashboard queries.
// Keys are namespaced; misses are not fatal.
type Cache struct {
	c   *redis.Client
	ttl time.Duration
}

func New(addr string, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Cache{
		c:   redis.NewClient(&redis.Options{Addr: addr}),
		ttl: ttl,
	}
}

func (c *Cache) Get(ctx context.Context, key string, dst any) bool {
	b, err := c.c.Get(ctx, "stats:"+key).Bytes()
	if err != nil {
		return false
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return false
	}
	return true
}

func (c *Cache) Set(ctx context.Context, key string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = c.c.Set(ctx, "stats:"+key, b, c.ttl).Err()
}
