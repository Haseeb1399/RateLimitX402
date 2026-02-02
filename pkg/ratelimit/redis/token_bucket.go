package redis

import (
	"context"
	"log"
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

		-- Natural refill based on elapsed time
		-- Only add tokens if below capacity (preserves overflow from paid refills)
		local elapsed = now - last_refill
		if tokens < capacity then
			tokens = tokens + elapsed * refill_rate
			if tokens > capacity then
				tokens = capacity
			end
		end

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
	// Returns both old and new token counts for logging
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
		return {current, new_tokens}
	`)

	result, err := refillScript.Run(
		context.Background(),
		r.client,
		[]string{fullKey},
		tokens,
		r.capacity,
		r.refillRate,
	).Int64Slice()

	if err != nil {
		return err
	}

	oldTokens := float64(result[0])
	newTokens := float64(result[1])
	log.Printf("[REFILL] key=%s before=%.2f added=%.2f after=%.2f", key, oldTokens, tokens, newTokens)

	return nil
}

// Available returns the current number of tokens for the given key.
// This is useful for debugging and testing.
func (r *TokenBucket) Available(key string) (float64, error) {
	fullKey := r.keyPrefix + key

	// Lua script to get current tokens after natural refill
	availableScript := redis.NewScript(`
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])

		local data = redis.call("HMGET", key, "tokens", "last_refill")
		local tokens = tonumber(data[1])
		local last_refill = tonumber(data[2])

		-- If key doesn't exist, return capacity
		if tokens == nil then
			return capacity
		end

		-- Calculate natural refill (but don't modify)
		-- Only add tokens if below capacity (preserves overflow from paid refills)
		if last_refill ~= nil and tokens < capacity then
			local elapsed = now - last_refill
			tokens = tokens + elapsed * refill_rate
			if tokens > capacity then
				tokens = capacity
			end
		end

		return tokens
	`)

	now := float64(time.Now().UnixMicro()) / 1e6

	result, err := availableScript.Run(
		context.Background(),
		r.client,
		[]string{fullKey},
		r.capacity,
		r.refillRate,
		now,
	).Float64()

	if err != nil {
		return 0, err
	}

	return result, nil
}

// Ensure TokenBucket implements Limiter interface.
var _ ratelimit.Limiter = (*TokenBucket)(nil)
