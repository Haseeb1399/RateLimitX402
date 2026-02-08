#!/usr/bin/env python3
"""
Bursty Load Experiment
======================

Shows how async X402 outperforms sync when traffic is bursty.
Bursty = users make many requests quickly, then pause.
"""

import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

from x402 import SimulationConfig, simulate_scheme


def run_burst_experiment():
    """Compare sync vs async under bursty load."""
    print("=" * 60)
    print("BURSTY LOAD EXPERIMENT")
    print("=" * 60)
    print("Simulating bursty traffic: users request rapidly, then pause.\n")
    
    # X FREE tier with aggressive request rate (bursty)
    # Users make requests every 10ms (100 req/sec) - much faster than refill
    config = SimulationConfig(
        num_users=100,
        requests_per_user=1000,
        token_capacity=15,             # X FREE tier - small bucket!
        refill_rate=0.017,             # 1 req/min - very slow refill
        tokens_per_request=1,
        price_per_refill_usd=0.10,
        tokens_per_payment=15,
        trust_threshold=3,
        avg_request_interval_ms=10.0,  # VERY FAST - 100 req/sec per user!
        load_multiplier=1.0,
        user_patience_ms=999999999.0,
    )
    
    total_requests = config.num_users * config.requests_per_user
    print(f"Config: {config.token_capacity} tokens, {config.refill_rate}/sec refill")
    print(f"Request rate: {1000/config.avg_request_interval_ms:.0f} req/sec per user (BURSTY)")
    print(f"Workload: {total_requests:,} total requests\n")
    
    # Run simulations
    results = {}
    for scheme in ["no_x402", "sync", "async"]:
        results[scheme] = simulate_scheme(config, scheme)
        print(results[scheme].summary())
    
    # Print comparison
    sync = results["sync"]
    async_r = results["async"]
    
    print("\n" + "=" * 60)
    print("📊 BURSTY LOAD RESULTS")
    print("=" * 60)
    print(f"Payments triggered: {sync.total_payments:,}")
    print(f"Payment rate: {sync.total_payments/total_requests*100:.1f}% of requests")
    print(f"Total X402 Cost: ${async_r.total_revenue:.2f}")
    print(f"\nSync time:  {sync.total_time_ms/1000/60:.1f} minutes")
    print(f"Async time: {async_r.total_time_ms/1000/60:.1f} minutes")
    print(f"Speedup:    {sync.total_time_ms/async_r.total_time_ms:.2f}x")
    print(f"\nSync avg latency:  {sync.avg_latency_ms:.0f}ms")
    print(f"Async avg latency: {async_r.avg_latency_ms:.0f}ms")
    print(f"P95 Sync:  {sync.p95_latency_ms:.0f}ms")
    print(f"P95 Async: {async_r.p95_latency_ms:.0f}ms")
    
    # Plot - 2 subplots: Time and P95 Latency
    fig, axes = plt.subplots(1, 2, figsize=(14, 5))
    schemes = ["No X402", "Sync X402", "Async X402"]
    times = [results["no_x402"].total_time_ms/1000/60,  # minutes
             sync.total_time_ms/1000/60,
             async_r.total_time_ms/1000/60]
    costs = [results["no_x402"].total_revenue,
             sync.total_revenue,
             async_r.total_revenue]
    p95s = [results["no_x402"].p95_latency_ms,
            sync.p95_latency_ms,
            async_r.p95_latency_ms]
    colors = ["#e74c3c", "#f39c12", "#27ae60"]
    
    # 1. Time with cost labels
    ax = axes[0]
    bars = ax.bar(schemes, times, color=colors)
    ax.set_ylabel('Time (minutes)')
    ax.set_title(f'Bursty Load: {sync.total_payments:,} payments triggered')
    for bar, t, c in zip(bars, times, costs):
        label = f'{t:.1f}m\n(${c:.0f})' if c > 0 else f'{t:.0f}m\n($0)'
        ax.annotate(label, xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontweight='bold', fontsize=9)
    ax.set_yscale('log')
    ax.grid(True, alpha=0.3, axis='y')
    ax.margins(y=0.2)
    
    # 2. P95 Latency
    ax = axes[1]
    bars = ax.bar(schemes, p95s, color=colors)
    ax.set_ylabel('P95 Latency (ms)')
    ax.set_title('P95 Latency Comparison')
    for bar, p in zip(bars, p95s):
        ax.annotate(f'{p:.0f}ms', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontweight='bold', fontsize=9)
    ax.set_yscale('log')
    ax.grid(True, alpha=0.3, axis='y')
    ax.margins(y=0.2)
    
    plt.tight_layout()
    plt.savefig("bursty_load.png", dpi=150)
    print(f"\nSaved: bursty_load.png")
    plt.close()


if __name__ == "__main__":
    run_burst_experiment()
