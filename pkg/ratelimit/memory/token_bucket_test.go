package memory

import (
	"math"
	"testing"
	"time"

	"github.com/haseeb/ratelimiter/pkg/ratelimit"
)

// approxEqual checks if two floats are approximately equal within a tolerance.
func approxEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

// mustAvailable calls Available and panics on error (for tests).
func mustAvailable(tb *TokenBucket) float64 {
	avail, err := tb.Available("")
	if err != nil {
		panic(err)
	}
	return avail
}

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

	if mustAvailable(tb) > 5 {
		t.Errorf("Expected tokens to be capped at capacity 5, got %f", mustAvailable(tb))
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

// TestTokenBucket_AvailableReturnsCorrectCount verifies Available() returns accurate token counts.
func TestTokenBucket_AvailableReturnsCorrectCount(t *testing.T) {
	tb := NewTokenBucket(4, 4) // Capacity 4, 4 tokens/sec

	// Initially full
	if avail := mustAvailable(tb); !approxEqual(avail, 4, 0.01) {
		t.Errorf("Expected 4 tokens initially, got %.2f", avail)
	}

	// Consume 1 token
	tb.Allow("")
	if avail := mustAvailable(tb); !approxEqual(avail, 3, 0.01) {
		t.Errorf("Expected 3 tokens after 1 Allow(), got %.2f", avail)
	}

	// Consume 2 more
	tb.Allow("")
	tb.Allow("")
	if avail := mustAvailable(tb); !approxEqual(avail, 1, 0.01) {
		t.Errorf("Expected 1 token after 3 Allow() calls, got %.2f", avail)
	}

	// Consume last token
	tb.Allow("")
	if avail := mustAvailable(tb); !approxEqual(avail, 0, 0.01) {
		t.Errorf("Expected 0 tokens after 4 Allow() calls, got %.2f", avail)
	}
}

// TestTokenBucket_NaturalRefillDuringPaymentDelay simulates the scenario where
// natural refill accumulates during payment verification/settlement delay.
// This is the scenario: exhaust bucket → get 402 → payment takes ~870ms → refill
func TestTokenBucket_NaturalRefillDuringPaymentDelay(t *testing.T) {
	tb := NewTokenBucket(4, 4) // Capacity 4, 4 tokens/sec

	// Exhaust all tokens (simulating rapid requests)
	for i := 0; i < 4; i++ {
		allowed, _ := tb.Allow("")
		if !allowed {
			t.Fatalf("Request %d should be allowed", i+1)
		}
	}

	// 5th request - rate limited (would return 402 in real scenario)
	allowed, _ := tb.Allow("")
	if allowed {
		t.Fatal("5th request should be rate limited")
	}

	// Simulate payment processing delay (~200ms)
	// During this time, natural refill adds: 0.2s * 4 tokens/sec = 0.8 tokens
	time.Sleep(200 * time.Millisecond)

	// Check tokens accumulated during delay
	availBefore := mustAvailable(tb)
	expectedMin := 0.7 // Allow some timing tolerance
	expectedMax := 1.0
	if availBefore < expectedMin || availBefore > expectedMax {
		t.Errorf("Expected %.1f-%.1f tokens after 200ms delay, got %.2f", expectedMin, expectedMax, availBefore)
	}

	// Now paid refill happens
	tb.Refill("", 4)

	// Total should be: accumulated (~0.8) + paid (4) = ~4.8
	availAfter := mustAvailable(tb)
	expectedMinAfter := 4.7
	expectedMaxAfter := 5.0
	if availAfter < expectedMinAfter || availAfter > expectedMaxAfter {
		t.Errorf("Expected %.1f-%.1f tokens after refill, got %.2f", expectedMinAfter, expectedMaxAfter, availAfter)
	}

	t.Logf("Tokens before refill: %.2f, after refill: %.2f", availBefore, availAfter)
}

// TestTokenBucket_BurstTokensPreserved verifies that burst tokens (above capacity)
// are preserved and not capped by natural refill.
func TestTokenBucket_BurstTokensPreserved(t *testing.T) {
	tb := NewTokenBucket(4, 4) // Capacity 4, 4 tokens/sec

	// Start full (4 tokens), add 4 more via paid refill
	tb.Refill("", 4)

	// Should have 8 tokens (burst)
	avail := mustAvailable(tb)
	if !approxEqual(avail, 8, 0.01) {
		t.Errorf("Expected 8 burst tokens, got %.2f", avail)
	}

	// Wait for natural refill period - burst should NOT increase
	time.Sleep(250 * time.Millisecond)

	// Still should have 8 tokens (no natural refill when above capacity)
	avail = mustAvailable(tb)
	if !approxEqual(avail, 8, 0.01) {
		t.Errorf("Expected 8 tokens preserved (no natural refill above capacity), got %.2f", avail)
	}

	// Consume down to 6
	tb.Allow("")
	tb.Allow("")
	avail = mustAvailable(tb)
	if !approxEqual(avail, 6, 0.01) {
		t.Errorf("Expected 6 tokens after 2 Allow() calls, got %.2f", avail)
	}

	// Wait again - still above capacity, no natural refill
	time.Sleep(250 * time.Millisecond)
	avail = mustAvailable(tb)
	if !approxEqual(avail, 6, 0.01) {
		t.Errorf("Expected 6 tokens preserved (still above capacity), got %.2f", avail)
	}
}

// TestTokenBucket_NaturalRefillResumesAfterBurstConsumed verifies that natural
// refill resumes once burst tokens are consumed below capacity.
func TestTokenBucket_NaturalRefillResumesAfterBurstConsumed(t *testing.T) {
	tb := NewTokenBucket(4, 10) // Capacity 4, 10 tokens/sec for faster test

	// Add burst tokens: 4 (initial) + 4 (paid) = 8
	tb.Refill("", 4)

	// Consume 6 tokens, leaving 2 (below capacity)
	for i := 0; i < 6; i++ {
		tb.Allow("")
	}

	avail := mustAvailable(tb)
	if !approxEqual(avail, 2, 0.01) {
		t.Errorf("Expected 2 tokens after consuming 6, got %.2f", avail)
	}

	// Wait 100ms - should get natural refill: 0.1s * 10 tokens/sec = 1 token
	time.Sleep(110 * time.Millisecond)

	avail = mustAvailable(tb)
	expectedMin := 2.9
	expectedMax := 3.2
	if avail < expectedMin || avail > expectedMax {
		t.Errorf("Expected %.1f-%.1f tokens after natural refill resumed, got %.2f", expectedMin, expectedMax, avail)
	}
}

// TestTokenBucket_ExactTokenCountAfterOperations tests precise token tracking
// through a sequence of operations.
func TestTokenBucket_ExactTokenCountAfterOperations(t *testing.T) {
	tb := NewTokenBucket(4, 4) // Capacity 4, 4 tokens/sec

	// Scenario from the user's log:
	// 1. Start with 4 tokens
	// 2. 4 rapid requests consume all
	// 3. Wait ~210ms (simulating payment delay)
	// 4. Expected: ~0.84 tokens accumulated
	// 5. Refill +4 tokens
	// 6. Expected: ~4.84 tokens total

	if avail := mustAvailable(tb); !approxEqual(avail, 4, 0.01) {
		t.Errorf("Step 1: Expected 4 tokens, got %.2f", avail)
	}

	// Rapid requests
	for i := 0; i < 4; i++ {
		tb.Allow("")
	}

	if avail := mustAvailable(tb); !approxEqual(avail, 0, 0.01) {
		t.Errorf("Step 2: Expected 0 tokens, got %.2f", avail)
	}

	// Simulate payment delay
	time.Sleep(210 * time.Millisecond)

	beforeRefill := mustAvailable(tb)
	t.Logf("Step 3: Tokens after 210ms delay: %.2f (expected ~0.84)", beforeRefill)

	// Paid refill
	tb.Refill("", 4)

	afterRefill := mustAvailable(tb)
	t.Logf("Step 4: Tokens after +4 refill: %.2f (expected ~4.84)", afterRefill)

	// Verify the refill added exactly 4 tokens
	diff := afterRefill - beforeRefill
	if !approxEqual(diff, 4, 0.01) {
		t.Errorf("Refill should add exactly 4 tokens, but added %.2f", diff)
	}
}

// TestTokenBucket_MultipleRefillsStack verifies that multiple paid refills stack.
func TestTokenBucket_MultipleRefillsStack(t *testing.T) {
	tb := NewTokenBucket(4, 1) // Capacity 4, slow refill

	// Initial: 4 tokens
	// Refill 1: +4 = 8
	// Refill 2: +4 = 12
	tb.Refill("", 4)
	tb.Refill("", 4)

	avail := mustAvailable(tb)
	if !approxEqual(avail, 12, 0.01) {
		t.Errorf("Expected 12 tokens after two refills, got %.2f", avail)
	}

	// Should handle 12 requests
	successCount := 0
	for i := 0; i < 15; i++ {
		allowed, _ := tb.Allow("")
		if allowed {
			successCount++
		}
	}

	if successCount != 12 {
		t.Errorf("Expected 12 successful requests, got %d", successCount)
	}
}

// TestTokenBucket_RefillOnEmptyBucket verifies refill works correctly on empty bucket.
func TestTokenBucket_RefillOnEmptyBucket(t *testing.T) {
	tb := NewTokenBucket(4, 4)

	// Exhaust bucket
	for i := 0; i < 4; i++ {
		tb.Allow("")
	}

	if avail := mustAvailable(tb); !approxEqual(avail, 0, 0.01) {
		t.Errorf("Expected 0 tokens, got %.2f", avail)
	}

	// Refill exactly to capacity
	tb.Refill("", 4)

	if avail := mustAvailable(tb); !approxEqual(avail, 4, 0.01) {
		t.Errorf("Expected 4 tokens after refill, got %.2f", avail)
	}

	// All 4 requests should succeed
	for i := 0; i < 4; i++ {
		allowed, _ := tb.Allow("")
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 5th should fail
	allowed, _ := tb.Allow("")
	if allowed {
		t.Error("5th request should be rejected")
	}
}
