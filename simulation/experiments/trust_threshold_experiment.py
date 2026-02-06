#!/usr/bin/env python3
"""
Example Experiment: Trust Threshold Analysis
=============================================

This experiment tests how different trust thresholds affect
the async scheme's performance.

Run with:
    python experiments/trust_threshold_experiment.py
"""

import sys
sys.path.insert(0, '..')  # Add parent to path for x402 import

from x402 import SimulationConfig, simulate_scheme


def main():
    print("=" * 60)
    print("Experiment: Trust Threshold Impact on Async Performance")
    print("=" * 60)
    
    # Base configuration with strict rate limits
    base_config = SimulationConfig(
        token_capacity=20,
        refill_rate=0.5,
        price_per_refill_usd=0.05,
        tokens_per_payment=20,
        num_users=500,
        requests_per_user=100,
        avg_request_interval_ms=100,
        load_multiplier=5.0,
    )
    
    # Test different trust thresholds
    thresholds = [1, 2, 3, 5, 10]
    
    print(f"\nBase config: capacity={base_config.token_capacity}, refill={base_config.refill_rate}/s")
    print(f"Users: {base_config.num_users}, Requests/user: {base_config.requests_per_user}")
    print("\n" + "-" * 80)
    print(f"{'Threshold':<12} {'Scheme':<10} {'Duration':>10} {'RPS':>10} {'Revenue':>10} {'Payments':>10}")
    print("-" * 80)
    
    for threshold in thresholds:
        config = SimulationConfig(**base_config.to_dict())
        config.trust_threshold = threshold
        
        # Run sync and async
        sync_result = simulate_scheme(config, "sync")
        async_result = simulate_scheme(config, "async")
        
        print(f"{threshold:<12} {'sync':<10} {sync_result.duration_str:>10} {sync_result.throughput_rps:>9.1f}/s ${sync_result.total_revenue:>9.2f} {sync_result.total_payments:>10}")
        print(f"{'':<12} {'async':<10} {async_result.duration_str:>10} {async_result.throughput_rps:>9.1f}/s ${async_result.total_revenue:>9.2f} {async_result.total_payments:>10}")
        
        # Calculate speedup
        speedup = sync_result.total_time_ms / max(1, async_result.total_time_ms)
        print(f"{'':<12} {'→ speedup':<10} {speedup:>10.2f}x")
        print("-" * 80)


if __name__ == "__main__":
    main()
