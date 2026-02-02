package redis

import (
	"context"
	"time"

	"github.com/haseeb/ratelimiter/pkg/ratelimit"
	"github.com/redis/go-redis/v9"
)

// TokenBucket implements a distributed token bucket using Redis.
type TokenBucket struct {
	client     *redis.Client
	capacity   float64
	refillRate float64 // tokens per second
	keyPrefix  string
	script     *redis.Script
}

// Config holds configuration for the Redis token bucket.
type Config struct {
	Client     *redis.Client
	Capacity   float64
	RefillRate float64
	KeyPrefix  string // Optional prefix for Redis keys (default: "ratelimit:")
}

// NewTokenBucket creates a new Redis-backed token bucket.
func NewTokenBucket(cfg Config) *TokenBucket {
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "ratelimit:"
	}

	// Lua script for atomic refill + consume
	script := redis.NewScript(`
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])

		local data = redis.call("HMGET", key, "tokens", "last_refill")
		local tokens = tonumber(data[1]) or capacity
		local last_refill = tonumber(data[2]) or now

		-- Refill tokens based on elapsed time
		local elapsed = now - last_refill
		tokens = math.min(capacity, tokens + elapsed * refill_rate)

		-- Try to consume one token
		if tokens >= 1 then
			tokens = tokens - 1
			redis.call("HMSET", key, "tokens", tokens, "last_refill", now)
			redis.call("EXPIRE", key, math.ceil(capacity / refill_rate) + 1)
			return 1
		else
			redis.call("HMSET", key, "tokens", tokens, "last_refill", now)
			redis.call("EXPIRE", key, math.ceil(capacity / refill_rate) + 1)
			return 0
		end
	`)

	return &TokenBucket{
		client:     cfg.Client,
		capacity:   cfg.Capacity,
		refillRate: cfg.RefillRate,
		keyPrefix:  prefix,
		script:     script,
	}
}

// Allow checks if a request for the given key should be allowed.
func (r *TokenBucket) Allow(key string) (bool, error) {
	fullKey := r.keyPrefix + key
	now := float64(time.Now().UnixMicro()) / 1e6 // seconds with microsecond precision

	result, err := r.script.Run(
		context.Background(),
		r.client,
		[]string{fullKey},
		r.capacity,
		r.refillRate,
		now,
	).Int()

	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// KeyPrefix returns the current key prefix (useful for testing).
func (r *TokenBucket) KeyPrefix() string {
	return r.keyPrefix
}

// Refill adds tokens to the bucket for the given key without capping at capacity.
// This allows paid tokens to exceed the normal limit ("burst" tokens).
func (r *TokenBucket) Refill(key string, tokens float64) error {
	fullKey := r.keyPrefix + key

	// Lua script for atomic refill without capacity cap
	refillScript := redis.NewScript(`
		local key = KEYS[1]
		local tokens_to_add = tonumber(ARGV[1])
		local capacity = tonumber(ARGV[2])
		local refill_rate = tonumber(ARGV[3])

		local current = tonumber(redis.call("HGET", key, "tokens")) or capacity
		local new_tokens = current + tokens_to_add
		-- No cap - allow overflow beyond capacity for paid tokens
		
		redis.call("HSET", key, "tokens", new_tokens)
		redis.call("EXPIRE", key, math.ceil(capacity / refill_rate) + 1)
		return new_tokens
	`)

	_, err := refillScript.Run(
		context.Background(),
		r.client,
		[]string{fullKey},
		tokens,
		r.capacity,
		r.refillRate,
	).Result()

	return err
}

// Ensure TokenBucket implements Limiter interface.
var _ ratelimit.Limiter = (*TokenBucket)(nil)
