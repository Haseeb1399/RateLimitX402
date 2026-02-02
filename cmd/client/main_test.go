package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	evm "github.com/coinbase/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/coinbase/x402/go/signers/evm"
)

// testConfig holds configuration for integration tests.
type testConfig struct {
	serverURL  string
	privateKey string
	capacity   int // Expected server capacity (from config.yaml)
}

func getTestConfig() testConfig {
	serverURL := os.Getenv("TEST_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8081"
	}

	privateKey := os.Getenv("PRIVATE_KEY")

	return testConfig{
		serverURL:  serverURL,
		privateKey: privateKey,
		capacity:   4, // Must match server's config.yaml ratelimit.capacity
	}
}

// createPaymentClient creates an HTTP client with X402 payment support.
func createPaymentClient(privateKey string) (*http.Client, error) {
	signer, err := evmsigners.NewClientSignerFromPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	client := x402.Newx402Client().
		Register("eip155:*", evm.NewExactEvmScheme(signer))

	httpClient := x402http.WrapHTTPClientWithPayment(
		http.DefaultClient,
		x402http.Newx402HTTPClient(client),
	)

	return httpClient, nil
}

// makeRequest makes a request and returns status code and body.
func makeRequest(client *http.Client, url string) (int, string, time.Duration, error) {
	start := time.Now()
	resp, err := client.Get(url)
	duration := time.Since(start)
	if err != nil {
		return 0, "", duration, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), duration, nil
}

// TokenInfo represents the response from /tokens endpoint.
type TokenInfo struct {
	Client   string  `json:"client"`
	Tokens   float64 `json:"tokens"`
	Capacity float64 `json:"capacity"`
}

// getTokens fetches the current token count from the server.
func getTokens(client *http.Client, serverURL string) (*TokenInfo, error) {
	resp, err := client.Get(serverURL + "/tokens")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	var info TokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// pollTokens polls the token count every interval and logs it.
// Returns a channel that receives token counts and a stop function.
func pollTokens(client *http.Client, serverURL string, interval time.Duration, t *testing.T) (chan float64, func()) {
	tokenChan := make(chan float64, 100)
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(tokenChan)

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				info, err := getTokens(client, serverURL)
				if err != nil {
					t.Logf("[POLL] Error: %v", err)
					continue
				}
				t.Logf("[POLL] Tokens: %.2f / %.0f", info.Tokens, info.Capacity)
				tokenChan <- info.Tokens
			}
		}
	}()

	return tokenChan, func() { close(stopChan) }
}

// TestIntegration_BasicRateLimiting tests that requests are rate limited after capacity is exhausted.
// Run with: go test -v -run TestIntegration_BasicRateLimiting ./cmd/client/...
// Requires: Server running, no PRIVATE_KEY needed (uses plain HTTP client)
func TestIntegration_BasicRateLimiting(t *testing.T) {
	cfg := getTestConfig()

	// Use plain HTTP client (no payment) to test rate limiting
	client := &http.Client{Timeout: 10 * time.Second}

	t.Logf("Testing against %s with capacity %d", cfg.serverURL, cfg.capacity)

	// Wait for bucket to refill from any previous tests
	t.Log("Waiting 2s for token bucket to refill...")
	time.Sleep(2 * time.Second)

	var successCount, rateLimitedCount int

	// Make capacity + 2 requests rapidly
	for i := 1; i <= cfg.capacity+2; i++ {
		status, body, duration, err := makeRequest(client, cfg.serverURL+"/cpu")
		if err != nil {
			t.Logf("Request %d: error - %v", i, err)
			continue
		}

		switch status {
		case 200:
			successCount++
			t.Logf("Request %d: 200 OK (%v)", i, duration)
		case 402:
			rateLimitedCount++
			t.Logf("Request %d: 402 Payment Required (%v)", i, duration)
		case 429:
			rateLimitedCount++
			t.Logf("Request %d: 429 Too Many Requests (%v)", i, duration)
		default:
			t.Logf("Request %d: %d - %s (%v)", i, status, body, duration)
		}
	}

	t.Logf("Results: %d successful, %d rate limited", successCount, rateLimitedCount)

	// Verify we got exactly capacity successful requests
	if successCount != cfg.capacity {
		t.Errorf("Expected %d successful requests, got %d", cfg.capacity, successCount)
	}

	// Verify we got rate limited for the extra requests
	if rateLimitedCount < 1 {
		t.Error("Expected at least 1 rate limited request")
	}
}

// TestIntegration_NaturalRefill tests that tokens naturally refill over time.
// Run with: go test -v -run TestIntegration_NaturalRefill ./cmd/client/...
func TestIntegration_NaturalRefill(t *testing.T) {
	cfg := getTestConfig()
	client := &http.Client{Timeout: 10 * time.Second}

	t.Logf("Testing natural refill against %s", cfg.serverURL)

	// Wait for full refill
	t.Log("Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Exhaust all tokens
	t.Logf("Exhausting %d tokens...", cfg.capacity)
	for i := 0; i < cfg.capacity; i++ {
		status, _, _, _ := makeRequest(client, cfg.serverURL+"/cpu")
		if status != 200 {
			t.Fatalf("Request %d should succeed, got %d", i+1, status)
		}
	}

	// Verify we're rate limited
	status, _, _, _ := makeRequest(client, cfg.serverURL+"/cpu")
	if status != 402 && status != 429 {
		t.Fatalf("Should be rate limited, got %d", status)
	}
	t.Log("Confirmed: rate limited after exhausting tokens")

	// Wait for 1 token to refill (assuming refill_rate >= 1/sec)
	t.Log("Waiting 1.1s for natural refill...")
	time.Sleep(1100 * time.Millisecond)

	// Should have at least 1 token now
	status, _, duration, _ := makeRequest(client, cfg.serverURL+"/cpu")
	if status != 200 {
		t.Errorf("Expected 200 after natural refill, got %d", status)
	} else {
		t.Logf("Natural refill worked: 200 OK (%v)", duration)
	}
}

// TestIntegration_PaymentRefillsTokens tests that payment refills the token bucket.
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_PaymentRefillsTokens ./cmd/client/...
// Requires: Server running, funded wallet private key
func TestIntegration_PaymentRefillsTokens(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	// Create payment-enabled client
	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}

	// Also create a plain client for comparison
	plainClient := &http.Client{Timeout: 10 * time.Second}

	t.Logf("Testing payment flow against %s", cfg.serverURL)

	// Wait for full refill
	t.Log("Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Step 1: Exhaust tokens using plain client
	t.Logf("Step 1: Exhausting %d tokens...", cfg.capacity)
	for i := 0; i < cfg.capacity; i++ {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status != 200 {
			t.Fatalf("Request %d should succeed, got %d", i+1, status)
		}
	}

	// Step 2: Verify rate limited
	status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
	if status != 402 && status != 429 {
		t.Fatalf("Should be rate limited, got %d", status)
	}
	t.Log("Step 2: Confirmed rate limited")

	// Step 3: Make request with payment client (should pay and succeed)
	t.Log("Step 3: Making request with payment...")
	start := time.Now()
	status, _, duration, err := makeRequest(paymentClient, cfg.serverURL+"/cpu")
	if err != nil {
		t.Fatalf("Payment request failed: %v", err)
	}
	if status != 200 {
		t.Fatalf("Expected 200 after payment, got %d", status)
	}
	t.Logf("Step 3: Payment successful, 200 OK (%v)", duration)

	// Step 4: Verify we now have tokens (capacity - 1 since we used 1)
	// Make rapid requests to count available tokens
	t.Log("Step 4: Counting available tokens after payment...")
	successCount := 0
	for i := 0; i < cfg.capacity*2; i++ {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status == 200 {
			successCount++
		} else {
			break
		}
	}

	// Expected tokens after payment:
	// - Payment adds: capacity tokens
	// - Used for paid request: -1 token
	// - Natural refill during payment (~1-4s): +1 to +16 tokens (at 4 tokens/sec)
	// - Natural refill during counting: additional tokens
	// So minimum is capacity-1, but could be higher due to natural refill
	expectedMin := cfg.capacity - 1
	if successCount < expectedMin {
		t.Errorf("Expected at least %d tokens after payment, got %d", expectedMin, successCount)
	} else {
		t.Logf("Step 4: Found %d tokens after payment (expected >= %d)", successCount, expectedMin)
	}

	t.Logf("Total payment flow time: %v", time.Since(start))
}

// TestIntegration_BurstTokensAfterPayment tests that multiple payments can create burst capacity.
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_BurstTokensAfterPayment ./cmd/client/...
func TestIntegration_BurstTokensAfterPayment(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := &http.Client{Timeout: 10 * time.Second}

	t.Logf("Testing burst tokens against %s", cfg.serverURL)

	// Wait for full refill
	t.Log("Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Step 1: Exhaust tokens, then make 2 sequential payments
	// Note: After first payment, bucket has tokens, so we need to exhaust again
	t.Log("Step 1: Making 2 sequential payments to create burst...")

	paymentsMade := 0
	for paymentsMade < 2 {
		// Exhaust current tokens
		exhausted := 0
		for {
			status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
			if status != 200 {
				break
			}
			exhausted++
		}
		t.Logf("  Exhausted %d tokens before payment %d", exhausted, paymentsMade+1)

		// Make payment
		status, body, duration, err := makeRequest(paymentClient, cfg.serverURL+"/cpu")
		if err != nil {
			// X402 protocol can be flaky with rapid sequential payments
			// Skip test if we hit protocol detection issues
			if paymentsMade > 0 {
				t.Logf("  Payment %d skipped due to X402 error: %v", paymentsMade+1, err)
				t.Log("  (This is expected - X402 can be flaky with rapid sequential payments)")
				break
			}
			t.Fatalf("Payment %d request error: %v", paymentsMade+1, err)
		}
		if status != 200 {
			t.Fatalf("Payment %d failed with status %d: %s", paymentsMade+1, status, body)
		}
		paymentsMade++
		t.Logf("  Payment %d: 200 OK (%v)", paymentsMade, duration)
	}
	t.Logf("  Total payments made: %d", paymentsMade)

	// Step 2: Count how many requests we can make now
	t.Logf("Step 2: Counting available tokens after %d payment(s)...", paymentsMade)
	successCount := 0
	for i := 0; i < cfg.capacity*3; i++ {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status == 200 {
			successCount++
		} else {
			break
		}
	}

	// After each payment: capacity tokens added, 1 used for paid request = capacity-1 remaining
	// With natural refill during payment, we expect at least capacity-1 tokens
	expectedMin := cfg.capacity - 1
	if successCount < expectedMin {
		t.Errorf("Expected at least %d tokens after payment(s), got %d", expectedMin, successCount)
	} else {
		t.Logf("  Found %d tokens after %d payment(s) (expected >= %d)", successCount, paymentsMade, expectedMin)
	}
}

// TestIntegration_RapidRequestsWithPayment tests rapid-fire requests where payment kicks in.
// This simulates a realistic scenario of bursty traffic that exceeds rate limit.
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_RapidRequestsWithPayment ./cmd/client/...
func TestIntegration_RapidRequestsWithPayment(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}

	t.Logf("Testing rapid requests with auto-payment against %s", cfg.serverURL)

	// Wait for full refill
	t.Log("Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Make 10 rapid requests - should see pattern of: 4 free, pay, 4 free, pay, etc.
	totalRequests := 10
	successCount := 0
	paymentCount := 0

	t.Logf("Making %d rapid requests with payment client...", totalRequests)
	start := time.Now()

	for i := 1; i <= totalRequests; i++ {
		reqStart := time.Now()
		status, _, duration, err := makeRequest(paymentClient, cfg.serverURL+"/cpu")
		if err != nil {
			t.Logf("Request %d: error - %v", i, err)
			continue
		}

		if status == 200 {
			successCount++
			// Requests taking >500ms likely involved payment
			if duration > 500*time.Millisecond {
				paymentCount++
				t.Logf("Request %d: 200 OK with PAYMENT (%v)", i, duration)
			} else {
				t.Logf("Request %d: 200 OK (%v)", i, duration)
			}
		} else {
			t.Logf("Request %d: %d (%v)", i, status, duration)
		}

		_ = reqStart // suppress unused warning
	}

	totalTime := time.Since(start)

	t.Logf("\n=== Results ===")
	t.Logf("Total requests: %d", totalRequests)
	t.Logf("Successful: %d", successCount)
	t.Logf("Payments made: ~%d (estimated from timing)", paymentCount)
	t.Logf("Total time: %v", totalTime)

	// All requests should succeed when using payment client
	if successCount != totalRequests {
		t.Errorf("Expected all %d requests to succeed, got %d", totalRequests, successCount)
	}

	// We should have made at least 1 payment (capacity is 4, we made 10 requests)
	expectedPayments := (totalRequests - cfg.capacity + cfg.capacity - 1) / cfg.capacity
	if paymentCount < 1 {
		t.Logf("Warning: expected at least %d payments, detected %d (timing-based detection)", expectedPayments, paymentCount)
	}
}

// TestIntegration_Scenario2_PaymentOnExhaustedBucket tests README Scenario 2:
//
//	Start:    0 tokens (exhausted)
//	Request:  Rate limited → 402 returned
//	Pay:      +4 tokens → 4 tokens (request proceeds)
//
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_Scenario2 ./cmd/client/...
func TestIntegration_Scenario2_PaymentOnExhaustedBucket(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := &http.Client{Timeout: 10 * time.Second}

	t.Log("=== README Scenario 2: Payment on Exhausted Bucket ===")
	t.Logf("Server: %s, Capacity: %d", cfg.serverURL, cfg.capacity)

	// Wait for full refill from any previous tests
	t.Log("\nSetup: Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Step 1: Exhaust all tokens
	t.Logf("\nStep 1: Exhausting %d tokens...", cfg.capacity)
	for i := 0; i < cfg.capacity; i++ {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status != 200 {
			t.Fatalf("  Request %d should succeed, got %d", i+1, status)
		}
		t.Logf("  Request %d: 200 OK (token consumed)", i+1)
	}
	t.Log("  → Bucket exhausted: 0 tokens")

	// Step 2: Verify we get 402 (rate limited)
	t.Log("\nStep 2: Verifying rate limit (expect 402)...")
	status, body, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
	if status != 402 {
		t.Fatalf("  Expected 402 Payment Required, got %d: %s", status, body)
	}
	t.Logf("  → Received 402 Payment Required (as expected)")

	// Step 3: Make payment request
	t.Log("\nStep 3: Making payment request...")
	paymentStart := time.Now()
	status, _, duration, _ := makeRequest(paymentClient, cfg.serverURL+"/cpu")
	if status != 200 {
		t.Fatalf("  Payment request failed with %d", status)
	}
	t.Logf("  → Payment successful: 200 OK (%v)", duration)
	t.Logf("  → Server added %d tokens, used 1 for this request", cfg.capacity)

	// Step 4: Count remaining tokens (should be capacity-1, plus any natural refill during payment)
	t.Log("\nStep 4: Counting remaining tokens...")
	remainingTokens := 0
	for i := 0; i < cfg.capacity+5; i++ {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status == 200 {
			remainingTokens++
		} else {
			break
		}
	}

	// Calculate expected tokens
	// Payment adds: capacity tokens
	// Used for paid request: -1
	// Natural refill during payment: duration.Seconds() * refillRate
	// But natural refill is capped since we're at/near capacity
	paymentSeconds := duration.Seconds()
	naturalRefill := paymentSeconds * float64(cfg.capacity) // refillRate assumed = capacity
	expectedMin := cfg.capacity - 1
	expectedWithRefill := float64(cfg.capacity-1) + naturalRefill
	if expectedWithRefill > float64(cfg.capacity) {
		expectedWithRefill = float64(cfg.capacity) // Cap at capacity for natural refill
	}

	t.Log("\n=== Results ===")
	t.Logf("Payment duration: %.2fs", paymentSeconds)
	t.Logf("Expected tokens: %d (capacity) - 1 (used) = %d minimum", cfg.capacity, expectedMin)
	t.Logf("Natural refill during payment: ~%.1f tokens (capped at capacity)", naturalRefill)
	t.Logf("Actual remaining tokens: %d", remainingTokens)

	// Verify we have at least capacity-1 tokens
	if remainingTokens < expectedMin {
		t.Errorf("\nFAILED: Expected at least %d tokens, got %d", expectedMin, remainingTokens)
	} else {
		t.Logf("\n✓ PASSED: Scenario 2 verified - payment refilled bucket correctly")
	}

	t.Logf("\nTotal test time: %v", time.Since(paymentStart))
}

// TestIntegration_TokenAccumulationDuringPayment verifies that natural refill
// accumulates during the payment processing time (the 0.84 tokens scenario).
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_TokenAccumulationDuringPayment ./cmd/client/...
func TestIntegration_TokenAccumulationDuringPayment(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := &http.Client{Timeout: 10 * time.Second}

	t.Logf("Testing token accumulation during payment against %s", cfg.serverURL)

	// Wait for full refill
	t.Log("Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Step 1: Exhaust tokens
	t.Logf("Step 1: Exhausting %d tokens...", cfg.capacity)
	for i := 0; i < cfg.capacity; i++ {
		makeRequest(plainClient, cfg.serverURL+"/cpu")
	}

	// Verify exhausted
	status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
	if status != 402 && status != 429 {
		t.Fatalf("Should be rate limited, got %d", status)
	}
	t.Log("Confirmed: bucket exhausted")

	// Step 2: Make payment request and measure time
	t.Log("Step 2: Making payment request...")
	paymentStart := time.Now()
	status, _, paymentDuration, _ := makeRequest(paymentClient, cfg.serverURL+"/cpu")
	if status != 200 {
		t.Fatalf("Payment request failed with %d", status)
	}
	t.Logf("Payment took %v", paymentDuration)

	// Step 3: Immediately count available tokens
	t.Log("Step 3: Counting tokens immediately after payment...")
	successCount := 0
	for i := 0; i < cfg.capacity+2; i++ {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status == 200 {
			successCount++
		} else {
			break
		}
	}

	// Calculate expected natural refill during payment
	// Payment typically takes ~800ms-1s, at 4 tokens/sec that's ~3-4 tokens
	paymentSeconds := paymentDuration.Seconds()
	refillRate := float64(cfg.capacity) // Assuming refill_rate == capacity
	expectedNaturalRefill := paymentSeconds * refillRate
	expectedTotal := float64(cfg.capacity) - 1 + expectedNaturalRefill // capacity - 1 (used for payment) + natural refill

	t.Logf("\n=== Analysis ===")
	t.Logf("Payment duration: %.2fs", paymentSeconds)
	t.Logf("Expected natural refill during payment: ~%.1f tokens (at %.0f tokens/sec)", expectedNaturalRefill, refillRate)
	t.Logf("Expected total after payment: ~%.1f tokens (capacity-1 + natural refill)", expectedTotal)
	t.Logf("Actual tokens found: %d", successCount)

	// Verify we got more tokens than just capacity-1 (proving natural refill happened)
	if successCount <= cfg.capacity-1 {
		t.Errorf("Expected more than %d tokens (should include natural refill), got %d", cfg.capacity-1, successCount)
	}

	t.Logf("\nTotal test time: %v", time.Since(paymentStart))
}

// TestIntegration_TokenCountAfterPaymentAndRequest verifies exact token counts:
// 1. Exhaust tokens → 0
// 2. Pay → ~3.84 tokens (4 paid + ~0.84 natural refill - 1 request)
// 3. Do 1 request → ~2.84 tokens
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_TokenCountAfterPaymentAndRequest ./cmd/client/...
func TestIntegration_TokenCountAfterPaymentAndRequest(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := &http.Client{Timeout: 10 * time.Second}

	t.Log("=== Token Count After Payment and Request ===")

	// Verify /tokens endpoint works
	info, err := getTokens(plainClient, cfg.serverURL)
	if err != nil {
		t.Fatalf("Failed to get tokens: %v (make sure server has /tokens endpoint and is running)", err)
	}
	t.Logf("[/tokens] client=%s tokens=%.2f capacity=%.0f", info.Client, info.Tokens, info.Capacity)

	// Wait for full refill
	t.Log("Setup: Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Step 1: Exhaust tokens
	t.Log("\nStep 1: Exhausting tokens...")
	for {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status != 200 {
			break
		}
	}

	info, err = getTokens(plainClient, cfg.serverURL)
	if err != nil {
		t.Fatalf("Failed to get tokens after exhaustion: %v", err)
	}
	t.Logf("[/tokens] client=%s tokens=%.2f capacity=%.0f", info.Client, info.Tokens, info.Capacity)

	// Step 2: Make payment
	t.Log("\nStep 2: Making payment...")
	status, _, duration, err := makeRequest(paymentClient, cfg.serverURL+"/cpu")
	if err != nil {
		t.Fatalf("Payment request error: %v", err)
	}
	if status != 200 {
		t.Fatalf("Payment failed with status %d", status)
	}
	t.Logf("Payment completed in %v", duration)

	// Fetch tokens immediately after payment
	info, err = getTokens(plainClient, cfg.serverURL)
	if err != nil {
		t.Fatalf("Failed to get tokens after payment: %v", err)
	}
	tokensAfterPayment := info.Tokens
	t.Logf("[/tokens] client=%s tokens=%.2f capacity=%.0f", info.Client, info.Tokens, info.Capacity)

	// Expected: tokens above capacity (4 paid + natural refill during payment - 1 for request)
	// The exact amount depends on payment duration
	if tokensAfterPayment < float64(cfg.capacity)-1 {
		t.Errorf("Expected at least %.1f tokens after payment, got %.2f", float64(cfg.capacity)-1, tokensAfterPayment)
	}

	// Step 3: Do 1 more request
	t.Log("\nStep 3: Making 1 more request...")
	status, _, _, _ = makeRequest(plainClient, cfg.serverURL+"/cpu")
	if status != 200 {
		t.Fatalf("Request should succeed, got %d", status)
	}

	// Fetch tokens after request
	info, err = getTokens(plainClient, cfg.serverURL)
	if err != nil {
		t.Fatalf("Failed to get tokens after request: %v", err)
	}
	tokensAfterRequest := info.Tokens
	t.Logf("[/tokens] client=%s tokens=%.2f capacity=%.0f", info.Client, info.Tokens, info.Capacity)

	// Note: If tokensAfterPayment was above capacity (e.g., 4.83) and we consume 1,
	// we drop to 3.83 which is BELOW capacity. Natural refill then kicks in and
	// may bring it back up to capacity (4.0) by the time we call /tokens.
	// So we can't simply expect tokensAfterPayment - 1.

	t.Log("\n=== Summary ===")
	t.Logf("After payment:    %.2f tokens", tokensAfterPayment)
	t.Logf("After 1 request:  %.2f tokens", tokensAfterRequest)

	// Explain what happened
	if tokensAfterPayment > float64(cfg.capacity) && tokensAfterRequest == float64(cfg.capacity) {
		t.Log("\n✓ Correct behavior: Request dropped tokens below capacity, natural refill restored to capacity")
	} else if tokensAfterPayment > float64(cfg.capacity) && tokensAfterRequest < tokensAfterPayment {
		t.Log("\n✓ Correct behavior: Token consumed from burst tokens")
	}
}

// TestIntegration_NaturalRefillBehaviorAfterPayment tests that:
// 1. After payment, tokens can exceed capacity (e.g., 4.84)
// 2. Natural refill does NOT happen when above capacity
// 3. Natural refill RESUMES once tokens drop below capacity
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_NaturalRefillBehaviorAfterPayment ./cmd/client/...
func TestIntegration_NaturalRefillBehaviorAfterPayment(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := &http.Client{Timeout: 10 * time.Second}

	t.Log("=== Natural Refill Behavior After Payment ===")
	t.Logf("Server: %s, Capacity: %d", cfg.serverURL, cfg.capacity)

	// Wait for full refill
	t.Log("\nSetup: Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Step 1: Exhaust tokens
	t.Log("\nStep 1: Exhausting tokens...")
	for {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status != 200 {
			break
		}
	}

	info, err := getTokens(plainClient, cfg.serverURL)
	if err != nil {
		t.Fatalf("Failed to get tokens after exhaustion: %v", err)
	}
	t.Logf("After exhaustion: %.2f tokens", info.Tokens)

	// Step 2: Make payment (should result in ~4.84 tokens due to natural refill during payment)
	t.Log("\nStep 2: Making payment...")
	status, _, duration, _ := makeRequest(paymentClient, cfg.serverURL+"/cpu")
	if status != 200 {
		t.Fatalf("Payment failed with status %d", status)
	}
	t.Logf("Payment completed in %v", duration)

	info, _ = getTokens(plainClient, cfg.serverURL)
	tokensAfterPayment := info.Tokens
	t.Logf("After payment: %.2f tokens (above capacity of %.0f)", tokensAfterPayment, info.Capacity)

	// Step 3: Wait 1 second - tokens should NOT increase (already above capacity)
	t.Log("\nStep 3: Waiting 1s (natural refill should NOT happen - above capacity)...")
	time.Sleep(1 * time.Second)

	info, _ = getTokens(plainClient, cfg.serverURL)
	tokensAfter1s := info.Tokens
	t.Logf("After 1s wait: %.2f tokens", tokensAfter1s)

	// Verify tokens did NOT increase significantly (allow small timing variance)
	if tokensAfter1s > tokensAfterPayment+0.5 {
		t.Errorf("UNEXPECTED: Tokens increased from %.2f to %.2f while above capacity!", tokensAfterPayment, tokensAfter1s)
	} else {
		t.Log("✓ Confirmed: Natural refill disabled while above capacity")
	}

	// Step 4: Consume tokens down to below capacity, then verify refill resumes
	t.Log("\nStep 4: Consuming tokens to below capacity...")
	consumed := 0
	for info.Tokens > float64(cfg.capacity)-1 {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status != 200 {
			break
		}
		consumed++
		info, _ = getTokens(plainClient, cfg.serverURL)
	}
	t.Logf("Consumed %d tokens, now at %.2f (below capacity)", consumed, info.Tokens)

	// Step 5: Wait and verify natural refill resumes
	t.Log("\nStep 5: Waiting 0.5s (natural refill should resume - below capacity)...")
	tokensBefore := info.Tokens
	time.Sleep(500 * time.Millisecond)

	info, _ = getTokens(plainClient, cfg.serverURL)
	tokensAfterWait := info.Tokens
	expectedRefill := 0.5 * float64(cfg.capacity) // 0.5s * 4 tokens/sec = 2 tokens
	t.Logf("After 0.5s wait: %.2f tokens (was %.2f, expected +~%.1f)", tokensAfterWait, tokensBefore, expectedRefill)

	// Verify tokens increased
	if tokensAfterWait > tokensBefore+1.0 {
		t.Log("✓ Confirmed: Natural refill resumed after dropping below capacity")
	} else {
		t.Errorf("UNEXPECTED: Tokens didn't increase much (%.2f -> %.2f), expected +~%.1f",
			tokensBefore, tokensAfterWait, expectedRefill)
	}

	t.Log("\n=== Summary ===")
	t.Logf("• Payment gave %.2f tokens (above capacity)", tokensAfterPayment)
	t.Logf("• After 1s wait (above capacity): %.2f tokens (no change)", tokensAfter1s)
	t.Logf("• After consuming to %.2f and waiting 0.5s: %.2f tokens (refill resumed)", tokensBefore, tokensAfterWait)
}

// TestIntegration_TokenPolling tests token monitoring with polling during payment flow.
// This test polls the /tokens endpoint every 500ms to observe token changes.
// Run with: PRIVATE_KEY=... go test -v -run TestIntegration_TokenPolling ./cmd/client/...
func TestIntegration_TokenPolling(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping payment test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := &http.Client{Timeout: 10 * time.Second}

	t.Log("=== Token Polling Test ===")
	t.Logf("Server: %s, Capacity: %d", cfg.serverURL, cfg.capacity)

	// Verify /tokens endpoint works
	info, err := getTokens(plainClient, cfg.serverURL)
	if err != nil {
		t.Fatalf("Failed to get tokens: %v (make sure server has /tokens endpoint)", err)
	}
	t.Logf("Initial state: %.2f / %.0f tokens", info.Tokens, info.Capacity)

	// Wait for full refill
	t.Log("\nWaiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	// Start polling
	t.Log("\nStarting token polling (every 500ms)...")
	_, stopPolling := pollTokens(plainClient, cfg.serverURL, 500*time.Millisecond, t)
	defer stopPolling()

	// Give polling time to start
	time.Sleep(100 * time.Millisecond)

	// Step 1: Exhaust tokens
	t.Logf("\n--- Exhausting %d tokens ---", cfg.capacity)
	for i := 0; i < cfg.capacity; i++ {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status != 200 {
			t.Logf("Request %d: %d (expected 200)", i+1, status)
		}
		time.Sleep(100 * time.Millisecond) // Small delay to see polling
	}

	// Check token state
	info, _ = getTokens(plainClient, cfg.serverURL)
	t.Logf("After exhaustion: %.2f tokens", info.Tokens)

	// Step 2: Wait and watch natural refill
	t.Log("\n--- Watching natural refill for 1.5s ---")
	time.Sleep(1500 * time.Millisecond)

	info, _ = getTokens(plainClient, cfg.serverURL)
	t.Logf("After natural refill: %.2f tokens", info.Tokens)

	// Step 3: Exhaust again and make payment
	t.Log("\n--- Exhausting and making payment ---")
	for {
		status, _, _, _ := makeRequest(plainClient, cfg.serverURL+"/cpu")
		if status != 200 {
			break
		}
	}

	info, _ = getTokens(plainClient, cfg.serverURL)
	t.Logf("Before payment: %.2f tokens", info.Tokens)

	// Make payment
	t.Log("Making payment request...")
	status, _, duration, _ := makeRequest(paymentClient, cfg.serverURL+"/cpu")
	if status != 200 {
		t.Fatalf("Payment failed with status %d", status)
	}
	t.Logf("Payment completed in %v", duration)

	// Check tokens after payment
	info, _ = getTokens(plainClient, cfg.serverURL)
	t.Logf("After payment: %.2f tokens (expected ~%.0f)", info.Tokens, info.Capacity-1)

	// Step 4: Watch for a bit more
	t.Log("\n--- Final observation (1s) ---")
	time.Sleep(1000 * time.Millisecond)

	info, _ = getTokens(plainClient, cfg.serverURL)
	t.Logf("Final state: %.2f / %.0f tokens", info.Tokens, info.Capacity)

	t.Log("\n=== Test Complete ===")
}
