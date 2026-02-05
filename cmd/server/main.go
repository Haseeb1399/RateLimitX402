package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	evm "github.com/coinbase/x402/go/mechanisms/evm/exact/server"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/haseeb/ratelimiter/internal/config"
	"github.com/haseeb/ratelimiter/internal/handlers"
	"github.com/haseeb/ratelimiter/pkg/ratelimit"
	"github.com/haseeb/ratelimiter/pkg/ratelimit/memory"
	ratelimitredis "github.com/haseeb/ratelimiter/pkg/ratelimit/redis"
	"github.com/haseeb/ratelimiter/pkg/trust"
)

func main() {
	// Load configuration
	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create rate limiter with config values
	var limiter ratelimit.Limiter
	if cfg.RateLimit.Strategy == "redis" {
		rdb := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		limiter = ratelimitredis.NewTokenBucket(ratelimitredis.Config{
			Client:     rdb,
			Capacity:   cfg.RateLimit.Capacity,
			RefillRate: cfg.RateLimit.RefillRate,
		})
		fmt.Printf("Using Redis rate limiter at %s\n", cfg.Redis.Addr)
	} else {
		limiter = memory.NewTokenBucket(cfg.RateLimit.Capacity, cfg.RateLimit.RefillRate)
		fmt.Printf("Using in-memory rate limiter\n")
	}

	// Create Gin router
	r := gin.Default()

	// Token monitoring endpoint (for testing/debugging) - registered BEFORE rate limiting
	r.GET("/tokens", func(c *gin.Context) {
		key := c.ClientIP()
		tokens, err := limiter.Available(key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"client":   key,
			"tokens":   tokens,
			"capacity": cfg.RateLimit.Capacity,
		})
	})

	if cfg.Payment.Enabled {
		// Configure X402 payment options for when rate limit is exceeded
		paymentOptions := x402http.PaymentOptions{
			{
				Scheme:  "exact",
				Price:   cfg.Payment.PricePerCapacity, // "$0.001"
				Network: "eip155:84532",               // Base Sepolia
				PayTo:   cfg.Payment.WalletAddress,
			},
		}

		// Create facilitator client
		facilitatorConfig := &x402http.FacilitatorConfig{
			URL: cfg.Payment.FacilitatorURL,
			HTTPClient: &http.Client{
				Timeout: 10 * time.Second,
				Transport: &loggingRoundTripper{
					proxied: http.DefaultTransport,
				},
			},
		}
		facilitator := x402http.NewHTTPFacilitatorClient(facilitatorConfig)

		// Create X402 resource server for payment processing
		server := x402.Newx402ResourceServer(
			x402.WithFacilitatorClient(facilitator),
		).Register("eip155:84532", evm.NewExactEvmScheme())

		// Create the HTTP server wrapper
		routes := x402http.RoutesConfig{
			"GET /cpu": {
				Accepts:     paymentOptions,
				Description: "CPU utilization endpoint - pay to refill rate limit",
				MimeType:    "application/json",
			},
		}
		httpServer := x402http.Wrappedx402HTTPResourceServer(routes, server)

		// Initialize - sync with facilitator to populate internal maps
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := httpServer.Initialize(ctx); err != nil {
			log.Printf("Warning: failed to initialize x402 server: %v", err)
		}
		cancel()

		// Create trust tracker for optimistic settlement
		var trustTracker *trust.Tracker
		var settlementQueue *SettlementQueue
		if cfg.Payment.Optimistic.Enabled {
			trustTracker = trust.New(trust.Config{
				Threshold: cfg.Payment.Optimistic.TrustThreshold,
				Window:    cfg.Payment.Optimistic.TrustWindow,
			})
			// Create settlement queue for sequential background processing
			settlementQueue = NewSettlementQueue(httpServer, trustTracker, 100)
			log.Printf("Optimistic settlement enabled (threshold: %d in %s, queued settlements)",
				cfg.Payment.Optimistic.TrustThreshold,
				cfg.Payment.Optimistic.TrustWindow)
		}

		// Apply custom rate limit + payment middleware
		r.Use(hybridRateLimitPaymentMiddleware(limiter, httpServer, cfg.RateLimit.Capacity, trustTracker, settlementQueue))

		fmt.Printf("Payment enabled: %s %s on %s\n",
			cfg.Payment.PricePerCapacity, cfg.Payment.Currency, cfg.Payment.Network)
	} else {
		// Simple rate limiting without payment
		r.Use(simpleRateLimitMiddleware(limiter))
	}

	// Register handlers
	r.GET("/cpu", handlers.GinCPUHandler())
	r.GET("/dashboard", handlers.GinDashboardHandler())

	// Start server
	fmt.Printf("Server starting on %s (rate limit: %.0f tokens, %.1f/sec refill)\n",
		cfg.Server.Port, cfg.RateLimit.Capacity, cfg.RateLimit.RefillRate)
	r.Run(cfg.Server.Port)
}

// simpleRateLimitMiddleware is a basic rate limiter that returns 429 when exceeded.
func simpleRateLimitMiddleware(limiter ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		allowed, err := limiter.Allow(key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate limiter error"})
			c.Abort()
			return
		}
		if !allowed {
			c.Header("Retry-After", "1")
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// hybridRateLimitPaymentMiddleware combines rate limiting with X402 payment.
// - If tokens available: serve request
// - If rate limited AND payment provided: verify, settle, refill, serve
// - If rate limited AND no payment: return 402 with payment requirements
// - If trusted client: optimistically refill and settle in background queue
func hybridRateLimitPaymentMiddleware(limiter ratelimit.Limiter, httpServer *x402http.HTTPServer, capacity float64, trustTracker *trust.Tracker, settlementQueue *SettlementQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()

		allowed, err := limiter.Allow(key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate limiter error"})
			c.Abort()
			return
		}

		if allowed {
			// Tokens available, proceed
			c.Next()
			return
		}

		// Rate limited - check for payment header (V2: PAYMENT-SIGNATURE, V1: X-PAYMENT)
		adapter := NewGinAdapter(c)
		paymentHeader := adapter.GetHeader("PAYMENT-SIGNATURE") // V2
		if paymentHeader == "" {
			paymentHeader = adapter.GetHeader("X-PAYMENT") // V1 fallback
		}

		reqCtx := x402http.HTTPRequestContext{
			Adapter:       adapter,
			Path:          c.Request.URL.Path,
			Method:        c.Request.Method,
			PaymentHeader: paymentHeader, // Important: populate this for payment verification
		}

		if paymentHeader == "" {
			// No payment - generate 402 response
			result := httpServer.ProcessHTTPRequest(c.Request.Context(), reqCtx, nil)
			if result.Response != nil {
				for k, v := range result.Response.Headers {
					c.Header(k, v)
				}
				c.JSON(result.Response.Status, result.Response.Body)
			} else {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error":   "Payment Required",
					"message": "Rate limit exceeded. Pay to refill your quota.",
				})
			}
			c.Abort()
			return
		}

		// Payment present - process it (verification happens in ProcessHTTPRequest)
		paymentStart := time.Now()
		result := httpServer.ProcessHTTPRequest(c.Request.Context(), reqCtx, nil)
		verificationLatency := time.Since(paymentStart)

		if result.Type == x402http.ResultPaymentVerified {
			// Extract wallet address from payment for trust tracking
			walletAddr := extractWalletAddress(paymentHeader)

			// Check if client is trusted for optimistic settlement
			if trustTracker != nil && settlementQueue != nil && trustTracker.IsTrusted(walletAddr) {
				// OPTIMISTIC: Refill immediately, settle via queue
				if err := limiter.Refill(key, capacity); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Refill error"})
					c.Abort()
					return
				}

				log.Printf("[OPTIMISTIC] Trusted wallet %s, queueing settlement (verify: %v)",
					truncateWallet(walletAddr), verificationLatency)

				// Enqueue settlement for sequential processing
				settlementQueue.Enqueue(SettlementJob{
					PaymentPayload:      *result.PaymentPayload,
					PaymentRequirements: *result.PaymentRequirements,
					WalletAddr:          walletAddr,
				})

				// Allow the request through immediately
				c.Next()
				return
			}

			// SYNCHRONOUS: Not trusted, settle before responding
			settlementStart := time.Now()
			settleResult := httpServer.ProcessSettlement(
				c.Request.Context(),
				*result.PaymentPayload,
				*result.PaymentRequirements,
			)
			settlementLatency := time.Since(settlementStart)

			if settleResult.Success {
				// Refill the bucket
				refillStart := time.Now()
				if err := limiter.Refill(key, capacity); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Refill error"})
					c.Abort()
					return
				}
				refillLatency := time.Since(refillStart)

				// Record success for trust building
				if trustTracker != nil {
					trustTracker.RecordSuccess(walletAddr)
					log.Printf("[PAYMENT] Settled TX: %s in %v (Verify: %v, Settle: %v, Refill: %v) [trust: %d/%d]",
						settleResult.Transaction, time.Since(paymentStart), verificationLatency, settlementLatency, refillLatency,
						trustTracker.RecentPayments(walletAddr), 3) // 3 is threshold, could make configurable
				} else {
					log.Printf("[PAYMENT] Settled TX: %s in %v (Verify: %v, Settle: %v, Refill: %v)",
						settleResult.Transaction, time.Since(paymentStart), verificationLatency, settlementLatency, refillLatency)
				}

				// Allow the request through
				c.Next()
				return
			}

			// Settlement failed
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":  "Settlement failed",
				"reason": settleResult.ErrorReason,
			})
			c.Abort()
			return
		}

		// Payment verification failed
		if result.Response != nil {
			for k, v := range result.Response.Headers {
				c.Header(k, v)
			}
			c.JSON(result.Response.Status, result.Response.Body)
		} else {
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":   "Payment Required",
				"message": "Invalid payment or rate limit exceeded.",
			})
		}
		c.Abort()
	}
}

// extractWalletAddress extracts the sender wallet address from the payment header.
// The payment header is a base64-encoded JSON with a "payload" containing "authorization.from".
func extractWalletAddress(paymentHeader string) string {
	if paymentHeader == "" {
		return ""
	}

	// Try to decode the base64 payment header
	decoded, err := base64.StdEncoding.DecodeString(paymentHeader)
	if err != nil {
		// Try URL-safe base64
		decoded, err = base64.URLEncoding.DecodeString(paymentHeader)
		if err != nil {
			return ""
		}
	}

	// Parse as JSON to extract the wallet address
	var payment struct {
		Payload struct {
			Authorization struct {
				From string `json:"from"`
			} `json:"authorization"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(decoded, &payment); err != nil {
		return ""
	}

	return strings.ToLower(payment.Payload.Authorization.From)
}

// truncateWallet returns a truncated wallet address for logging.
func truncateWallet(wallet string) string {
	if len(wallet) <= 10 {
		return wallet
	}
	return wallet[:6] + "..." + wallet[len(wallet)-4:]
}

// GinAdapter implements x402http.HTTPAdapter for Gin
type GinAdapter struct {
	ctx *gin.Context
}

func NewGinAdapter(ctx *gin.Context) *GinAdapter {
	return &GinAdapter{ctx: ctx}
}

func (a *GinAdapter) GetHeader(name string) string {
	return a.ctx.GetHeader(name)
}

func (a *GinAdapter) GetMethod() string {
	return a.ctx.Request.Method
}

func (a *GinAdapter) GetPath() string {
	return a.ctx.Request.URL.Path
}

func (a *GinAdapter) GetURL() string {
	scheme := "http"
	if a.ctx.Request.TLS != nil {
		scheme = "https"
	}
	host := a.ctx.Request.Host
	if host == "" {
		host = a.ctx.GetHeader("Host")
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, a.ctx.Request.URL.Path)
}

func (a *GinAdapter) GetAcceptHeader() string {
	return a.ctx.GetHeader("Accept")
}

func (a *GinAdapter) GetUserAgent() string {
	return a.ctx.GetHeader("User-Agent")
}

// loggingRoundTripper logs the duration of HTTP requests
type loggingRoundTripper struct {
	proxied http.RoundTripper
}

func (lrt *loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := lrt.proxied.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		log.Printf("[FACILITATOR] Request to %s failed in %v: %v", req.URL.String(), duration, err)
	} else {
		log.Printf("[FACILITATOR] Request to %s [%d] took %v", req.URL.String(), resp.StatusCode, duration)
	}
	return resp, err
}
