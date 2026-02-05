package main

import (
	"context"
	"log"
	"sync"
	"time"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	"github.com/haseeb/ratelimiter/pkg/trust"
)

// SettlementJob represents a background settlement to process.
type SettlementJob struct {
	PaymentPayload      x402.PaymentPayload
	PaymentRequirements x402.PaymentRequirements
	WalletAddr          string
	QueuedAt            time.Time
}

// SettlementQueue processes settlements sequentially to avoid nonce collisions.
type SettlementQueue struct {
	jobs         chan SettlementJob
	httpServer   *x402http.HTTPServer
	trustTracker *trust.Tracker
	wg           sync.WaitGroup
	mu           sync.Mutex
	pending      int
}

// NewSettlementQueue creates a new settlement queue with a worker.
func NewSettlementQueue(httpServer *x402http.HTTPServer, trustTracker *trust.Tracker, bufferSize int) *SettlementQueue {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	sq := &SettlementQueue{
		jobs:         make(chan SettlementJob, bufferSize),
		httpServer:   httpServer,
		trustTracker: trustTracker,
	}

	// Start worker goroutine
	sq.wg.Add(1)
	go sq.worker()

	return sq
}

// Enqueue adds a settlement job to the queue.
func (sq *SettlementQueue) Enqueue(job SettlementJob) {
	sq.mu.Lock()
	sq.pending++
	sq.mu.Unlock()

	job.QueuedAt = time.Now()
	sq.jobs <- job
	log.Printf("[QUEUE] Enqueued settlement for wallet %s (pending: %d)",
		truncateWallet(job.WalletAddr), sq.Pending())
}

// Pending returns the number of pending settlements.
func (sq *SettlementQueue) Pending() int {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return sq.pending
}

// worker processes settlements one at a time with delay between each.
func (sq *SettlementQueue) worker() {
	defer sq.wg.Done()

	first := true
	for job := range sq.jobs {
		// Add delay between settlements to let blockchain state propagate
		// Skip delay for the first job
		if !first {
			log.Printf("[QUEUE] Waiting 3s before next settlement...")
			time.Sleep(3 * time.Second)
		}
		first = false

		sq.processSettlement(job)

		sq.mu.Lock()
		sq.pending--
		sq.mu.Unlock()
	}
}

// processSettlement handles a single settlement.
func (sq *SettlementQueue) processSettlement(job SettlementJob) {
	queueLatency := time.Since(job.QueuedAt)
	settlementStart := time.Now()

	settleResult := sq.httpServer.ProcessSettlement(
		context.Background(),
		job.PaymentPayload,
		job.PaymentRequirements,
	)
	settlementLatency := time.Since(settlementStart)

	if settleResult.Success {
		if sq.trustTracker != nil {
			sq.trustTracker.RecordSuccess(job.WalletAddr)
		}
		log.Printf("[QUEUE] Settlement succeeded: %s (queue: %v, settle: %v)",
			settleResult.Transaction, queueLatency, settlementLatency)
	} else {
		if sq.trustTracker != nil {
			// Soft penalty: revoke trust, don't debit tokens
			sq.trustTracker.RecordFailure(job.WalletAddr)
		}
		log.Printf("[QUEUE] Settlement FAILED: %s (queue: %v, wallet trust revoked)",
			settleResult.ErrorReason, queueLatency)
	}
}

// Close shuts down the queue gracefully.
func (sq *SettlementQueue) Close() {
	close(sq.jobs)
	sq.wg.Wait()
}
