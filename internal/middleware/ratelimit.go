package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/haseeb/ratelimiter/pkg/ratelimit"
)

// RateLimitMiddleware wraps an http.Handler and applies rate limiting.
// Returns 429 Too Many Requests when the limit is exceeded.
func RateLimitMiddleware(limiter ratelimit.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use client IP as the rate limit key
		key := r.RemoteAddr

		allowed, err := limiter.Allow(key)
		if err != nil {
			http.Error(w, "Rate limiter error", http.StatusInternalServerError)
			return
		}

		if !allowed {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimitHandler wraps an http.HandlerFunc for convenience.
func RateLimitHandler(limiter ratelimit.Limiter, handler http.HandlerFunc) http.Handler {
	return RateLimitMiddleware(limiter, handler)
}

// GinRateLimitMiddleware creates a Gin middleware for rate limiting.
// When rate limited, it aborts with 402 status.
func GinRateLimitMiddleware(limiter ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()

		allowed, err := limiter.Allow(key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate limiter error"})
			c.Abort()
			return
		}

		if !allowed {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"error":   "Rate limit exceeded",
				"message": "Pay to refill your token bucket",
			})
			return
		}

		c.Next()
	}
}
