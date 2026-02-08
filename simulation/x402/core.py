"""
X402 Simulation Core Classes
============================

Dataclasses for simulation configuration, user state, and results.
"""

import numpy as np
from dataclasses import dataclass, field
from typing import List


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
    # Sync payment: mean ~3.0s, observed range 1.9s - 5.2s (Verification + Payment)
    sync_latency_mean_ms: float = 3000.0
    sync_latency_std_ms: float = 800.0
    
    # Async payment: mean ~300ms, observed range 250ms - 400ms   (Usually just verification. Payment settled Async)
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
