"""
X402 Rate Limiter Simulation
============================

Compare three payment schemes:
1. No X402 (traditional rate limiting - users just wait)
2. Sync X402 (synchronous payment settlement)
3. Async X402 (optimistic payment for trusted clients)

Analyze: Performance, Pricing, Revenue
"""

import numpy as np
import matplotlib
matplotlib.use('Agg')  # Non-interactive backend for saving plots
import matplotlib.pyplot as plt
from dataclasses import dataclass, field
from typing import List, Tuple
import json
import os

# =============================================================================
# CONFIGURATION - Tweak these parameters!
# =============================================================================

@dataclass
class SimulationConfig:
    """All configurable parameters for the simulation."""
    
    # --- Token Bucket Parameters ---
    token_capacity: int = 4               # Max tokens per client
    refill_rate: float = 1.0              # Tokens per second (slower = more payments)
    tokens_per_request: int = 1           # Tokens consumed per request
    
    # --- Payment Parameters ---
    price_per_refill_usd: float = 0.001   # Price in USD per capacity refill
    tokens_per_payment: int = 4           # Tokens added per payment
    
    # --- Latency Distributions (from real benchmarks) ---
    # Sync payment: mean ~3.0s, observed range 1.9s - 5.2s
    sync_latency_mean_ms: float = 3000.0
    sync_latency_std_ms: float = 800.0
    
    # Async payment: mean ~300ms, observed range 250ms - 400ms  
    async_latency_mean_ms: float = 300.0
    async_latency_std_ms: float = 50.0
    
    # --- Trust Parameters ---
    trust_threshold: int = 3              # Payments to become trusted
    trust_window_hours: float = 1.0       # Time window for trust
    
    # --- User Behavior ---
    num_users: int = 100                  # Concurrent users
    requests_per_user: int = 50           # Requests each user makes
    user_patience_ms: float = 5000.0      # Users leave if wait > this
    user_retry_probability: float = 0.9   # Probability user retries after 402
    
    # --- Time Simulation ---
    avg_request_interval_ms: float = 200.0  # Base interval (before load multiplier)
    
    # --- Load Scaling ---
    load_multiplier: float = 1.0          # Multiplier for request rate (2.0 = 2x faster requests)
    
    # --- Settlement Failures (for async) ---
    settlement_failure_rate: float = 0.02   # 2% of settlements fail
    
    @property
    def effective_request_interval_ms(self) -> float:
        """Request interval after applying load multiplier."""
        return self.avg_request_interval_ms / self.load_multiplier
    
    def to_dict(self):
        return {k: v for k, v in self.__dict__.items()}


# =============================================================================
# REAL-WORLD API RATE LIMIT PRESETS
# =============================================================================

def config_openai() -> SimulationConfig:
    """
    OpenAI API Rate Limits (as of 2026)
    - Standard tier: 500 RPM (~8.3 req/sec)
    - Token limits: 30,000 - 500,000 TPM depending on model
    - gpt-4o-realtime-preview: 200 RPM, 1,000 RPD
    
    Source: https://platform.openai.com/docs/guides/rate-limits
    """
    return SimulationConfig(
        token_capacity=50,            # ~500 RPM / 10 = 50 burst (1 min bucket)
        refill_rate=8.3,              # 500 RPM = 8.3 req/sec sustained
        tokens_per_request=1,
        price_per_refill_usd=0.03,    # ~$0.03 per 1K tokens (varies by model)
        tokens_per_payment=50,
        num_users=1000,
        requests_per_user=100,
        avg_request_interval_ms=500,  # AI API calls are typically slower
    )

def config_stripe() -> SimulationConfig:
    """
    Stripe API Rate Limits
    - Default: 100 requests/second in live mode
    - Payment Intents: 1,000 updates/hour per PaymentIntent
    
    Source: https://docs.stripe.com/rate-limits
    """
    return SimulationConfig(
        token_capacity=100,           # 100 req/sec burst
        refill_rate=100.0,            # Sustained 100 req/sec
        tokens_per_request=1,
        price_per_refill_usd=0.10,    # Higher value (payment processing)
        tokens_per_payment=100,
        num_users=1000,
        requests_per_user=500,        # High volume per merchant
        avg_request_interval_ms=10,   # Fast interval for high RPS
    )


def config_github() -> SimulationConfig:
    """
    GitHub REST API Rate Limits
    - Unauthenticated: 60 req/hour
    - Authenticated: 5,000 req/hour
    - GitHub Apps: 5,000-12,500 req/hour (scales with repos/users)
    
    Source: https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
    """
    return SimulationConfig(
        token_capacity=83,            # 5000/hour = ~83/min = ~1.4/sec burst
        refill_rate=1.4,              # Sustained ~1.4 req/sec
        tokens_per_request=1,
        price_per_refill_usd=0.01,    # Developer tools pricing
        tokens_per_payment=83,
        num_users=1000,
        requests_per_user=200,
        avg_request_interval_ms=500,  # Developer API usage pattern
    )


def config_twitter() -> SimulationConfig:
    """
    X (Twitter) API Rate Limits
    - Basic: 15,000 tweets/month write, 10,000 read
    - Pro ($5k/mo): 300,000 tweets/month
    - ~300 requests per 15 minutes for reads
    
    Source: https://developer.x.com/en/docs/x-api/rate-limits
    """
    return SimulationConfig(
        token_capacity=20,            # 300/15min = 20/min burst
        refill_rate=0.33,             # ~20 per minute = 0.33/sec
        tokens_per_request=1,
        price_per_refill_usd=0.05,    # Social API premium pricing
        tokens_per_payment=20,
        num_users=1000,
        requests_per_user=100,
        avg_request_interval_ms=300,  # Social app polling pattern
    )


def config_cloudflare() -> SimulationConfig:
    """
    Cloudflare API Rate Limits
    - Default: 1,200 requests per 5 minutes = 4 req/sec
    - Enterprise: Higher limits available
    
    Source: https://developers.cloudflare.com/fundamentals/api/reference/limits/
    """
    return SimulationConfig(
        token_capacity=40,            # Burst capacity
        refill_rate=4.0,              # 4 req/sec sustained
        tokens_per_request=1,
        price_per_refill_usd=0.02,
        tokens_per_payment=40,
        num_users=1000,
        requests_per_user=150,
        avg_request_interval_ms=250,
    )


# All available presets (real-world services)
PRESETS = {
    "openai": config_openai,
    "stripe": config_stripe,
    "github": config_github,
    "twitter": config_twitter,
    "cloudflare": config_cloudflare,
}


# =============================================================================
# SIMULATION ENGINE
# =============================================================================

@dataclass
class UserState:
    """Track individual user state."""
    tokens: float
    last_refill_time: float
    payments_made: int = 0
    trust_level: int = 0
    is_trusted: bool = False
    total_wait_time_ms: float = 0.0
    successful_requests: int = 0
    failed_requests: int = 0
    revenue_generated: float = 0.0
    churned: bool = False


@dataclass
class SimulationResult:
    """Results from running a simulation."""
    scheme: str
    total_requests: int = 0
    successful_requests: int = 0
    failed_requests: int = 0
    total_payments: int = 0
    total_revenue: float = 0.0
    avg_latency_ms: float = 0.0
    p50_latency_ms: float = 0.0
    p95_latency_ms: float = 0.0
    p99_latency_ms: float = 0.0
    churned_users: int = 0
    total_time_ms: float = 0.0  # Total simulated time
    latencies: List[float] = field(default_factory=list)
    
    @property
    def throughput_rps(self) -> float:
        """Requests per second (throughput)."""
        if self.total_time_ms <= 0:
            return 0.0
        return self.successful_requests / (self.total_time_ms / 1000.0)
    
    @property
    def duration_str(self) -> str:
        """Human-readable duration."""
        secs = self.total_time_ms / 1000.0
        if secs < 60:
            return f"{secs:.1f}s"
        elif secs < 3600:
            return f"{secs/60:.1f}m"
        else:
            return f"{secs/3600:.1f}h"
    
    def summary(self) -> str:
        return f"""
=== {self.scheme} ===
Requests: {self.successful_requests}/{self.total_requests} successful ({100*self.successful_requests/max(1,self.total_requests):.1f}%)
Payments: {self.total_payments}
Revenue: ${self.total_revenue:.4f}
Latency: avg={self.avg_latency_ms:.0f}ms, p50={self.p50_latency_ms:.0f}ms, p95={self.p95_latency_ms:.0f}ms
Duration: {self.duration_str} ({self.throughput_rps:.1f} req/s)
Churned: {self.churned_users} users ({100*self.churned_users/max(1, self.total_requests//50):.1f}%)
"""


def simulate_scheme(config: SimulationConfig, scheme: str) -> SimulationResult:
    """
    Simulate one payment scheme.
    
    Schemes:
    - "no_x402": Traditional rate limiting (users wait for natural refill)
    - "sync": Synchronous X402 payment
    - "async": Optimistic X402 payment for trusted users
    """
    np.random.seed(1399)  # Reproducibility
    
    result = SimulationResult(scheme=scheme)
    latencies = []
    
    # Initialize users
    users = [
        UserState(tokens=config.token_capacity, last_refill_time=0.0)
        for _ in range(config.num_users)
    ]
    
    current_time = 0.0
    
    for user_idx, user in enumerate(users):
        if user.churned:
            continue
            
        for req_num in range(config.requests_per_user):
            result.total_requests += 1
            
            # Simulate time passing (natural refill)
            time_delta = np.random.exponential(config.effective_request_interval_ms)
            current_time += time_delta
            
            # Natural token refill
            refill_amount = (time_delta / 1000.0) * config.refill_rate
            user.tokens = min(config.token_capacity, user.tokens + refill_amount)
            
            # Try to consume token
            if user.tokens >= config.tokens_per_request:
                # Success - have tokens
                user.tokens -= config.tokens_per_request
                result.successful_requests += 1
                user.successful_requests += 1
                latencies.append(50.0)  # Normal request latency ~50ms
                continue
            
            # Rate limited! What happens next depends on scheme
            if scheme == "no_x402":
                # Traditional: User waits for natural refill or leaves
                wait_time = (config.tokens_per_request - user.tokens) / config.refill_rate * 1000
                
                if wait_time > config.user_patience_ms:
                    # User churns
                    result.failed_requests += 1
                    user.churned = True
                    result.churned_users += 1
                    break
                else:
                    # User waits
                    user.tokens = config.tokens_per_request
                    user.tokens -= config.tokens_per_request
                    result.successful_requests += 1
                    latencies.append(wait_time)
                    current_time += wait_time  # Blocking wait
                    
            elif scheme == "sync":
                # Sync X402: Pay and wait for settlement
                if np.random.random() > config.user_retry_probability:
                    result.failed_requests += 1
                    continue
                
                # Payment latency (sync)
                payment_latency = max(500, np.random.normal(
                    config.sync_latency_mean_ms, 
                    config.sync_latency_std_ms
                ))
                
                if payment_latency > config.user_patience_ms * 2:  # More patient for paid
                    result.failed_requests += 1
                    user.churned = True
                    result.churned_users += 1
                    break
                
                # Payment successful
                user.tokens = config.tokens_per_payment
                user.tokens -= config.tokens_per_request
                user.payments_made += 1
                user.trust_level += 1
                result.total_payments += 1
                result.total_revenue += config.price_per_refill_usd
                result.successful_requests += 1
                latencies.append(payment_latency)
                current_time += payment_latency  # Blocking payment wait
                
            elif scheme == "async":
                # Async X402: Trusted users get fast response
                if np.random.random() > config.user_retry_probability:
                    result.failed_requests += 1
                    continue
                
                # Check if trusted
                is_trusted = user.trust_level >= config.trust_threshold
                
                if is_trusted:
                    # Fast optimistic payment
                    payment_latency = max(150, np.random.normal(
                        config.async_latency_mean_ms,
                        config.async_latency_std_ms
                    ))
                    
                    # Check for settlement failure
                    if np.random.random() < config.settlement_failure_rate:
                        # Settlement failed - lose trust
                        user.trust_level = 0
                        # But user still got their request through!
                else:
                    # Not trusted yet - sync payment to build trust
                    payment_latency = max(500, np.random.normal(
                        config.sync_latency_mean_ms,
                        config.sync_latency_std_ms
                    ))
                
                if payment_latency > config.user_patience_ms * 2:
                    result.failed_requests += 1
                    user.churned = True
                    result.churned_users += 1
                    break
                
                # Payment successful
                user.tokens = config.tokens_per_payment
                user.tokens -= config.tokens_per_request
                user.payments_made += 1
                user.trust_level += 1
                result.total_payments += 1
                result.total_revenue += config.price_per_refill_usd
                result.successful_requests += 1
                latencies.append(payment_latency)
                current_time += payment_latency  # Blocking payment wait
    
    # Calculate statistics
    if latencies:
        result.latencies = latencies
        result.avg_latency_ms = np.mean(latencies)
        result.p50_latency_ms = np.percentile(latencies, 50)
        result.p95_latency_ms = np.percentile(latencies, 95)
        result.p99_latency_ms = np.percentile(latencies, 99)
    
    result.total_time_ms = current_time
    
    return result


# =============================================================================
# ANALYSIS & VISUALIZATION
# =============================================================================

def run_comparison(config: SimulationConfig) -> dict:
    """Run all three schemes and compare."""
    results = {}
    
    for scheme in ["no_x402", "sync", "async"]:
        results[scheme] = simulate_scheme(config, scheme)
        print(results[scheme].summary())
    
    return results


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
    
    # Add value labels
    for bar, val in zip(bars1, revenues):
        ax.annotate(f'${val:.3f}', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                   ha='center', va='bottom')
    
    plt.tight_layout()
    if save_path:
        plt.savefig(save_path, dpi=150)
        print(f"Saved: {save_path}")
    plt.close()


def sensitivity_analysis(base_config: SimulationConfig, param_name: str, values: list) -> dict:
    """Run sensitivity analysis on a parameter."""
    results = {scheme: [] for scheme in ["no_x402", "sync", "async"]}
    
    for val in values:
        config = SimulationConfig(**base_config.to_dict())
        setattr(config, param_name, val)
        
        for scheme in ["no_x402", "sync", "async"]:
            result = simulate_scheme(config, scheme)
            results[scheme].append({
                "param_value": val,
                "revenue": result.total_revenue,
                "avg_latency": result.avg_latency_ms,
                "success_rate": result.successful_requests / max(1, result.total_requests),
                "churn_rate": result.churned_users / max(1, result.total_requests // 50)
            })
    
    return results


# =============================================================================
# MAIN - Run this!
# =============================================================================

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


if __name__ == "__main__":
    import sys
    
    # Check for command line argument - skip if running in Jupyter
    if len(sys.argv) > 1 and "jupyter" in sys.argv[1]:
        pass  # Running in notebook - do nothing, let user call functions directly
    elif len(sys.argv) > 1:
        if sys.argv[1] == "all":
            # Compare all presets
            print("=" * 60)
            print("X402 Rate Limiter Simulation - ALL PRESETS")
            print("=" * 60)
            
            all_results = compare_presets()
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
