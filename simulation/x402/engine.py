"""
X402 Simulation Engine
======================

Core simulation logic for comparing payment schemes.
"""

import numpy as np
from .core import SimulationConfig, UserState, SimulationResult


def simulate_scheme(config: SimulationConfig, scheme: str) -> SimulationResult:
    """
    Simulate one payment scheme.
    
    Args:
        config: Simulation configuration
        scheme: One of "no_x402", "sync", "async"
    
    Returns:
        SimulationResult with all metrics
    
    Schemes:
    - "no_x402": Traditional rate limiting (users wait for natural refill)
    - "sync": Synchronous X402 payment (always wait for settlement)
    - "async": Optimistic X402 payment (fast for trusted users)
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
                current_time += 50.0  # Advance time for request processing
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
                    # User waits for natural token refill
                    user.tokens = config.tokens_per_request  # After waiting, user has exactly enough tokens
                    user.tokens -= config.tokens_per_request  # Consume tokens for the request (now 0)
                    result.successful_requests += 1  
                    latencies.append(wait_time)  
                    current_time += wait_time 
                    
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
                latencies.append(payment_latency + 50.0) # Payment + request Procesing. 
                current_time += payment_latency  + 50.0  # Blocking payment wait
                
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
                latencies.append(payment_latency + 50.0)  # Payment + request processing
                current_time += payment_latency + 50.0  # Blocking payment wait
    
    # Calculate statistics
    if latencies:
        result.latencies = latencies
        result.avg_latency_ms = np.mean(latencies)
        result.p50_latency_ms = np.percentile(latencies, 50)
        result.p95_latency_ms = np.percentile(latencies, 95)
        result.p99_latency_ms = np.percentile(latencies, 99)
    
    result.total_time_ms = current_time / config.num_users  # Per-user average
    
    return result


def run_comparison(config: SimulationConfig) -> dict:
    """Run all three schemes and compare."""
    results = {}
    
    for scheme in ["no_x402", "sync", "async"]:
        results[scheme] = simulate_scheme(config, scheme)
        print(results[scheme].summary())
    
    return results


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
