# Rate Limiter with X402 Payment Integration

A token bucket rate limiter that accepts crypto payments to refill quota. When users exceed their rate limit, they can pay via the X402 protocol to instantly restore their capacity.

## Prerequisites

- Go 1.25+
- Redis (optional, for distributed rate limiting)
- A funded wallet on Base Sepolia for client payments

## Configuration

Edit `config.yaml` to customize the server:

```yaml
server:
  port: ":8081"              # Server listen address

ratelimit:
  capacity: 4                # Maximum tokens in bucket
  refill_rate: 4             # Tokens added per second
  strategy: "memory"         # "memory" or "redis"

redis:
  addr: "localhost:6379"     # Redis address (if strategy: "redis")
  password: ""
  db: 0

payment:
  enabled: true
  facilitator_url: "https://www.x402.org/facilitator"
  wallet_address: "0x..."    # Your wallet to receive payments
  price_per_capacity: "0.001" # USDC per capacity refill
  network: "base-sepolia"
  currency: "USDC"
```

## Quick Start

1. **Install dependencies**
   ```bash
   go mod download
   ```

2. **Start the server**
   ```bash
   go run ./cmd/server/main.go
   ```

3. **Run the client** (requires a funded Base Sepolia wallet)
   ```bash
   PRIVATE_KEY=<your-private-key> go run ./cmd/client/main.go
   ```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /cpu` | Returns CPU utilization (rate limited) |
| `GET /dashboard` | Live monitoring dashboard |
| `GET /tokens` | Returns current token count for client (for debugging) |

## End-to-End Payment Flow

```
┌─────────┐          ┌─────────┐          ┌─────────────┐          ┌─────────────┐
│  Client │          │  Server │          │ Facilitator │          │  Blockchain │
└────┬────┘          └────┬────┘          └──────┬──────┘          └──────┬──────┘
     │                    │                      │                        │
     │  1. GET /cpu       │                      │                        │
     │───────────────────>│                      │                        │
     │                    │                      │                        │
     │      ┌─────────────┴─────────────┐       │                        │
     │      │ limiter.Allow(clientIP)   │       │                        │
     │      │ Check token bucket        │       │                        │
     │      └─────────────┬─────────────┘       │                        │
     │                    │                      │                        │
     │    ╔═══════════════╧═══════════════╗     │                        │
     │    ║  If tokens available:         ║     │                        │
     │    ║  → Return 200 OK + response   ║     │                        │
     │    ╚═══════════════╤═══════════════╝     │                        │
     │                    │                      │                        │
     │    ╔═══════════════╧═══════════════╗     │                        │
     │    ║  If rate limited + no payment:║     │                        │
     │    ║  → Return 402 Payment Required║     │                        │
     │    ║    with payment requirements  ║     │                        │
     │    ╚═══════════════╤═══════════════╝     │                        │
     │                    │                      │                        │
     │<───────────────────│                      │                        │
     │  402 + requirements│                      │                        │
     │                    │                      │                        │
     │  2. Sign payment   │                      │                        │
     │  ─ ─ ─ ─ ─ ─ ─ ─ ─>│                      │                        │
     │                    │                      │                        │
     │  3. GET /cpu       │                      │                        │
     │  + Payment header  │                      │                        │
     │───────────────────>│                      │                        │
     │                    │                      │                        │
     │                    │  4. Verify payment   │                        │
     │                    │─────────────────────>│                        │
     │                    │                      │                        │
     │                    │  5. Settle on-chain  │                        │
     │                    │─────────────────────>│───────────────────────>│
     │                    │                      │   Transfer USDC        │
     │                    │                      │<───────────────────────│
     │                    │<─────────────────────│   TX confirmed         │
     │                    │                      │                        │
     │      ┌─────────────┴─────────────┐       │                        │
     │      │ limiter.Refill(clientIP,  │       │                        │
     │      │            capacity)      │       │                        │
     │      │ Add tokens to bucket      │       │                        │
     │      └─────────────┬─────────────┘       │                        │
     │                    │                      │                        │
     │<───────────────────│                      │                        │
     │  6. 200 OK         │                      │                        │
     │  + CPU response    │                      │                        │
     │                    │                      │                        │
```

### Flow Steps

1. **Request arrives** - Client makes `GET /cpu` request
2. **Token check** - `limiter.Allow(clientIP)` checks if tokens are available
3. **If allowed** - Request proceeds, returns `200 OK` with response
4. **If rate limited + no payment** - Returns `402 Payment Required` with X402 payment requirements (price, network, wallet address)
5. **If rate limited + payment header present**:
   - Server verifies payment signature via X402 protocol
   - Server requests settlement through Facilitator service
   - Facilitator executes on-chain transfer (Base Sepolia USDC)
   - On success: `limiter.Refill(clientIP, capacity)` adds tokens to bucket
   - Request proceeds, returns `200 OK`

## Rate Limiting Behavior

The rate limiter uses a **token bucket algorithm**:

- **Natural refill**: Tokens regenerate at `refill_rate` per second, capped at `capacity`
- **Paid refill**: Adds tokens that can exceed capacity (burst tokens)
- **Consumption**: Each request consumes 1 token
- **Reactive payment**: Payment only occurs when rate limited (402 response) - users cannot pre-pay

### Important: Natural Refill Rules

| Token State | Natural Refill Behavior |
|-------------|------------------------|
| Below capacity | Refills at `refill_rate`, capped at `capacity` |
| At or above capacity | **No natural refill** (burst tokens preserved) |

This prevents unbounded token accumulation while preserving paid burst capacity.

### Examples

Example with `capacity: 4, refill_rate: 4`:

```
Scenario 1 - Normal flow:
  Start:    4 tokens
  Request:  -1 token → 3 tokens remaining
  Wait 1s:  +4 tokens → capped at 4 (capacity)

Scenario 2 - Payment flow (README Scenario 2):
  Start:    0 tokens (exhausted)
  Request:  Rate limited → 402 returned
  Pay:      +4 tokens added, -1 for request → 3 tokens remaining

Scenario 3 - Natural refill during payment processing:
  Start:    0 tokens (exhausted)
  Request:  Rate limited → 402 returned
  Payment:  Takes ~0.8s to verify + settle
  Result:   +4 (paid) + 0.84 (natural refill during 0.8s) - 1 (request) = 4.84 tokens

Scenario 4 - Burst tokens don't grow:
  Start:    4.84 tokens (above capacity, from payment)
  Wait 1s:  Still 4.84 tokens (no natural refill when above capacity!)
  Request:  -1 token → 3.84 tokens
  Wait 1s:  +4 tokens → capped at 4 (natural refill resumes below capacity)
```

## Testing

Run unit tests:
```bash
go test ./...
```

Run integration tests (requires running server and funded wallet):
```bash
# Start server
go run ./cmd/server/main.go

# Run client integration tests
PRIVATE_KEY=<your-private-key> go test -v ./cmd/client/...
```

## License

MIT