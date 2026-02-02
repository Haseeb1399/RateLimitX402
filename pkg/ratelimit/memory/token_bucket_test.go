package memory

import (
	"testing"
	"time"

	"github.com/haseeb/ratelimiter/pkg/ratelimit"
)

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(5, 1)

	// Consume all 5
	for i := 0; i < 5; i++ {
		allowed, err := tb.Allow("")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 6th should be rejected
	allowed, _ := tb.Allow("")
	if allowed {
		t.Error("Expected 6th request to be rejected")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := NewTokenBucket(5, 10) // 10 tokens per second

	// Empty the bucket
	for i := 0; i < 5; i++ {
		tb.Allow("")
	}

	allowed, _ := tb.Allow("")
	if allowed {
		t.Error("Should be empty now")
	}

	// Wait 100ms, should get 1 token (10/sec * 0.1sec = 1)
	time.Sleep(110 * time.Millisecond)

	allowed, _ = tb.Allow("")
	if !allowed {
		t.Error("Expected token to be refilled after wait")
	}

	allowed, _ = tb.Allow("")
	if allowed {
		t.Error("Should only have refilled 1 token")
	}
}

func TestTokenBucket_MaxCapacity(t *testing.T) {
	tb := NewTokenBucket(5, 100) // Fast refill

	time.Sleep(100 * time.Millisecond)

	if tb.Available() > 5 {
		t.Errorf("Expected tokens to be capped at capacity 5, got %f", tb.Available())
	}
}

func TestTokenBucket_ThreadSafety(t *testing.T) {
	tb := NewTokenBucket(1000, 100)
	const goroutines = 100
	const reqsPerG = 10

	done := make(chan bool)
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < reqsPerG; j++ {
				tb.Allow("")
			}
			done <- true
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Just checking it doesn't crash or race (run with -race)
}

// TestLimiterInterface verifies that TokenBucket implements the Limiter interface.
func TestLimiterInterface(t *testing.T) {
	var _ ratelimit.Limiter = NewTokenBucket(10, 1)
}

func TestTokenBucket_RefillMethod(t *testing.T) {
	tb := NewTokenBucket(5, 1) // Capacity 5

	// Empty the bucket
	for i := 0; i < 5; i++ {
		tb.Allow("")
	}

	// Bucket should be empty
	allowed, _ := tb.Allow("")
	if allowed {
		t.Error("Bucket should be empty")
	}

	// Refill with 3 tokens
	err := tb.Refill("", 3)
	if err != nil {
		t.Fatalf("Refill error: %v", err)
	}

	// Should now have 3 tokens
	for i := 0; i < 3; i++ {
		allowed, _ = tb.Allow("")
		if !allowed {
			t.Errorf("Request %d should be allowed after refill", i+1)
		}
	}

	// 4th should be rejected
	allowed, _ = tb.Allow("")
	if allowed {
		t.Error("4th request should be rejected")
	}
}

func TestTokenBucket_RefillExceedsCapacity(t *testing.T) {
	tb := NewTokenBucket(5, 1) // Capacity 5

	// Start with full bucket (5 tokens)
	// Refill with 5 more - should allow overflow
	err := tb.Refill("", 5)
	if err != nil {
		t.Fatalf("Refill error: %v", err)
	}

	// Should now have 10 tokens (5 original + 5 refill)
	successCount := 0
	for i := 0; i < 12; i++ {
		allowed, _ := tb.Allow("")
		if allowed {
			successCount++
		}
	}

	if successCount != 10 {
		t.Errorf("Expected 10 successful requests (overflow), got %d", successCount)
	}
}

func TestTokenBucket_PartialConsumeRefillAndNaturalRegen(t *testing.T) {
	tb := NewTokenBucket(5, 10) // Capacity 5, 10 tokens/sec refill

	// Start with 5 tokens, consume 3 (leaving 2)
	for i := 0; i < 3; i++ {
		allowed, _ := tb.Allow("")
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Buy/Refill 5 more tokens (2 + 5 = 7, exceeds capacity)
	err := tb.Refill("", 5)
	if err != nil {
		t.Fatalf("Refill error: %v", err)
	}

	// Should have 7 tokens now
	// Let natural refill happen (100ms = 1 token, but we're above capacity so no natural regen)
	time.Sleep(110 * time.Millisecond)

	// Still should have only 7 tokens (overflow preserved, no natural regen above capacity)
	successCount := 0
	for i := 0; i < 10; i++ {
		allowed, _ := tb.Allow("")
		if allowed {
			successCount++
		}
	}

	if successCount != 7 {
		t.Errorf("Expected 7 successful requests (2 remaining + 5 refill, no natural regen above capacity), got %d", successCount)
	}
}
