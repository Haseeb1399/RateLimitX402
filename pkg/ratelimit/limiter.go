package ratelimit

// Limiter is the interface for rate limiters.
// Implementations can be in-memory, Redis-backed, or any other storage.
type Limiter interface {
	// Allow checks if a request identified by key should be allowed.
	// Returns true if allowed, false if rate limited.
	Allow(key string) (bool, error)

	// Refill adds tokens to the bucket for the given key.
	// Used when a user pays to refill their rate limit quota.
	// Returns error if the refill fails.
	Refill(key string, tokens float64) error
}
