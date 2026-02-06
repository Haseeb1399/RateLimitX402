#!/usr/bin/env python3
"""
X402 Rate Limiter Simulation - CLI
==================================

Compare three payment schemes:
1. No X402 (traditional rate limiting - users just wait)
2. Sync X402 (synchronous payment settlement)
3. Async X402 (optimistic payment for trusted clients)

Usage:
    python x402_simulation.py all                # Compare all presets
    python x402_simulation.py <preset_name>      # Run single preset
"""

import sys
import json

from x402 import (
    SimulationConfig,
    PRESETS,
    run_comparison,
    compare_presets,
    print_summary_table,
    plot_preset_comparison,
    plot_latency_comparison,
    plot_revenue_comparison,
)


def main():
    """CLI entrypoint."""
    # Check for command line argument
    if len(sys.argv) > 1 and "jupyter" in sys.argv[1]:
        pass  # Running in notebook
    elif len(sys.argv) > 1:
        if sys.argv[1] == "all":
            # Compare all presets
            print("=" * 60)
            print("X402 Rate Limiter Simulation - ALL PRESETS")
            print("=" * 60)
            
            all_results = compare_presets(load_multiplier=15)
            print_summary_table(all_results)
            
            print("\nGenerating comparison plots...")
            plot_preset_comparison(all_results, "preset_comparison.png")
            
        elif sys.argv[1] in PRESETS:
            # Run single preset
            preset_name = sys.argv[1]
            config = PRESETS[preset_name]()
            
            print("=" * 60)
            print(f"X402 Rate Limiter Simulation - {preset_name.upper()}")
            print("=" * 60)
            print(f"\nConfiguration:")
            print(json.dumps(config.to_dict(), indent=2))
            print("\n")
            
            results = run_comparison(config)
            plot_latency_comparison(results, f"{preset_name}_latency.png")
            plot_revenue_comparison(results, f"{preset_name}_revenue.png")
        else:
            print(f"Unknown preset: {sys.argv[1]}")
            print(f"Available presets: {list(PRESETS.keys())}")
    else:
        # Default: show available options and run all
        print("=" * 60)
        print("X402 Rate Limiter Simulation")
        print("=" * 60)
        print("\nUsage:")
        print("  python x402_simulation.py all                # Compare all presets")
        print("  python x402_simulation.py <preset_name>      # Run single preset")
        print("\nAvailable presets:")
        for name, func in PRESETS.items():
            config = func()
            print(f"  {name:<15} - Capacity: {config.token_capacity:>3}, "
                  f"Refill: {config.refill_rate:>4}/s, Price: ${config.price_per_refill_usd}")
        
        print("\nRunning 'all' by default...")
        print()
        
        all_results = compare_presets(load_multiplier=15)
        print_summary_table(all_results)
        plot_preset_comparison(all_results, "preset_comparison.png")


if __name__ == "__main__":
    main()
