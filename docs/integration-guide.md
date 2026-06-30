# Arbiter Integration Blueprint: Developer Guide

Arbiter sits alongside your existing autonomous trading agents as a non-blocking, parallel risk layer. Your agent fires trades to the broker as normal, while simultaneously mirroring each payload to Arbiter over a local socket. If a violation is detected, Arbiter fires back a `KILL_FLATTEN` signal and calls the broker liquidation API — without touching your execution path.

```
[ Your AI Trading Agent ]
       │
       ├── (1) Sync trade ─────────────────> [ Broker API (Alpaca) ]
       │
       └── (2) Async mirror, fire-and-forget> [ Arbiter Gateway :8080 ]
                                                      │
                                               Unix socket / vsock
                                                      │
                                               [ Rust Shadow Engine ]
                                               Gate 1: Semantic Guardrail
                                               Gate 2: Slippage Divergence
                                               Gate 3: Audit Ledger
                                                      │
                                               KILL? → broker flatten
                                               ALLOW? → silent pass
```

---

## 1. Ingestion Contract

### `POST /v1/risk/check`

**Endpoint:** `http://127.0.0.1:8080/v1/risk/check`  
**Method:** `POST`  
**Headers:** `Content-Type: application/json`

### Request Schema

| Field | Type | Description |
|-------|------|-------------|
| `agent_id` | `string` | Unique identifier for the LLM deployment |
| `timestamp` | `int64` | Unix epoch timestamp |
| `asset_class` | `string` | `us_equity`, `crypto`, or `options` |
| `ticker` | `string` | Max 8 characters (stack-allocated in Rust engine) |
| `order_type` | `string` | `MARKET`, `LIMIT`, `STOP`, or `STOP_LIMIT` |
| `quantity` | `float64` | Lot size |
| `price` | `float64` | Target execution price (USD) |
| `context_window_reasoning` | `string` | Raw LLM chain-of-thought tokens — scanned by the Rust semantic guardrail |
| `crypto_checksum` | `string` | Client-side integrity hash |

### Response Schema

| Field | Type | Description |
|-------|------|-------------|
| `decision` | `string` | `ALLOW` or `KILL` |
| `reason` | `string` | Explanation of the evaluation outcome |
| `latency_ms` | `int64` | Go-side processing time in milliseconds |

### Example Responses

**Approved:**
```json
{"decision":"ALLOW","reason":"Payload within established risk parameters.","latency_ms":0}
```

**Killed — semantic violation:**
```json
{"decision":"KILL","reason":"Semantic Guardrail Violation: Hallucination or Injection Loop Detected","latency_ms":0}
```

**Killed — position size:**
```json
{"decision":"KILL","reason":"Position size limit exceeded. Requested: $90000.00, Max: $100000.00","latency_ms":0}
```

---

## 2. Risk Evaluation Pipeline

```
Inbound payload
      ↓
 GATE 1: ContextGuardrail (Rust, pre-compiled RegexSet, microseconds)
      ├─ VIOLATION → ledger("REJECTED_SEMANTIC") → KILL_FLATTEN → continue
      ↓ clean
 GATE 2: DivergenceEngine (Rust, integer math vs. Redis order book)
      ├─ VIOLATION → ledger("REJECTED_DIVERGENCE") → KILL_FLATTEN
      ↓ clear
 ledger("APPROVED") → silent pass
```

Every entry is SHA-256 hashed, chained to the previous block, and appended to `/var/log/arbiter/compliance_ledger.json`.

---

## 3. Quick-Start Integration Examples

### Python (LangChain / Alpaca)

```python
import requests
import time

def execute_and_mirror_trade(agent_id, ticker, price, qty, reasoning):
    payload = {
        "agent_id": agent_id,
        "timestamp": int(time.time()),
        "asset_class": "us_equity",
        "ticker": ticker,
        "order_type": "LIMIT",
        "quantity": float(qty),
        "price": float(price),
        "context_window_reasoning": reasoning,
        "crypto_checksum": "sha256_agent_hash",
    }

    # 1. Fire live trade to broker
    # alpaca.submit_order(symbol=ticker, qty=qty, side='buy', ...)

    # 2. Parallel mirror to Arbiter — fire and forget
    try:
        resp = requests.post(
            "http://127.0.0.1:8080/v1/risk/check",
            json=payload,
            timeout=0.005,
        )
        verdict = resp.json()
        if verdict["decision"] == "KILL":
            cancel_broker_order()  # abort pending order
    except requests.exceptions.Timeout:
        pass  # Arbiter absorbs asynchronously
```

### Python SDK (Drop-in)

```python
from sdk.python.arbiter_client import ArbiterClient

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
    pass
```

### TypeScript / Node.js

```typescript
import { ArbiterClient } from './sdk/typescript/arbiter-client';

const client = new ArbiterClient('http://127.0.0.1:8080');

const verdict = await client.checkRisk({
    agentId: 'quant_alpha_v3',
    ticker: 'AAPL',
    orderType: 'LIMIT',
    quantity: 100,
    price: 185.50,
    reasoning: 'Momentum signal on 20-day EMA crossover.',
});

if (verdict.decision === 'KILL') {
    // Cancel pending order
}
```

### cURL

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

### Download Audit Trail

```bash
curl -H "X-Arbiter-Auth: YOUR_COMPLIANCE_TOKEN" \
     http://127.0.0.1:8080/v1/compliance/export \
     -o audit_trail.json
```

### Live Telemetry

```bash
curl http://127.0.0.1:8080/v1/telemetry/health
```
```json
{
  "total_signals_read": 54329,
  "successful_drops": 54310,
  "processing_failures": 19
}
```

### Compliance Dashboard

```
http://127.0.0.1:8080/portal
```

---

## 5. Live Order Book Feed (Redis)

Arbiter subscribes to a Redis `market_updates` channel for real-time divergence math. Publish top-of-book ticks in this format:

```json
{
  "symbol": "AAPL",
  "top_bid": 1824500,
  "top_ask": 1824600,
  "volume": 1000
}
```

Prices are **scaled integers** (`price × 10000`) for deterministic fixed-point arithmetic. If Redis is unavailable, Arbiter logs a warning and degrades gracefully — the semantic guardrail and position limit checks still run; only the slippage divergence check has no reference price to compare against.

---

## 6. Deployment Modes

| Mode | `ARBITER_ENV` | Transport | Use Case |
|------|---------------|-----------|----------|
| **DEV** (default) | `DEV` | Unix socket (`/var/run/arbiter/shadow.sock` in tmpfs) | Local dev, demos |
| **PROD** | `PROD` | vsock (CID 16, port 5005) | AWS Nitro Enclave |

### DEV
```bash
cp .env.example .env && docker compose up --build
```

### PROD (Nitro Enclave)
```bash
cd enclave && ./build-enclave.sh
```

---

## 7. Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ARBITER_ENV` | No | `DEV` | `DEV` (Unix socket) or `PROD` (vsock) |
| `ALPACA_API_KEY_ID` | Yes | — | Broker API key |
| `ALPACA_API_SECRET_KEY` | Yes | — | Broker API secret |
| `ARBITER_COMPLIANCE_TOKEN` | Yes | — | Token for `/v1/compliance/export` |
| `ARBITER_REDIS_URL` | No | `redis://127.0.0.1:6379` | Live order book feed |
| `ARBITER_ENCLAVE_CID` | No | `16` | Enclave CID when `ARBITER_ENV=PROD` |

---

## 8. OpenAPI Spec

```bash
# Generate typed clients in any language
openapi-generator generate -i api/openapi.yaml -g python -o sdk/python-generated
openapi-generator generate -i api/openapi.yaml -g typescript-axios -o sdk/ts-generated
openapi-generator generate -i api/openapi.yaml -g go -o sdk/go-generated
openapi-generator generate -i api/openapi.yaml -g java -o sdk/java-generated
```
