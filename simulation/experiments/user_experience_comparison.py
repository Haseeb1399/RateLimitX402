#!/usr/bin/env python3
"""
User Experience Comparison Experiment
======================================

Compare the three payment schemes from a single user's perspective:
- Time to complete N requests
- Money spent
- Effective throughput

Uses the existing simulation engine - just configures it for a single-user analysis.
"""

import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

from x402 import SimulationConfig, simulate_scheme, SimulationResult


def plot_cross_platform_comparison(all_results: dict, save_path: str = None):
    """Visualize comparison across all platforms."""
    fig, axes = plt.subplots(1, 2, figsize=(14, 6))
    
    platforms = list(all_results.keys())
    schemes = ["no_x402", "sync", "async"]
    scheme_labels = ["No X402", "Sync X402", "Async X402"]
    colors = ["#e74c3c", "#f39c12", "#27ae60"]
    
    x = np.arange(len(platforms))
    width = 0.25
    
    # 1. Total Time (minutes) - grouped by platform
    ax = axes[0]
    for i, (scheme, color, label) in enumerate(zip(schemes, colors, scheme_labels)):
        times = [all_results[p][scheme].total_time_ms / 1000 / 60 for p in platforms]  # minutes
        bars = ax.bar(x + i*width, times, width, label=label, color=color)
        for bar, t in zip(bars, times):
            ax.annotate(f'{t:.0f}m', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                       ha='center', va='bottom', fontsize=7, clip_on=False)
    ax.set_ylabel('Time (minutes)')
    ax.set_title('Time to Complete (per user)', pad=20)
    ax.set_xticks(x + width)
    ax.set_xticklabels(platforms)
    ax.legend()
    ax.set_yscale('log')  # Log scale for better visibility
    ax.grid(True, alpha=0.3, axis='y')
    ax.margins(y=0.2)  # Add margin to prevent label overlap
    
    # 2. Effective Throughput (req/s) - grouped by platform
    ax = axes[1]
    for i, (scheme, color, label) in enumerate(zip(schemes, colors, scheme_labels)):
        throughputs = [all_results[p][scheme].throughput_rps for p in platforms]
        bars = ax.bar(x + i*width, throughputs, width, label=label, color=color)
        for bar, t in zip(bars, throughputs):
            ax.annotate(f'{t:.1f}', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                       ha='center', va='bottom', fontsize=8)
    ax.set_ylabel('Requests per Second')
    ax.set_title('Effective Throughput')
    ax.set_xticks(x + width)
    ax.set_xticklabels(platforms)
    ax.legend()
    ax.grid(True, alpha=0.3, axis='y')
    
    plt.tight_layout()
    if save_path:
        plt.savefig(save_path, dpi=150)
        print(f"\nSaved: {save_path}")
    plt.close()


def print_comparison(results: dict):
    """Print comparison table."""
    print("\n" + "=" * 90)
    print("USER EXPERIENCE COMPARISON")
    print("=" * 90)
    
    no_x402, sync, async_r = results["no_x402"], results["sync"], results["async"]
    
    print(f"\n{'Metric':<25} {'No X402':>15} {'Sync X402':>15} {'Async X402':>15} {'Speedup':>12}")
    print("-" * 90)
    
    rows = [
        ("Total Time (s)", 
         f"{no_x402.total_time_ms/1000:.1f}", f"{sync.total_time_ms/1000:.1f}", f"{async_r.total_time_ms/1000:.1f}",
         f"{sync.total_time_ms/async_r.total_time_ms:.2f}x"),
        ("Throughput (req/s)", 
         f"{no_x402.throughput_rps:.2f}", f"{sync.throughput_rps:.2f}", f"{async_r.throughput_rps:.2f}",
         f"{async_r.throughput_rps/max(0.01, sync.throughput_rps):.2f}x"),
        ("Money Spent ($)", 
         f"{no_x402.total_revenue:.4f}", f"{sync.total_revenue:.4f}", f"{async_r.total_revenue:.4f}", "-"),
        ("Payments", 
         f"{no_x402.total_payments}", f"{sync.total_payments}", f"{async_r.total_payments}", "-"),
        ("Avg Latency (ms)", 
         f"{no_x402.avg_latency_ms:.0f}", f"{sync.avg_latency_ms:.0f}", f"{async_r.avg_latency_ms:.0f}",
         f"{sync.avg_latency_ms/max(1, async_r.avg_latency_ms):.2f}x"),
        ("P95 Latency (ms)", 
         f"{no_x402.p95_latency_ms:.0f}", f"{sync.p95_latency_ms:.0f}", f"{async_r.p95_latency_ms:.0f}",
         f"{sync.p95_latency_ms/max(1, async_r.p95_latency_ms):.2f}x"),
    ]
    
    for row in rows:
        print(f"{row[0]:<25} {row[1]:>15} {row[2]:>15} {row[3]:>15} {row[4]:>12}")
    print("=" * 90)


def get_reddit_config() -> SimulationConfig:
    """Reddit API Rate Limits (2024)
    - 100 queries per minute (OAuth) = ~1.67 req/sec
    - Paid tier: $0.24 per 1,000 API calls
    Source: https://support.reddithelp.com/hc/en-us/articles/16160319875092-Reddit-Data-API-Wiki
    """
    return SimulationConfig(
        num_users=1000,
        requests_per_user=1000,
        token_capacity=100,
        refill_rate=1.67,              # ~100 per minute
        tokens_per_request=1,
        price_per_refill_usd=0.024,    # $0.24/1000 = $0.024/100
        tokens_per_payment=100,
        trust_threshold=3,
        avg_request_interval_ms=50.0,
        load_multiplier=1.0,
        user_patience_ms=999999999.0,  # Users never churn
    )


def get_x_config() -> SimulationConfig:
    """X (Twitter) API Rate Limits - Basic Tier ($200/mo)
    - 450 req/15min (30x free tier)
    Source: https://developer.x.com/en/docs/x-api/rate-limits
    """
    return SimulationConfig(
        num_users=1000,
        requests_per_user=1000,
        token_capacity=450,            # 30x more tokens than free
        refill_rate=0.5,               # 450/900sec = 0.5/sec (30x faster than free)
        tokens_per_request=1,
        price_per_refill_usd=0.10,
        tokens_per_payment=450,
        trust_threshold=3,
        avg_request_interval_ms=50.0,
        load_multiplier=1.0,
        user_patience_ms=999999999.0,  # Users never churn
    )


def get_instagram_config() -> SimulationConfig:
    """Instagram Graph API Rate Limits (2024)
    - 200 requests per hour per user = ~3.3 req/min = 0.056/sec
    - Rolling hourly window
    Source: https://developers.facebook.com/docs/instagram-api/overview#rate-limiting
    """
    return SimulationConfig(
        num_users=1000,
        requests_per_user=1000,
        token_capacity=200,            # 200 requests per hour
        refill_rate=0.056,             # 200/3600 = 0.056/sec
        tokens_per_request=1,
        price_per_refill_usd=0.05,     # Hypothetical pricing
        tokens_per_payment=200,
        trust_threshold=3,
        avg_request_interval_ms=50.0,
        load_multiplier=1.0,
        user_patience_ms=999999999.0,  # Users never churn
    )


PLATFORMS = {
    "reddit": ("Reddit", get_reddit_config),
    "x": ("X (Twitter)", get_x_config),
    "instagram": ("Instagram", get_instagram_config),
}


def main():
    """Run cross-platform comparison."""
    print("=" * 60)
    print("CROSS-PLATFORM X402 COMPARISON")
    print("=" * 60)
    print("Comparing X402 performance across social media API rate limits\n")
    
    all_results = {}
    
    for platform, (platform_name, config_fn) in PLATFORMS.items():
        config = config_fn()
        print(f"\n{'='*40}")
        print(f"Platform: {platform_name}")
        print(f"Config: {config.token_capacity} tokens, {config.refill_rate:.3f}/sec refill")
        print(f"Workload: {config.num_users * config.requests_per_user:,} requests")
        print(f"{'='*40}")
        
        results = {}
        for scheme in ["no_x402", "sync", "async"]:
            results[scheme] = simulate_scheme(config, scheme)
            print(results[scheme].summary())
        
        all_results[platform] = results
        print_comparison(results)
    
    # Generate cross-platform comparison chart
    plot_cross_platform_comparison(all_results, "cross_platform_comparison.png")
    
    # Summary insights
    print("\n" + "=" * 60)
    print("📊 CROSS-PLATFORM INSIGHTS")
    print("=" * 60)
    for platform in PLATFORMS:
        sync = all_results[platform]["sync"].total_time_ms
        async_t = all_results[platform]["async"].total_time_ms
        print(f"  {platform}: Async is {sync/async_t:.1f}x faster than Sync")


if __name__ == "__main__":
    main()
