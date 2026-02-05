package trust

import (
	"sync"
	"time"
)

// Config holds trust tracker configuration.
type Config struct {
	Threshold int           // Successful payments needed to become trusted
	Window    time.Duration // Time window for counting payments
}

// Tracker tracks wallet trust based on payment history.
type Tracker struct {
	mu       sync.RWMutex
	payments map[string][]time.Time // wallet address → payment timestamps
	config   Config
}

// New creates a new trust tracker with the given config.
func New(cfg Config) *Tracker {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 3
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Hour
	}
	return &Tracker{
		payments: make(map[string][]time.Time),
		config:   cfg,
	}
}

// IsTrusted returns true if the wallet has enough recent successful payments.
func (t *Tracker) IsTrusted(wallet string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.countRecent(wallet) >= t.config.Threshold
}

// countRecent counts payments within the time window (must hold lock).
func (t *Tracker) countRecent(wallet string) int {
	cutoff := time.Now().Add(-t.config.Window)
	count := 0
	for _, ts := range t.payments[wallet] {
		if ts.After(cutoff) {
			count++
		}
	}
	return count
}

// RecordSuccess adds a successful payment timestamp for the wallet.
func (t *Tracker) RecordSuccess(wallet string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.payments[wallet] = append(t.payments[wallet], time.Now())
	t.cleanup(wallet)
}

// RecordFailure clears payment history for the wallet (soft penalty).
func (t *Tracker) RecordFailure(wallet string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.payments, wallet)
}

// cleanup removes expired timestamps to prevent memory growth (must hold lock).
func (t *Tracker) cleanup(wallet string) {
	cutoff := time.Now().Add(-t.config.Window)
	payments := t.payments[wallet]
	kept := make([]time.Time, 0, len(payments))
	for _, ts := range payments {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	t.payments[wallet] = kept
}

// Stats returns trust statistics for monitoring.
type Stats struct {
	TrustedWallets   int `json:"trusted_wallets"`
	TotalWalletsSeen int `json:"total_wallets_seen"`
}

func (t *Tracker) Stats() Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	trusted := 0
	for wallet := range t.payments {
		if t.countRecent(wallet) >= t.config.Threshold {
			trusted++
		}
	}
	return Stats{
		TrustedWallets:   trusted,
		TotalWalletsSeen: len(t.payments),
	}
}

// RecentPayments returns the count of recent payments for a wallet.
func (t *Tracker) RecentPayments(wallet string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.countRecent(wallet)
}
