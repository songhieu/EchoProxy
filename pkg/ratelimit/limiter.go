// Package ratelimit implements a per-key rate limiter using a Redis fixed-window
// counter. It is shared by proxy-gateway and ingest-api so a single API key sees
// the same budget regardless of which surface produced traffic.
//
// The Lua script makes the increment + TTL set atomic, so concurrent callers
// across multiple replicas never race past the limit by more than 1.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Decision is what Allow returns. Kept tiny so the hot path can branch on bool.
type Decision struct {
	Allowed    bool
	Remaining  int           // approximate; floor of (limit - count)
	RetryAfter time.Duration // populated when Allowed is false
}

// Limiter is goroutine-safe.
type Limiter struct {
	client *redis.Client
	prefix string
}

// New constructs a Limiter. addr is "host:port"; prefix is added to every key
// (e.g. "rl:proxy:" so two services can share Redis without colliding).
func New(addr, prefix string) (*Limiter, error) {
	if addr == "" {
		return nil, errors.New("ratelimit: redis addr required")
	}
	if prefix == "" {
		prefix = "rl:"
	}
	cli := redis.NewClient(&redis.Options{Addr: addr})
	return &Limiter{client: cli, prefix: prefix}, nil
}

// Disabled returns a Limiter that always allows. Use it when Redis is not
// configured so callers don't need to nil-check.
func Disabled() *Limiter { return &Limiter{} }

// Close releases the underlying Redis connection.
func (l *Limiter) Close() error {
	if l.client == nil {
		return nil
	}
	return l.client.Close()
}

// luaScript: atomically INCR key and set TTL on first hit. Returns the new
// counter value.
const luaScript = `
local v = redis.call('INCR', KEYS[1])
if v == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return v
`

var script = redis.NewScript(luaScript)

// Allow consumes one request token for the given (apiKeyID, windowSeconds=1)
// bucket. limitRPS=0 disables the limit (everything allowed).
func (l *Limiter) Allow(ctx context.Context, apiKeyID uint64, limitRPS int) Decision {
	if l.client == nil || limitRPS <= 0 {
		return Decision{Allowed: true, Remaining: -1}
	}
	now := time.Now().Unix() // 1-second window
	key := fmt.Sprintf("%s%d:%d", l.prefix, apiKeyID, now)
	res, err := script.Run(ctx, l.client, []string{key}, 1100).Result() // PEXPIRE in ms
	if err != nil {
		// Fail-open: a Redis blip should never break user traffic.
		return Decision{Allowed: true, Remaining: -1}
	}
	count, ok := toInt(res)
	if !ok {
		return Decision{Allowed: true, Remaining: -1}
	}
	if count > int64(limitRPS) {
		return Decision{
			Allowed:    false,
			Remaining:  0,
			RetryAfter: time.Second, // current window expires within 1s
		}
	}
	return Decision{Allowed: true, Remaining: limitRPS - int(count)}
}

func toInt(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		return n, err == nil
	}
	return 0, false
}
