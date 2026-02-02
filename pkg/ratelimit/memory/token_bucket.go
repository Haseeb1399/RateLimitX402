package memory

import (
	"sync"
	"time"

	"github.com/haseeb/ratelimiter/pkg/ratelimit"
)

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	capacity       float64
	refillRate     float64 // tokens per second
	tokens         float64
	lastRefillTime time.Time
	mu             sync.Mutex
}

// NewTokenBucket creates a new TokenBucket with the given capacity and refill rate.
func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:       capacity,
		refillRate:     refillRate,
		tokens:         capacity, // Start full
		lastRefillTime: time.Now(),
	}
}

// refill calculates how many tokens should be added since the last refill.
// Only caps at capacity if tokens were below capacity before adding.
// This preserves "overflow" tokens from paid refills.
func (tb *TokenBucket) refill() {
	now := time.Now()
	duration := now.Sub(tb.lastRefillTime)
	tokensToAdd := duration.Seconds() * tb.refillRate

	// Only add tokens if below capacity (natural regeneration)
	// If already above capacity (from paid refill), don't cap
	if tb.tokens < tb.capacity {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
	}
	tb.lastRefillTime = now
}

// Allow checks if a token is available and consumes it if so.
// The key parameter is ignored for in-memory implementation but required for Limiter interface.
func (tb *TokenBucket) Allow(key string) (bool, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true, nil
	}

	return false, nil
}

// Available returns the current number of tokens (after a refill).
func (tb *TokenBucket) Available() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// Refill adds tokens to the bucket without capping at capacity.
// This allows paid tokens to exceed the normal limit ("burst" tokens).
// The key parameter is ignored for in-memory implementation.
func (tb *TokenBucket) Refill(key string, tokens float64) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.tokens += tokens
	// No cap - allow overflow beyond capacity for paid tokens
	return nil
}

// Ensure TokenBucket implements Limiter interface.
var _ ratelimit.Limiter = (*TokenBucket)(nil)
