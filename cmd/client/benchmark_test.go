package main

import (
	"net/http"
	"sort"
	"testing"
	"time"
)

// TestBenchmark_CompareSyncVsOptimistic compares latency for trusted vs untrusted.
// This test demonstrates the difference in response time:
// - First 3 payments: Synchronous (~1.5s each) - building trust
// - Next payments: Optimistic (~200ms each) - trusted
//
// Run with: PRIVATE_KEY=... go test -v -run TestBenchmark_CompareSyncVsOptimistic
func TestBenchmark_CompareSyncVsOptimistic(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping benchmark test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := createPlainClient()

	t.Log("=== Sync vs Optimistic Benchmark ===")
	t.Logf("Server: %s, Capacity: %d", cfg.serverURL, cfg.capacity)
	t.Log("Trust threshold: 3 successful payments in 1 hour")
	t.Log("")

	// Wait for bucket refill from previous tests
	t.Log("Setup: Waiting 2s for full refill...")
	time.Sleep(2 * time.Second)

	var syncLatencies []time.Duration
	var optimisticLatencies []time.Duration

	// Phase 1: Build trust (first 3 payments - synchronous)
	t.Log("Phase 1: Building trust (3 payments, should be ~1.5s - 3.0s each)")
	for i := 1; i <= 3; i++ {
		// Exhaust tokens first
		exhaustTokens(plainClient, cfg.serverURL)

		// Make payment request
		start := time.Now()
		status, _, duration, _ := makeRequest(paymentClient, cfg.serverURL+"/cpu")
		latency := time.Since(start)

		if status == 200 {
			syncLatencies = append(syncLatencies, latency)
			t.Logf("  Payment %d: 200 OK in %v (sync)", i, duration)
		} else {
			t.Logf("  Payment %d: %d in %v", i, status, duration)
		}
	}

	// Phase 2: Trusted payments (should be optimistic - ~200ms)
	t.Log("")
	t.Log("Phase 2: Trusted payments (3 payments, should be ~200ms each)")
	for i := 1; i <= 3; i++ {
		// Exhaust tokens first
		exhaustTokens(plainClient, cfg.serverURL)

		// Make payment request
		start := time.Now()
		status, _, duration, _ := makeRequest(paymentClient, cfg.serverURL+"/cpu")
		latency := time.Since(start)

		if status == 200 {
			optimisticLatencies = append(optimisticLatencies, latency)
			t.Logf("  Payment %d: 200 OK in %v (optimistic)", i, duration)
		} else {
			t.Logf("  Payment %d: %d in %v", i, status, duration)
		}
	}

	// Calculate and display results
	t.Log("")
	t.Log("=== Results ===")

	if len(syncLatencies) > 0 {
		syncStats := calculateStats(syncLatencies)
		t.Logf("Synchronous (untrusted) payments:")
		t.Logf("  Count: %d", len(syncLatencies))
		t.Logf("  Avg:   %v", syncStats.avg)
		t.Logf("  Min:   %v", syncStats.min)
		t.Logf("  Max:   %v", syncStats.max)
	}

	if len(optimisticLatencies) > 0 {
		optStats := calculateStats(optimisticLatencies)
		t.Logf("Optimistic (trusted) payments:")
		t.Logf("  Count: %d", len(optimisticLatencies))
		t.Logf("  Avg:   %v", optStats.avg)
		t.Logf("  Min:   %v", optStats.min)
		t.Logf("  Max:   %v", optStats.max)
	}

	// Calculate speedup
	if len(syncLatencies) > 0 && len(optimisticLatencies) > 0 {
		syncStats := calculateStats(syncLatencies)
		optStats := calculateStats(optimisticLatencies)
		speedup := float64(syncStats.avg) / float64(optStats.avg)
		t.Logf("")
		t.Logf("Speedup: %.1fx faster when trusted", speedup)

		// Verify significant speedup (should be at least 2x)
		if speedup < 2.0 {
			t.Logf("Warning: Expected at least 2x speedup, got %.1fx", speedup)
		} else {
			t.Logf("✓ Optimistic settlement working correctly!")
		}
	}
}

// TestBenchmark_LatencyDistribution measures p50/p95/p99 latencies.
// Run with: PRIVATE_KEY=... go test -v -run TestBenchmark_LatencyDistribution
func TestBenchmark_LatencyDistribution(t *testing.T) {
	cfg := getTestConfig()

	if cfg.privateKey == "" {
		t.Skip("PRIVATE_KEY not set - skipping benchmark test")
	}

	paymentClient, err := createPaymentClient(cfg.privateKey)
	if err != nil {
		t.Fatalf("Failed to create payment client: %v", err)
	}
	plainClient := createPlainClient()

	t.Log("=== Latency Distribution Benchmark ===")
	t.Logf("Server: %s", cfg.serverURL)

	// Wait for bucket refill
	time.Sleep(2 * time.Second)

	// Make 10 payment requests to get distribution
	const numRequests = 10
	var latencies []time.Duration

	t.Logf("Making %d payment requests...", numRequests)
	for i := 1; i <= numRequests; i++ {
		// Exhaust tokens first
		exhaustTokens(plainClient, cfg.serverURL)

		start := time.Now()
		status, _, _, _ := makeRequest(paymentClient, cfg.serverURL+"/cpu")
		latency := time.Since(start)

		if status == 200 {
			latencies = append(latencies, latency)
			t.Logf("  Request %d: %v", i, latency)
		}
	}

	if len(latencies) == 0 {
		t.Fatal("No successful payments recorded")
	}

	// Sort for percentile calculation
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	stats := calculateStats(latencies)
	p50 := percentile(latencies, 50)
	p95 := percentile(latencies, 95)
	p99 := percentile(latencies, 99)

	t.Log("")
	t.Log("=== Latency Distribution ===")
	t.Logf("  Min:  %v", stats.min)
	t.Logf("  Avg:  %v", stats.avg)
	t.Logf("  P50:  %v", p50)
	t.Logf("  P95:  %v", p95)
	t.Logf("  P99:  %v", p99)
	t.Logf("  Max:  %v", stats.max)
}

// exhaustTokens makes requests until rate limited.
func exhaustTokens(client *http.Client, serverURL string) {
	for {
		status, _, _, _ := makeRequest(client, serverURL+"/cpu")
		if status != 200 {
			break
		}
	}
}

// createPlainClient creates an HTTP client without payment support.
func createPlainClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// Helper type for http.Client compatibility
type Response struct {
	StatusCode int
}

type latencyStats struct {
	min, max, avg time.Duration
}

func calculateStats(latencies []time.Duration) latencyStats {
	if len(latencies) == 0 {
		return latencyStats{}
	}

	var sum time.Duration
	min := latencies[0]
	max := latencies[0]

	for _, l := range latencies {
		sum += l
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
	}

	return latencyStats{
		min: min,
		max: max,
		avg: sum / time.Duration(len(latencies)),
	}
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) - 1) * p / 100
	return sorted[idx]
}
