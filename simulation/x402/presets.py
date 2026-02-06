"""
X402 Real-World API Presets
===========================

Rate limit configurations based on real-world APIs.
"""

from .core import SimulationConfig


def config_openai() -> SimulationConfig:
    """
    OpenAI API Rate Limits (as of 2026)
    - Standard tier: 500 RPM (~8.3 req/sec)
    - Token limits: 30,000 - 500,000 TPM depending on model
    
    Source: https://platform.openai.com/docs/guides/rate-limits
    """
    return SimulationConfig(
        token_capacity=50,
        refill_rate=8.3,
        tokens_per_request=1,
        price_per_refill_usd=0.03,
        tokens_per_payment=50,
        num_users=1000,
        requests_per_user=100,
        avg_request_interval_ms=500,
    )


def config_stripe() -> SimulationConfig:
    """
    Stripe API Rate Limits
    - Default: 100 requests/second in live mode
    
    Source: https://docs.stripe.com/rate-limits
    """
    return SimulationConfig(
        token_capacity=100,
        refill_rate=100.0,
        tokens_per_request=1,
        price_per_refill_usd=0.10,
        tokens_per_payment=100,
        num_users=1000,
        requests_per_user=500,
        avg_request_interval_ms=10,
    )


def config_github() -> SimulationConfig:
    """
    GitHub REST API Rate Limits
    - Authenticated: 5,000 req/hour
    
    Source: https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
    """
    return SimulationConfig(
        token_capacity=83,
        refill_rate=1.4,
        tokens_per_request=1,
        price_per_refill_usd=0.01,
        tokens_per_payment=83,
        num_users=1000,
        requests_per_user=200,
        avg_request_interval_ms=500,
    )


def config_twitter() -> SimulationConfig:
    """
    X (Twitter) API Rate Limits
    - ~300 requests per 15 minutes for reads
    
    Source: https://developer.x.com/en/docs/x-api/rate-limits
    """
    return SimulationConfig(
        token_capacity=20,
        refill_rate=0.33,
        tokens_per_request=1,
        price_per_refill_usd=0.05,
        tokens_per_payment=20,
        num_users=1000,
        requests_per_user=100,
        avg_request_interval_ms=300,
    )


def config_cloudflare() -> SimulationConfig:
    """
    Cloudflare API Rate Limits
    - Default: 1,200 requests per 5 minutes = 4 req/sec
    
    Source: https://developers.cloudflare.com/fundamentals/api/reference/limits/
    """
    return SimulationConfig(
        token_capacity=40,
        refill_rate=4.0,
        tokens_per_request=1,
        price_per_refill_usd=0.02,
        tokens_per_payment=40,
        num_users=1000,
        requests_per_user=150,
        avg_request_interval_ms=250,
    )


# All available presets
PRESETS = {
    "openai": config_openai,
    "stripe": config_stripe,
    "github": config_github,
    "twitter": config_twitter,
    "cloudflare": config_cloudflare,
}
