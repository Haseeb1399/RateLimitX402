package redis

import (
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/haseeb/ratelimiter/pkg/ratelimit"
	goredis "github.com/redis/go-redis/v9"
)

// setupMiniredis creates a miniredis server and returns a redis client and cleanup function.
func setupMiniredis(t *testing.T) (*goredis.Client, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	return client, func() {
		client.Close()
		mr.Close()
	}
}

func TestTokenBucket_Allow(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   5,
		RefillRate: 1,
	})

	// Consume all 5 tokens
	for i := 0; i < 5; i++ {
		allowed, err := rtb.Allow("test-key")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 6th should be rejected
	allowed, err := rtb.Allow("test-key")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if allowed {
		t.Error("Expected 6th request to be rejected")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   5,
		RefillRate: 10, // 10 tokens per second
	})

	// Empty the bucket
	for i := 0; i < 5; i++ {
		rtb.Allow("refill-test")
	}

	allowed, _ := rtb.Allow("refill-test")
	if allowed {
		t.Error("Should be empty now")
	}

	// Wait 110ms, should get 1 token (10/sec * 0.11sec ≈ 1.1)
	time.Sleep(110 * time.Millisecond)

	allowed, err := rtb.Allow("refill-test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Expected token to be refilled after wait")
	}

	// Should be empty again after consuming the refilled token
	allowed, _ = rtb.Allow("refill-test")
	if allowed {
		t.Error("Should only have refilled ~1 token")
	}
}

func TestTokenBucket_MaxCapacity(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   5,
		RefillRate: 100, // Fast refill
	})

	// Consume all 5 tokens first
	for i := 0; i < 5; i++ {
		allowed, _ := rtb.Allow("cap-test")
		if !allowed {
			t.Errorf("Initial request %d should be allowed", i+1)
		}
	}

	// Wait for bucket to refill (should refill to capacity, not beyond)
	time.Sleep(100 * time.Millisecond)

	// Should still only be able to consume 5 tokens (capacity)
	successCount := 0
	for i := 0; i < 10; i++ {
		allowed, _ := rtb.Allow("cap-test")
		if allowed {
			successCount++
		}
	}

	if successCount != 5 {
		t.Errorf("Expected 5 successful requests (capacity), got %d", successCount)
	}
}

func TestTokenBucket_DifferentKeys(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   2,
		RefillRate: 0.1, // Very slow refill
	})

	// User A consumes their tokens
	rtb.Allow("user-a")
	rtb.Allow("user-a")
	allowedA, _ := rtb.Allow("user-a")
	if allowedA {
		t.Error("User A should be rate limited")
	}

	// User B should still have their tokens
	allowedB, err := rtb.Allow("user-b")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !allowedB {
		t.Error("User B should not be rate limited")
	}
}

func TestTokenBucket_CustomKeyPrefix(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   5,
		RefillRate: 1,
		KeyPrefix:  "custom:",
	})

	allowed, err := rtb.Allow("test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Expected request to be allowed")
	}
}

func TestTokenBucket_DefaultKeyPrefix(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   5,
		RefillRate: 1,
		// No KeyPrefix specified, should default to "ratelimit:"
	})

	if rtb.KeyPrefix() != "ratelimit:" {
		t.Errorf("Expected default key prefix 'ratelimit:', got '%s'", rtb.KeyPrefix())
	}
}

func TestTokenBucket_ThreadSafety(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   1000,
		RefillRate: 100,
	})

	const goroutines = 50
	const reqsPerG = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < reqsPerG; j++ {
				_, err := rtb.Allow("concurrent-test")
				if err != nil {
					t.Errorf("Unexpected error during concurrent access: %v", err)
				}
			}
		}()
	}

	wg.Wait()
	// Just checking it doesn't crash, deadlock, or return errors
}

// TestLimiterInterface verifies that TokenBucket implements the Limiter interface.
func TestLimiterInterface(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	var _ ratelimit.Limiter = NewTokenBucket(Config{
		Client:     client,
		Capacity:   10,
		RefillRate: 1,
	})
}

func TestTokenBucket_ZeroInitialState(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   3,
		RefillRate: 1,
	})

	// First request to a new key should start with full capacity
	allowed, _ := rtb.Allow("new-key")
	if !allowed {
		t.Error("First request should be allowed (bucket starts full)")
	}
}

func TestTokenBucket_BurstThenThrottle(t *testing.T) {
	client, cleanup := setupMiniredis(t)
	defer cleanup()

	rtb := NewTokenBucket(Config{
		Client:     client,
		Capacity:   3,
		RefillRate: 1, // 1 token per second
	})

	// Burst: consume all 3 tokens quickly
	for i := 0; i < 3; i++ {
		allowed, _ := rtb.Allow("burst-key")
		if !allowed {
			t.Errorf("Burst request %d should be allowed", i+1)
		}
	}

	// Should be throttled now
	allowed, _ := rtb.Allow("burst-key")
	if allowed {
		t.Error("Should be throttled after burst")
	}

	// Wait for 1 token to refill
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	allowed, _ = rtb.Allow("burst-key")
	if !allowed {
		t.Error("Should be allowed after refill")
	}
}
