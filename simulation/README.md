# X402 Simulation

Simulation framework for comparing X402 payment schemes against traditional rate limiting.

## Schemes Compared

| Scheme | Description |
|--------|-------------|
| **no_x402** | Traditional rate limiting (users wait for token refill) |
| **sync** | Synchronous X402 payment (~3s settlement) |
| **async** | Optimistic X402 payment (~300ms for trusted users) |

## Quick Start

### CLI
```bash
python3 x402_simulation.py              # Run all presets
python3 x402_simulation.py twitter      # Single preset
```

### Python
```python
from x402 import SimulationConfig, simulate_scheme, PRESETS

# Using a preset
config = PRESETS["twitter"]()
result = simulate_scheme(config, "async")
print(result.summary())

# Custom configuration
config = SimulationConfig(
    token_capacity=20,
    refill_rate=0.5,
    trust_threshold=1,
    num_users=500,
)
for scheme in ["no_x402", "sync", "async"]:
    result = simulate_scheme(config, scheme)
    print(result.summary())
```

## Package Structure

```
x402/
├── __init__.py   # Package exports
├── core.py       # SimulationConfig, UserState, SimulationResult
├── engine.py     # simulate_scheme(), run_comparison()
├── presets.py    # Real-world API presets (OpenAI, Stripe, GitHub, etc.)
└── viz.py        # Plotting and analysis functions
```

## Key Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `token_capacity` | Max tokens per client | 4 |
| `refill_rate` | Tokens per second | 1.0 |
| `trust_threshold` | Payments before trust | 3 |
| `load_multiplier` | Scale request rate | 1.0 |

## Creating Experiments

See `experiments/` for examples. Import the package and customize:

```python
from x402 import SimulationConfig, simulate_scheme

config = SimulationConfig(
    token_capacity=10,
    refill_rate=0.5,
    trust_threshold=1,  # Lower = faster async trust
)

sync = simulate_scheme(config, "sync")
async_result = simulate_scheme(config, "async")

speedup = sync.total_time_ms / async_result.total_time_ms
print(f"Async is {speedup:.2f}x faster")
```
