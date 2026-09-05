// Package ratelimit bounds how often something may happen.
//
// The counters live in Redis so that every instance of the hub enforces one
// limit, and expire on their own so there is nothing to clean up. The
// algorithm is a token bucket: a caller may burst up to the bucket's size,
// then continue at the refill rate, which suits chat — a person pastes three
// messages in a row, a backend answers a group in one go — without letting
// anyone run flat out.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Policy is a bucket's shape: Burst tokens to start, refilled at Rate per
// second.
type Policy struct {
	Rate  float64
	Burst int
}

// Limiter decides whether an action may happen now.
type Limiter interface {
	// Allow spends one token from the bucket named by key. It returns zero
	// when the action may proceed, or how long until it could.
	Allow(ctx context.Context, key string, p Policy) (time.Duration, error)
}

// Unlimited allows everything. For tests of things that are not the limit,
// and for callers that have not been given a real one.
type Unlimited struct{}

// Allow always permits.
func (Unlimited) Allow(context.Context, string, Policy) (time.Duration, error) { return 0, nil }

// Redis is a Limiter backed by a Redis server.
type Redis struct {
	client *redis.Client
	prefix string
}

// NewRedis builds a limiter. Every key is prefixed, so several hubs, or
// several tests, can share one server without sharing buckets.
func NewRedis(client *redis.Client, prefix string) *Redis {
	return &Redis{client: client, prefix: prefix}
}

// tokenBucket runs atomically on the server: read the bucket, refill it for
// the time elapsed, spend if possible, write it back. Doing this in one
// script rather than several commands is what makes two instances of the
// hub unable to both grant the last token.
var tokenBucket = redis.NewScript(`
local rate  = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now   = tonumber(ARGV[3])
local cost  = tonumber(ARGV[4])

local state  = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(state[1])
local ts     = tonumber(state[2])
if tokens == nil then
  tokens = burst
  ts = now
end

local elapsed = math.max(0, now - ts) / 1000
tokens = math.min(burst, tokens + elapsed * rate)

local allowed = 0
local retry_ms = 0
if tokens >= cost then
  tokens = tokens - cost
  allowed = 1
else
  retry_ms = math.ceil((cost - tokens) / rate * 1000)
end

redis.call('HSET', KEYS[1], 'tokens', tokens, 'ts', now)
redis.call('PEXPIRE', KEYS[1], math.ceil(burst / rate * 1000) + 1000)
return {allowed, retry_ms}
`)

// Allow implements Limiter.
func (r *Redis) Allow(ctx context.Context, key string, p Policy) (time.Duration, error) {
	res, err := tokenBucket.Run(ctx, r.client, []string{r.prefix + key},
		p.Rate, p.Burst, time.Now().UnixMilli(), 1).Int64Slice()
	if err != nil {
		return 0, fmt.Errorf("ratelimit %s: %w", key, err)
	}
	if len(res) != 2 {
		return 0, fmt.Errorf("ratelimit %s: unexpected reply %v", key, res)
	}
	if res[0] == 1 {
		return 0, nil
	}
	return time.Duration(res[1]) * time.Millisecond, nil
}
