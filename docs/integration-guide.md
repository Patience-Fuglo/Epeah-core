# Arbiter Integration Blueprint: Developer Guide

Arbiter sits alongside your existing autonomous trading agents (built with LangChain, LlamaIndex, or raw Alpaca SDKs) as a non-blocking, parallel risk layer.

```
[ Your AI Trading Agent ]
       │
       ├── (Sync HTTP Trade Request) ───────> [ Broker API (Alpaca) ]
       │
       └── (Async Parallel JSON Mirror) ───> [ Arbiter Gateway (:8080) ]
                                                      │
                                               (Nitro Enclave Sub-1ms Check)
                                                      │
                                               ┌──────┴──────┐
                                               │  ALLOW      │  KILL
                                               │  (silent)   │  → Broker Liquidation
                                               └─────────────┘
```

Your agent fires its trade to the broker as normal, while simultaneously sending a duplicate payload to Arbiter's high-speed local listener. Arbiter evaluates asynchronously — zero latency impact on your live execution path.

---

## 1. Ingestion Contract Specification

### `POST /v1/risk/check`

**Endpoint:** `http://127.0.0.1:8080/v1/risk/check`
**Method:** `POST`
**Headers:** `Content-Type: application/json`

### Request Payload Schema

| Field | Type | Description |
|-------|------|-------------|
| `agent_id` | `string` | Unique identifier for the LLM deployment |
| `timestamp` | `int64` | Unix epoch timestamp of order generation |
| `asset_class` | `string` | `us_equity`, `crypto`, or `options` |
| `ticker` | `string` | Max 8-character symbol (stack-allocated in Rust engine) |
| `order_type` | `string` | `MARKET`, `LIMIT`, `STOP`, or `STOP_LIMIT` |
| `quantity` | `float64` | Total share or lot size requested |
| `price` | `float64` | Target execution price in USD |
| `context_window_reasoning` | `string` | Raw chain-of-thought tokens from your model |
| `crypto_checksum` | `string` | Client-side integrity hash for payload verification |

### Response Schema

| Field | Type | Description |
|-------|------|-------------|
| `decision` | `string` | `ALLOW` (trade cleared) or `KILL` (trade blocked, broker kill-switch triggered) |
| `reason` | `string` | Human-readable explanation of the evaluation outcome |
| `latency_ms` | `int64` | Processing time in milliseconds |

### Example Request

```json
{
  "agent_id": "quant_alpha_v3",
  "timestamp": 1719543200,
  "asset_class": "us_equity",
  "ticker": "AAPL",
  "order_type": "LIMIT",
  "quantity": 100.0,
  "price": 185.50,
  "context_window_reasoning": "Momentum signal triggered on 20-day EMA crossover. Risk-adjusted position within 2% portfolio allocation.",
  "crypto_checksum": "a1b2c3d4e5f67890abcdef"
}
```

### Example Responses

**Trade Approved:**
```json
{
  "decision": "ALLOW",
  "reason": "Payload within established risk parameters.",
  "latency_ms": 0
}
```

**Trade Killed — Position Size:**
```json
{
  "decision": "KILL",
  "reason": "Position size limit exceeded. Requested: $185500.00, Max: $100000.00",
  "latency_ms": 0
}
```

**Trade Killed — Restricted Ticker:**
```json
{
  "decision": "KILL",
  "reason": "Ticker 'DOGE' is on the internal risk restriction list.",
  "latency_ms": 0
}
```

---

## 2. Risk Evaluation Pipeline

Every payload passes through three sequential evaluation gates inside the Rust shadow engine:

```
Gate 1: Semantic Guardrail (regex DFA, microsecond eval)
  → Detects prompt injection, hallucination loops, risk override attempts
  → KILL on match, skip all downstream processing

Gate 2: Slippage Divergence Engine (integer math, config-driven threshold)
  → Compares execution price against shadow sandbox market state
  → KILL if deviation exceeds configured bps threshold (default: 15bps)

Gate 3: Position Size & Asset Restriction (Go-side, sub-millisecond)
  → Notional value check against max position size ($100,000 default)
  → Banned asset list enforcement
```

Every evaluation (APPROVED, REJECTED_SEMANTIC, REJECTED_DIVERGENCE) is cryptographically hashed and appended to an immutable Merkle-chained audit ledger.

---

## 3. Quick-Start Integration Examples

### Python (LangChain / Alpaca)

```python
import requests
import time

def execute_and_mirror_trade(agent_id, ticker, price, qty, reasoning):
    payload = {
        "agent_id": agent_id,
        "timestamp": int(time.time_ns()),
        "asset_class": "us_equity",
        "ticker": ticker,
        "order_type": "LIMIT",
        "quantity": float(qty),
        "price": float(price),
        "context_window_reasoning": reasoning,
        "crypto_checksum": "sha256_agent_verification_hash"
    }

    # 1. Fire live trade to broker
    # alpaca.submit_order(symbol=ticker, qty=qty, side='buy', ...)

    # 2. Parallel, non-blocking mirror to Arbiter sidecar
    try:
        resp = requests.post(
            "http://127.0.0.1:8080/v1/risk/check",
            json=payload,
            timeout=0.005
        )
        verdict = resp.json()
        if verdict["decision"] == "KILL":
            # Cancel pending broker order immediately
            print(f"ARBITER KILL: {verdict['reason']}")
    except requests.exceptions.Timeout:
        pass  # Gateway absorbs payload asynchronously
```

### Python SDK (Drop-In Client)

```python
from arbiter_client import ArbiterClient

client = ArbiterClient("http://127.0.0.1:8080")

verdict = client.check_risk(
    agent_id="quant_alpha_v3",
    ticker="AAPL",
    order_type="LIMIT",
    quantity=100,
    price=185.50,
    reasoning="Momentum signal on 20-day EMA crossover.",
)

if verdict["decision"] == "KILL":
    # Abort order
    ...
```

### TypeScript / Node.js

```typescript
import axios from 'axios';

async function logTradeToArbiter(agentId: string, ticker: string, reasoning: string) {
    const payload = {
        agent_id: agentId,
        timestamp: Date.now() * 1000000,
        asset_class: "us_equity",
        ticker: ticker,
        order_type: "MARKET",
        quantity: 50.0,
        price: 900.25,
        context_window_reasoning: reasoning,
        crypto_checksum: "node_verified_envelope"
    };

    // Fire-and-forget to maintain hot-path speed
    axios.post('http://127.0.0.1:8080/v1/risk/check', payload, { timeout: 5 })
         .catch(() => {});
}
```

### cURL (Manual Testing)

```bash
curl -X POST http://127.0.0.1:8080/v1/risk/check \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "test_agent",
    "timestamp": 1719543200,
    "asset_class": "us_equity",
    "ticker": "AAPL",
    "order_type": "LIMIT",
    "quantity": 100,
    "price": 185.50,
    "context_window_reasoning": "Standard rebalance within parameters.",
    "crypto_checksum": "test_hash"
  }'
```

---

## 4. Administrative Endpoints

### `GET /v1/compliance/export`

Downloads the cryptographic audit trail as a FINRA-compliant JSON file.

```bash
curl -H "X-Arbiter-Auth: YOUR_COMPLIANCE_TOKEN" \
     http://127.0.0.1:8080/v1/compliance/export \
     -o audit_trail.json
```

### `GET /v1/telemetry/health`

Returns real-time signal processing metrics from the kill-signal worker pool.

```bash
curl http://127.0.0.1:8080/v1/telemetry/health
```

Response:
```json
{
  "total_signals_read": 54329,
  "successful_drops": 54310,
  "processing_failures": 19
}
```

---

## 5. Deployment Modes

| Mode | Transport | `ARBITER_ENV` | Use Case |
|------|-----------|---------------|----------|
| **DEV** (default) | Unix socket (`/var/run/arbiter/shadow.sock`) | `DEV` | Local development, demos, standard VPC |
| **PROD** | vsock (CID:5005) | `PROD` | AWS Nitro Enclave, institutional deployment |

### Local Development (DEV)

```bash
cp .env.example .env
docker compose up --build
```

### Nitro Enclave (PROD)

```bash
cd enclave && ./build-enclave.sh
```

---

## 6. Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `ARBITER_ENV` | No | `DEV` (default) for Unix sockets, `PROD` for Nitro Enclave vsock |
| `ALPACA_API_KEY_ID` | Yes | Broker API key for emergency liquidation |
| `ALPACA_API_SECRET_KEY` | Yes | Broker API secret |
| `ARBITER_COMPLIANCE_TOKEN` | Yes | Auth token for `/v1/compliance/export` |
| `ARBITER_ENCLAVE_CID` | No | Enclave CID when `ARBITER_ENV=PROD` (default: 16) |

---

## 7. OpenAPI Specification

Full machine-readable API schema available at `api/openapi.yaml`. Generate typed clients in any language:

```bash
openapi-generator generate -i api/openapi.yaml -g python -o sdk/python-generated
openapi-generator generate -i api/openapi.yaml -g typescript-axios -o sdk/ts-generated
openapi-generator generate -i api/openapi.yaml -g go -o sdk/go-generated
```
