"""
X402 Simulation Package
=======================

Modular simulation framework for comparing X402 payment schemes.

Usage:
    from x402 import SimulationConfig, simulate_scheme, PRESETS
    
    # Run with a preset
    config = PRESETS["openai"]()
    result = simulate_scheme(config, "async")
    print(result.summary())
    
    # Custom configuration
    config = SimulationConfig(
        token_capacity=10,
        refill_rate=0.5,
        trust_threshold=2,
    )
    result = simulate_scheme(config, "sync")
"""

# Core classes
from .core import SimulationConfig, UserState, SimulationResult

# Simulation engine
from .engine import simulate_scheme, run_comparison, sensitivity_analysis

# Presets
from .presets import (
    PRESETS,
    config_openai,
    config_stripe,
    config_github,
    config_twitter,
    config_cloudflare,
)

# Visualization
from .viz import (
    compare_presets,
    plot_latency_comparison,
    plot_revenue_comparison,
    plot_preset_comparison,
    print_summary_table,
)

__all__ = [
    # Core
    "SimulationConfig",
    "UserState", 
    "SimulationResult",
    # Engine
    "simulate_scheme",
    "run_comparison",
    "sensitivity_analysis",
    # Presets
    "PRESETS",
    "config_openai",
    "config_stripe",
    "config_github",
    "config_twitter",
    "config_cloudflare",
    # Viz
    "compare_presets",
    "plot_latency_comparison",
    "plot_revenue_comparison",
    "plot_preset_comparison",
    "print_summary_table",
]
