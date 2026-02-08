#!/usr/bin/env python3
"""
LinkedIn-Ready X402 Visualizations
===================================

Professional, colorblind-friendly graphs for LinkedIn posts.
"""

import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib import rcParams

from x402 import SimulationConfig, simulate_scheme

# =============================================================================
# Professional Styling
# =============================================================================

# Set2 palette (colorblind-friendly)
COLORS = {
    'traditional': '#FC8D62',  # Coral/Orange
    'sync': '#66C2A5',         # Teal
    'async': '#8DA0CB',        # Purple/Blue
}

# Hatching patterns for additional accessibility
HATCHES = {
    'traditional': '',
    'sync': '///',
    'async': '...',
}

def setup_style():
    """Configure professional matplotlib style."""
    rcParams['font.family'] = 'sans-serif'
    rcParams['font.sans-serif'] = ['DejaVu Sans', 'Arial', 'Helvetica']
    rcParams['font.size'] = 11
    rcParams['axes.titlesize'] = 14
    rcParams['axes.titleweight'] = 'bold'
    rcParams['axes.labelsize'] = 12
    rcParams['axes.labelweight'] = 'medium'
    rcParams['axes.spines.top'] = False
    rcParams['axes.spines.right'] = False
    rcParams['figure.facecolor'] = 'white'
    rcParams['axes.facecolor'] = 'white'
    rcParams['axes.grid'] = True
    rcParams['grid.alpha'] = 0.3
    rcParams['grid.linestyle'] = '--'


# =============================================================================
# Platform Configurations
# =============================================================================

def get_reddit_config():
    return SimulationConfig(
        num_users=1000, requests_per_user=1000,
        token_capacity=100, refill_rate=1.67,
        tokens_per_request=1, price_per_refill_usd=0.024,
        tokens_per_payment=100, trust_threshold=3,
        avg_request_interval_ms=50.0, load_multiplier=1.0,
        user_patience_ms=999999999.0,
    )

def get_x_paid_config():
    return SimulationConfig(
        num_users=1000, requests_per_user=1000,
        token_capacity=450, refill_rate=0.5,
        tokens_per_request=1, price_per_refill_usd=0.10,
        tokens_per_payment=450, trust_threshold=3,
        avg_request_interval_ms=50.0, load_multiplier=1.0,
        user_patience_ms=999999999.0,
    )

def get_instagram_config():
    return SimulationConfig(
        num_users=1000, requests_per_user=1000,
        token_capacity=200, refill_rate=0.056,
        tokens_per_request=1, price_per_refill_usd=0.05,
        tokens_per_payment=200, trust_threshold=3,
        avg_request_interval_ms=50.0, load_multiplier=1.0,
        user_patience_ms=999999999.0,
    )

def get_bursty_config():
    """X free tier with bursty load."""
    return SimulationConfig(
        num_users=100, requests_per_user=1000,
        token_capacity=15, refill_rate=0.017,
        tokens_per_request=1, price_per_refill_usd=0.10,
        tokens_per_payment=15, trust_threshold=3,
        avg_request_interval_ms=10.0, load_multiplier=1.0,
        user_patience_ms=999999999.0,
    )


# =============================================================================
# Image 1: X402 vs Traditional Rate Limiting
# =============================================================================

def create_image1():
    """X402 vs Traditional Rate Limiting across platforms."""
    setup_style()
    
    print("Generating Image 1: X402 vs Traditional Rate Limiting...")
    
    # Run simulations
    platforms = {
        'Reddit': get_reddit_config(),
        'X ($200/mo)': get_x_paid_config(),
        'Instagram': get_instagram_config(),
    }
    
    subscription_costs = {
        'Reddit': 240.0,      # $0.24/1000 * 1M
        'X ($200/mo)': 200.0,
        'Instagram': 0.0,     # No paid tier
    }
    
    results = {}
    for name, config in platforms.items():
        print(f"  Running {name}...")
        results[name] = {
            'no_x402': simulate_scheme(config, 'no_x402'),
            'async': simulate_scheme(config, 'async'),
        }
    
    # Create figure
    fig, axes = plt.subplots(1, 2, figsize=(12, 5))
    fig.suptitle('X402 vs Traditional Rate Limiting\n(1 Million API Requests)', 
                 fontsize=16, fontweight='bold', y=1.02)
    
    platform_names = list(platforms.keys())
    x = np.arange(len(platform_names))
    width = 0.35
    
    # Left: Time Comparison (in minutes)
    ax = axes[0]
    trad_times = [results[p]['no_x402'].total_time_ms/1000/60 for p in platform_names]  # minutes
    x402_times = [results[p]['async'].total_time_ms/1000/60 for p in platform_names]  # minutes
    
    bars1 = ax.bar(x - width/2, trad_times, width, label='Traditional', 
                   color=COLORS['traditional'], hatch=HATCHES['traditional'], edgecolor='black', linewidth=0.5)
    bars2 = ax.bar(x + width/2, x402_times, width, label='X402 (Async)', 
                   color=COLORS['async'], hatch=HATCHES['async'], edgecolor='black', linewidth=0.5)
    
    ax.set_ylabel('Time (minutes)')
    ax.set_title('Time to Complete (per user)')
    ax.set_xticks(x)
    ax.set_xticklabels(platform_names)
    ax.set_yscale('log')
    ax.legend(loc='upper left')
    
    # Add labels
    for bar, t in zip(bars1, trad_times):
        ax.annotate(f'{t:.0f}m', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontsize=9, fontweight='bold')
    for bar, t in zip(bars2, x402_times):
        ax.annotate(f'{t:.0f}m', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontsize=9, fontweight='bold')
    ax.margins(y=0.2)
    
    # Right: Cost Comparison
    ax = axes[1]
    trad_costs = [subscription_costs[p] for p in platform_names]
    x402_costs = [results[p]['async'].total_revenue for p in platform_names]
    
    bars1 = ax.bar(x - width/2, trad_costs, width, label='Subscription', 
                   color=COLORS['traditional'], hatch=HATCHES['traditional'], edgecolor='black', linewidth=0.5)
    bars2 = ax.bar(x + width/2, x402_costs, width, label='X402 Pay-per-Use', 
                   color=COLORS['async'], hatch=HATCHES['async'], edgecolor='black', linewidth=0.5)
    
    ax.set_ylabel('Cost ($)')
    ax.set_title('Cost Comparison')
    ax.set_xticks(x)
    ax.set_xticklabels(platform_names)
    ax.legend(loc='upper right')
    
    # Add labels
    for bar, c in zip(bars1, trad_costs):
        if c > 0:
            ax.annotate(f'${c:.0f}', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                       ha='center', va='bottom', fontsize=9, fontweight='bold')
        else:
            ax.annotate('N/A', xy=(bar.get_x() + bar.get_width()/2, 10),
                       ha='center', va='bottom', fontsize=9, fontweight='bold', color='gray')
    for bar, c in zip(bars2, x402_costs):
        ax.annotate(f'${c:.0f}', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontsize=9, fontweight='bold')
    ax.margins(y=0.2)
    
    plt.tight_layout()
    plt.savefig('linkedin_image1.png', dpi=150, bbox_inches='tight', 
                facecolor='white', edgecolor='none')
    print("  Saved: linkedin_image1.png")
    plt.close()


# =============================================================================
# Image 2: When Async X402 Shines
# =============================================================================

def create_image2():
    """Bursty load comparison showing async advantage."""
    setup_style()
    
    print("\nGenerating Image 2: When Async X402 Shines...")
    
    config = get_bursty_config()
    
    print("  Running simulations...")
    results = {
        'no_x402': simulate_scheme(config, 'no_x402'),
        'sync': simulate_scheme(config, 'sync'),
        'async': simulate_scheme(config, 'async'),
    }
    
    # Create figure
    fig, axes = plt.subplots(1, 2, figsize=(12, 5))
    payments = results['sync'].total_payments
    fig.suptitle(f'When Async X402 Shines: Bursty Traffic\n({payments:,} payments triggered | 100K requests)', 
                 fontsize=16, fontweight='bold', y=1.02)
    
    schemes = ['No X402', 'Sync X402', 'Async X402']
    scheme_keys = ['no_x402', 'sync', 'async']
    colors = [COLORS['traditional'], COLORS['sync'], COLORS['async']]
    hatches = [HATCHES['traditional'], HATCHES['sync'], HATCHES['async']]
    
    # Left: Time Comparison (in minutes)
    ax = axes[0]
    times = [results[k].total_time_ms/1000/60 for k in scheme_keys]  # minutes
    costs = [results[k].total_revenue for k in scheme_keys]
    
    bars = ax.bar(schemes, times, color=colors, edgecolor='black', linewidth=0.5)
    for bar, h in zip(bars, hatches):
        bar.set_hatch(h)
    
    ax.set_ylabel('Time (minutes)')
    ax.set_title('Time to Complete (per user)')
    ax.set_yscale('log')
    
    # Add labels with cost
    for bar, t, c in zip(bars, times, costs):
        label = f'{t:.1f}m\n(${c:.0f})' if c > 0 else f'{t:.0f}m\n($0)'
        ax.annotate(label, xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontsize=10, fontweight='bold')
    ax.margins(y=0.25)
    
    # Add speedup annotation
    speedup = results['sync'].total_time_ms / results['async'].total_time_ms
    ax.annotate(f'{speedup:.1f}x\nfaster', xy=(2, times[2]), xytext=(2.5, times[1]),
               fontsize=12, fontweight='bold', color=COLORS['async'],
               arrowprops=dict(arrowstyle='->', color=COLORS['async'], lw=2))
    
    # Right: P95 Latency
    ax = axes[1]
    p95s = [results[k].p95_latency_ms for k in scheme_keys]
    
    bars = ax.bar(schemes, p95s, color=colors, edgecolor='black', linewidth=0.5)
    for bar, h in zip(bars, hatches):
        bar.set_hatch(h)
    
    ax.set_ylabel('P95 Latency (ms)')
    ax.set_title('P95 Latency (Lower is Better)')
    ax.set_yscale('log')
    
    # Add labels
    for bar, p in zip(bars, p95s):
        label = f'{p/1000:.0f}s' if p >= 1000 else f'{p:.0f}ms'
        ax.annotate(label, xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom', fontsize=10, fontweight='bold')
    ax.margins(y=0.2)
    
    # Add improvement annotation
    improvement = results['sync'].p95_latency_ms / results['async'].p95_latency_ms
    ax.annotate(f'{improvement:.0f}x\nbetter', xy=(2, p95s[2]), xytext=(2.5, p95s[1]),
               fontsize=12, fontweight='bold', color=COLORS['async'],
               arrowprops=dict(arrowstyle='->', color=COLORS['async'], lw=2))
    
    plt.tight_layout()
    plt.savefig('linkedin_image2.png', dpi=150, bbox_inches='tight',
                facecolor='white', edgecolor='none')
    print("  Saved: linkedin_image2.png")
    plt.close()


# =============================================================================
# Main
# =============================================================================

def main():
    print("=" * 60)
    print("GENERATING LINKEDIN VISUALIZATIONS")
    print("=" * 60)
    
    create_image1()
    create_image2()
    
    print("\n" + "=" * 60)
    print("✅ Done! Created:")
    print("  1. linkedin_image1.png - X402 vs Traditional")
    print("  2. linkedin_image2.png - When Async Shines")
    print("=" * 60)


if __name__ == "__main__":
    main()
