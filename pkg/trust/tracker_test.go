package trust

import (
	"testing"
	"time"
)

func TestTracker_IsTrusted(t *testing.T) {
	tracker := New(Config{
		Threshold: 3,
		Window:    time.Hour,
	})

	wallet := "0x1234567890abcdef"

	// Initially not trusted
	if tracker.IsTrusted(wallet) {
		t.Error("New wallet should not be trusted")
	}

	// After 1 payment - still not trusted
	tracker.RecordSuccess(wallet)
	if tracker.IsTrusted(wallet) {
		t.Error("Wallet with 1 payment should not be trusted")
	}

	// After 2 payments - still not trusted
	tracker.RecordSuccess(wallet)
	if tracker.IsTrusted(wallet) {
		t.Error("Wallet with 2 payments should not be trusted")
	}

	// After 3 payments - now trusted
	tracker.RecordSuccess(wallet)
	if !tracker.IsTrusted(wallet) {
		t.Error("Wallet with 3 payments should be trusted")
	}
}

func TestTracker_RecordFailure_RevokesTrust(t *testing.T) {
	tracker := New(Config{
		Threshold: 3,
		Window:    time.Hour,
	})

	wallet := "0xabcdef1234567890"

	// Build trust
	tracker.RecordSuccess(wallet)
	tracker.RecordSuccess(wallet)
	tracker.RecordSuccess(wallet)

	if !tracker.IsTrusted(wallet) {
		t.Fatal("Wallet should be trusted after 3 payments")
	}

	// Failure revokes trust (soft penalty)
	tracker.RecordFailure(wallet)

	if tracker.IsTrusted(wallet) {
		t.Error("Wallet should not be trusted after failure")
	}

	// Needs to rebuild trust from scratch
	if tracker.RecentPayments(wallet) != 0 {
		t.Error("Wallet should have 0 payments after failure")
	}
}

func TestTracker_WindowExpiry(t *testing.T) {
	tracker := New(Config{
		Threshold: 2,
		Window:    100 * time.Millisecond, // Very short for testing
	})

	wallet := "0xtest"

	// Build trust
	tracker.RecordSuccess(wallet)
	tracker.RecordSuccess(wallet)

	if !tracker.IsTrusted(wallet) {
		t.Error("Wallet should be trusted with 2 payments")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	if tracker.IsTrusted(wallet) {
		t.Error("Trust should expire after window passes")
	}
}

func TestTracker_DifferentWallets(t *testing.T) {
	tracker := New(Config{
		Threshold: 2,
		Window:    time.Hour,
	})

	wallet1 := "0xwallet1"
	wallet2 := "0xwallet2"

	// Trust wallet1
	tracker.RecordSuccess(wallet1)
	tracker.RecordSuccess(wallet1)

	if !tracker.IsTrusted(wallet1) {
		t.Error("Wallet1 should be trusted")
	}

	if tracker.IsTrusted(wallet2) {
		t.Error("Wallet2 should not be trusted")
	}
}

func TestTracker_Stats(t *testing.T) {
	tracker := New(Config{
		Threshold: 2,
		Window:    time.Hour,
	})

	// Add some wallets with varying payment counts
	tracker.RecordSuccess("wallet1")
	tracker.RecordSuccess("wallet1")
	tracker.RecordSuccess("wallet2")
	tracker.RecordSuccess("wallet3")
	tracker.RecordSuccess("wallet3")
	tracker.RecordSuccess("wallet3")

	stats := tracker.Stats()

	if stats.TotalWalletsSeen != 3 {
		t.Errorf("Expected 3 total wallets, got %d", stats.TotalWalletsSeen)
	}

	// wallet1 (2 payments) and wallet3 (3 payments) are trusted, wallet2 (1 payment) is not
	if stats.TrustedWallets != 2 {
		t.Errorf("Expected 2 trusted wallets, got %d", stats.TrustedWallets)
	}
}

func TestTracker_DefaultConfig(t *testing.T) {
	// Test with zero values (should use defaults)
	tracker := New(Config{})

	wallet := "0xtest"

	// Default threshold is 3
	tracker.RecordSuccess(wallet)
	tracker.RecordSuccess(wallet)
	if tracker.IsTrusted(wallet) {
		t.Error("Should not be trusted with 2 payments (default threshold is 3)")
	}

	tracker.RecordSuccess(wallet)
	if !tracker.IsTrusted(wallet) {
		t.Error("Should be trusted with 3 payments")
	}
}

func TestTracker_Concurrent(t *testing.T) {
	tracker := New(Config{
		Threshold: 10,
		Window:    time.Hour,
	})

	wallet := "0xconcurrent"
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func() {
			tracker.RecordSuccess(wallet)
			done <- true
		}()
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			tracker.IsTrusted(wallet)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Should have 10 payments
	if tracker.RecentPayments(wallet) != 10 {
		t.Errorf("Expected 10 payments, got %d", tracker.RecentPayments(wallet))
	}
}
