#!/usr/bin/env python3
"""
Paid Tier vs X402 Cost Comparison
==================================

Compare the cost of using platform paid tiers vs X402 micropayments.

Key Question: If you're willing to pay, should you subscribe or use X402?
"""

import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

from x402 import SimulationConfig, simulate_scheme


# =============================================================================
# Platform Configurations - Free and Paid Tiers
# =============================================================================

def get_x_paid_config() -> SimulationConfig:
    """X (Twitter) Basic Tier ($200/mo) - 450 req/15min (30x free)"""
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
        user_patience_ms=999999999.0,
    )


def get_reddit_config() -> SimulationConfig:
    """Reddit API - Same rate for free/paid, paid is $0.24/1000 requests"""
    return SimulationConfig(
        num_users=1000,
        requests_per_user=1000,
        token_capacity=100,
        refill_rate=1.67,
        tokens_per_request=1,
        price_per_refill_usd=0.024,
        tokens_per_payment=100,
        trust_threshold=3,
        avg_request_interval_ms=50.0,
        load_multiplier=1.0,
        user_patience_ms=999999999.0,
    )


def get_instagram_config() -> SimulationConfig:
    """Instagram Graph API - 200 req/hour (no paid tier exists)"""
    return SimulationConfig(
        num_users=1000,
        requests_per_user=1000,
        token_capacity=200,
        refill_rate=0.056,
        tokens_per_request=1,
        price_per_refill_usd=0.05,
        tokens_per_payment=200,
        trust_threshold=3,
        avg_request_interval_ms=50.0,
        load_multiplier=1.0,
        user_patience_ms=999999999.0,
    )


# Platform definitions with monthly subscription costs
PLATFORMS = {
    "X ($200/mo)": {
        "config_fn": get_x_paid_config,
        "monthly_cost": 200,
        "has_paid_tier": True,
    },
    "Reddit": {
        "config_fn": get_reddit_config,
        "monthly_cost": 0,  # Pay per request, not subscription
        "per_request_cost": 0.00024,  # $0.24/1000
        "has_paid_tier": False,
    },
    "Instagram": {
        "config_fn": get_instagram_config,
        "monthly_cost": 0,
        "has_paid_tier": False,  # No paid tier exists
    },
}


def plot_cost_comparison(results: dict, save_path: str = None):
    """Plot cost comparison: Subscription vs X402."""
    fig, axes = plt.subplots(1, 2, figsize=(14, 6))
    
    platforms = list(results.keys())
    x = np.arange(len(platforms))
    width = 0.35
    
    # Extract data
    subscription_costs = []
    x402_costs = []
    times_no_x402 = []
    times_x402 = []
    
    for platform in platforms:
        r = results[platform]
        subscription_costs.append(r["subscription_cost"])
        x402_costs.append(r["x402_cost"])
        times_no_x402.append(r["time_no_x402_mins"])
        times_x402.append(r["time_x402_mins"])
    
    # 1. Cost comparison
    ax = axes[0]
    bars1 = ax.bar(x - width/2, subscription_costs, width, label='Subscription/Traditional', color='#e74c3c')
    bars2 = ax.bar(x + width/2, x402_costs, width, label='X402 Payments', color='#27ae60')
    
    ax.set_ylabel('Cost ($)')
    ax.set_title('Cost Comparison: Subscription vs X402')
    ax.set_xticks(x)
    ax.set_xticklabels(platforms, rotation=15, ha='right')
    ax.legend()
    ax.grid(True, alpha=0.3, axis='y')
    
    # Add value labels
    for bar, cost in zip(bars1, subscription_costs):
        if cost > 0:
            ax.annotate(f'${cost:.0f}', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                       ha='center', va='bottom', fontsize=9)
    for bar, cost in zip(bars2, x402_costs):
        if cost > 0:
            ax.annotate(f'${cost:.0f}', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                       ha='center', va='bottom', fontsize=9)
    
    # 2. Time comparison
    ax = axes[1]
    bars1 = ax.bar(x - width/2, times_no_x402, width, label='Without X402', color='#e74c3c')
    bars2 = ax.bar(x + width/2, times_x402, width, label='With X402 (Async)', color='#27ae60')
    
    ax.set_ylabel('Time (minutes)')
    ax.set_title('Time to Complete (per user)')
    ax.set_xticks(x)
    ax.set_xticklabels(platforms, rotation=15, ha='right')
    ax.legend()
    ax.set_yscale('log')
    ax.grid(True, alpha=0.3, axis='y')
    
    # Add value labels
    for bar, t in zip(bars1, times_no_x402):
        ax.annotate(f'{t:.0f}m', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontsize=8)
    for bar, t in zip(bars2, times_x402):
        ax.annotate(f'{t:.0f}m', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontsize=8)
    
    plt.tight_layout()
    if save_path:
        plt.savefig(save_path, dpi=150)
        print(f"\nSaved: {save_path}")
    plt.close()


def main():
    """Run cost comparison experiment."""
    print("=" * 70)
    print("PAID TIER vs X402 COST COMPARISON")
    print("=" * 70)
    print("Question: If you're willing to pay, should you subscribe or use X402?\n")
    
    all_results = {}
    
    for platform_name, platform_info in PLATFORMS.items():
        config = platform_info["config_fn"]()
        total_requests = config.num_users * config.requests_per_user
        
        print(f"\n{'='*50}")
        print(f"Platform: {platform_name}")
        print(f"Rate Limit: {config.token_capacity} tokens, {config.refill_rate:.3f}/sec refill")
        print(f"Workload: {total_requests:,} requests")
        print(f"{'='*50}")
        
        # Run simulations
        no_x402 = simulate_scheme(config, "no_x402")
        async_x402 = simulate_scheme(config, "async")
        
        print(f"\n  Without X402: {no_x402.total_time_ms/1000/60:.1f} minutes")
        print(f"  With X402:    {async_x402.total_time_ms/1000/60:.1f} minutes")
        print(f"  X402 Cost:    ${async_x402.total_revenue:.2f} ({async_x402.total_payments} payments)")
        
        # Calculate subscription cost
        subscription_cost = platform_info["monthly_cost"]
        if "per_request_cost" in platform_info:
            # Reddit charges per request
            subscription_cost = total_requests * platform_info["per_request_cost"]
            print(f"  Reddit API Cost: ${subscription_cost:.2f} ($0.24/1000 requests)")
        elif subscription_cost > 0:
            print(f"  Subscription: ${subscription_cost}/month")
        
        all_results[platform_name] = {
            "subscription_cost": subscription_cost,
            "x402_cost": async_x402.total_revenue,
            "time_no_x402_mins": no_x402.total_time_ms / 1000 / 60,
            "time_x402_mins": async_x402.total_time_ms / 1000 / 60,
            "total_requests": total_requests,
        }
    
    # Generate comparison chart
    plot_cost_comparison(all_results, "cost_comparison.png")
    
    # Print summary table
    print("\n" + "=" * 70)
    print("📊 COST COMPARISON SUMMARY")
    print("=" * 70)
    print(f"{'Platform':<20} {'Subscription':<15} {'X402 Cost':<15} {'Savings':<15}")
    print("-" * 70)
    
    for platform, data in all_results.items():
        sub = data["subscription_cost"]
        x402 = data["x402_cost"]
        if sub > 0 and x402 > 0:
            savings = sub - x402
            pct = (savings / sub) * 100 if sub > 0 else 0
            savings_str = f"${savings:.0f} ({pct:.0f}%)" if savings > 0 else f"-${-savings:.0f}"
        else:
            savings_str = "N/A"
        print(f"{platform:<20} ${sub:<14.2f} ${x402:<14.2f} {savings_str}")
    
    print("=" * 70)


if __name__ == "__main__":
    main()
