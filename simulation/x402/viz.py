"""
X402 Visualization Functions
=============================

Plotting and analysis output functions.
"""

import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

from .core import SimulationConfig
from .engine import simulate_scheme
from .presets import PRESETS


def compare_presets(preset_names: list = None, load_multiplier: float = 1.0):
    """Compare multiple preset configurations.
    
    Args:
        preset_names: List of preset names to compare (default: all)
        load_multiplier: Multiply request rate by this factor (2.0 = 2x faster requests)
    """
    if preset_names is None:
        preset_names = list(PRESETS.keys())
    
    all_results = {}
    
    for name in preset_names:
        config = PRESETS[name]()
        config.load_multiplier = load_multiplier
        
        print(f"\n{'='*60}")
        print(f"PRESET: {name.upper()}" + (f" (load: {load_multiplier}x)" if load_multiplier != 1.0 else ""))
        print(f"{'='*60}")
        print(f"Capacity: {config.token_capacity}, Refill: {config.refill_rate}/s, Price: ${config.price_per_refill_usd}")
        print(f"Users: {config.num_users}, Requests/user: {config.requests_per_user}")
        print(f"Request interval: {config.effective_request_interval_ms:.0f}ms (base: {config.avg_request_interval_ms:.0f}ms)")
        
        results = {}
        for scheme in ["no_x402", "sync", "async"]:
            results[scheme] = simulate_scheme(config, scheme)
            print(results[scheme].summary())
        
        all_results[name] = results
    
    return all_results


def plot_latency_comparison(results: dict, save_path: str = None):
    """Plot latency distributions for each scheme."""
    fig, axes = plt.subplots(1, 3, figsize=(15, 5))
    
    colors = {"no_x402": "#e74c3c", "sync": "#f39c12", "async": "#27ae60"}
    titles = {"no_x402": "No X402\n(Wait for Refill)", "sync": "Sync X402\n(Pay & Wait)", "async": "Async X402\n(Optimistic)"}
    
    for idx, (scheme, result) in enumerate(results.items()):
        ax = axes[idx]
        if result.latencies:
            ax.hist(result.latencies, bins=50, color=colors[scheme], alpha=0.7, edgecolor='black')
            ax.axvline(result.avg_latency_ms, color='black', linestyle='--', label=f'Avg: {result.avg_latency_ms:.0f}ms')
            ax.axvline(result.p95_latency_ms, color='red', linestyle=':', label=f'P95: {result.p95_latency_ms:.0f}ms')
        ax.set_title(titles[scheme], fontsize=14)
        ax.set_xlabel('Latency (ms)')
        ax.set_ylabel('Count')
        ax.legend()
    
    plt.tight_layout()
    if save_path:
        plt.savefig(save_path, dpi=150)
        print(f"Saved: {save_path}")
    plt.close()


def plot_revenue_comparison(results: dict, save_path: str = None):
    """Plot revenue comparison."""
    fig, ax = plt.subplots(figsize=(10, 6))
    
    schemes = list(results.keys())
    revenues = [results[s].total_revenue for s in schemes]
    payments = [results[s].total_payments for s in schemes]
    
    x = np.arange(len(schemes))
    width = 0.35
    
    bars1 = ax.bar(x - width/2, revenues, width, label='Revenue ($)', color='#27ae60')
    bars2 = ax.bar(x + width/2, [p * 0.001 for p in payments], width, label='Payments (scaled)', color='#3498db')
    
    ax.set_ylabel('Value')
    ax.set_title('Revenue & Payment Comparison')
    ax.set_xticks(x)
    ax.set_xticklabels(['No X402', 'Sync X402', 'Async X402'])
    ax.legend()
    
    for bar, val in zip(bars1, revenues):
        ax.annotate(f'${val:.3f}', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom')
    
    plt.tight_layout()
    if save_path:
        plt.savefig(save_path, dpi=150)
        print(f"Saved: {save_path}")
    plt.close()


def plot_preset_comparison(all_results: dict, save_path: str = None):
    """Create comprehensive comparison chart across presets."""
    fig, axes = plt.subplots(2, 2, figsize=(14, 10))
    
    presets = list(all_results.keys())
    x = np.arange(len(presets))
    width = 0.25
    
    # Revenue comparison
    ax = axes[0, 0]
    for i, scheme in enumerate(["no_x402", "sync", "async"]):
        revenues = [all_results[p][scheme].total_revenue for p in presets]
        ax.bar(x + i*width, revenues, width, label=scheme.replace("_", " ").title())
    ax.set_ylabel('Revenue ($)')
    ax.set_title('Revenue by Preset & Scheme')
    ax.set_xticks(x + width)
    ax.set_xticklabels([p.replace("_", "\n") for p in presets])
    ax.legend()
    
    # Latency comparison
    ax = axes[0, 1]
    for i, scheme in enumerate(["no_x402", "sync", "async"]):
        latencies = [all_results[p][scheme].avg_latency_ms for p in presets]
        ax.bar(x + i*width, latencies, width, label=scheme.replace("_", " ").title())
    ax.set_ylabel('Avg Latency (ms)')
    ax.set_title('Latency by Preset & Scheme')
    ax.set_xticks(x + width)
    ax.set_xticklabels([p.replace("_", "\n") for p in presets])
    ax.legend()
    
    # Success rate comparison
    ax = axes[1, 0]
    for i, scheme in enumerate(["no_x402", "sync", "async"]):
        rates = [all_results[p][scheme].successful_requests / max(1, all_results[p][scheme].total_requests) * 100 
                 for p in presets]
        ax.bar(x + i*width, rates, width, label=scheme.replace("_", " ").title())
    ax.set_ylabel('Success Rate (%)')
    ax.set_title('Success Rate by Preset & Scheme')
    ax.set_xticks(x + width)
    ax.set_xticklabels([p.replace("_", "\n") for p in presets])
    ax.legend()
    ax.set_ylim(90, 100)
    
    # Speedup (Async vs Sync)
    ax = axes[1, 1]
    speedups = []
    for p in presets:
        sync_lat = all_results[p]["sync"].avg_latency_ms
        async_lat = all_results[p]["async"].avg_latency_ms
        speedups.append(sync_lat / max(1, async_lat))
    ax.bar(x, speedups, color='#27ae60')
    ax.axhline(y=1, color='red', linestyle='--', label='No improvement')
    ax.set_ylabel('Speedup (x)')
    ax.set_title('Async vs Sync Speedup')
    ax.set_xticks(x)
    ax.set_xticklabels([p.replace("_", "\n") for p in presets])
    ax.legend()
    
    plt.tight_layout()
    if save_path:
        plt.savefig(save_path, dpi=150)
        print(f"Saved: {save_path}")
    plt.close()


def print_summary_table(all_results: dict):
    """Print a summary table comparing all presets."""
    print("\n" + "=" * 130)
    print("SUMMARY COMPARISON TABLE")
    print("=" * 130)
    
    print(f"\n{'Preset':<12} {'Scheme':<8} {'Requests':>10} {'Duration':>10} {'RPS':>8} {'Revenue':>10} {'Payments':>8} {'Avg Lat':>9} {'P95 Lat':>9} {'Success':>8}")
    print("-" * 130)
    
    for preset_name, results in all_results.items():
        for scheme in ["no_x402", "sync", "async"]:
            r = results[scheme]
            success_rate = r.successful_requests / max(1, r.total_requests) * 100
            print(f"{preset_name:<12} {scheme:<8} {r.total_requests:>10} {r.duration_str:>10} {r.throughput_rps:>7.0f}/s ${r.total_revenue:>9.2f} {r.total_payments:>8} "
                  f"{r.avg_latency_ms:>8.0f}ms {r.p95_latency_ms:>8.0f}ms {success_rate:>7.1f}%")
        print("-" * 130)
