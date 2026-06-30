# Arbiter Core

An independent pre-trade risk engine and governance layer built specifically to manage non-deterministic execution risks originating from autonomous AI agents.

---

## System Status & Built Realities

This repository operates under a mandate of strict engineering honesty. No fabricated latency profiles, no unbuilt features presented as live.

| Component | Status | Description |
|-----------|--------|-------------|
| **Go Rules Core** | Shipped | Type-safe transaction processing — JSON ingestion, validation, and routing |
| **Graduated Action Matrix (PASS / BLOCK / ESCALATE)** | Shipped | Three-outcome decision schema based on exact value severity margins — replaces binary allow/deny |
| **0–100 Confidence Scorer** | Shipped | Per-decision risk confidence derived from rule severity weights |
| **Human-In-The-Loop Layer** | Shipped | Dashboard holds borderline trades pending manual review — logs reviewer identity, credentials (e.g., CQF), written reason, and nanosecond timestamp |
| **Tamper-Evident Ledger** | Shipped | SHA-256 Merkle-linked chain anchoring all engine decisions and human resolutions — cryptographically verifiable |
| **Alpaca Broker Kill-Switch** | Shipped (mock) | Code path calls `DELETE /v2/positions/{ticker}` — prints to console, ready for a live API key |

---

## Evaluation Pipeline

```
POST /v1/risk/check
        ↓
┌──────────────────────────────────────────┐
│  Rule 1: BannedAssetCheck                │
│    GHOST, TEST → HARD breach             │
│                                          │
│  Rule 2: GraduatedSizeCheck              │
│    TotalValue > $50,000  → HARD breach   │
│    TotalValue > $40,000  → SOFT breach   │
│    TotalValue ≤ $40,000  → OK            │
└──────────────────────────────────────────┘
        ↓
  HARD breach   → BLOCK   (confidence −40 to −50)
  SOFT breach   → ESCALATE (confidence −15, enters review queue)
  No breach     → PASS    (confidence 100)
        ↓
  SHA-256 hash computed and chained to previous block
  Decision stored in registry
  Human resolutions also chained — full immutable audit trail
```

---

## Quick Start

```bash
git clone https://github.com/Patience-Fuglo/arbiter-core.git
cd arbiter-core
go run main.go
```

Open `http://localhost:8080` to use the governance console.

No Docker, no Redis, no Rust toolchain required.

---

## Benchmarks & Verification

```bash
# Real handler latency on your hardware
go test -bench=. -benchmem

# Tamper-detection: watch the chain break when data is altered
go test -v -run=TestCryptographicTamperDetection
```

Expected tamper test output:
```
[TEST STEP 1] Appending valid blocks sequentially into blockchain ledger...
[TEST STEP 2] Baseline audit ledger verification successful. Chain structure valid.
[TEST STEP 3] Executing deliberate data tamper intercept on Block 1 historic values...
[TEST STEP 4] SUCCESS. System isolated structural modification. Validation mismatch: cryptographic corruption at block tx_...
PASS
```

---

## API

### `POST /v1/risk/check`

```json
{
  "agent_id":    "quant_bot_1",
  "ticker":      "SOL",
  "quantity":    1.0,
  "price":       45000.0,
  "total_value": 45000.0
}
```

Response:
```json
{
  "id":               "tx_1719543200000000000",
  "outcome":          "ESCALATE",
  "confidence_score": 85,
  "rule_details": [
    { "rule_name": "BannedAssetCheck",   "triggered": false, "severity": "OK",   "reason": "Asset is cleared." },
    { "rule_name": "GraduatedSizeCheck", "triggered": true,  "severity": "SOFT", "reason": "Trade value of $45000.00 enters the borderline risk escalation band (> $40000.00)." }
  ],
  "prev_hash": "a3f9...",
  "hash":      "7c2b..."
}
```

### `GET /escalations`

Returns all trades currently awaiting human review.

### `POST /escalations/{id}/resolve`

```json
{
  "action":     "ALLOW",
  "reviewer":   "Jane Smith",
  "credential": "CQF",
  "reason":     "Reviewed counterparty exposure — within fund mandate."
}
```

`ALLOW` approves and logs. `REJECT` logs and calls `executeEmergencyFlatten()`.

---

## File Structure

```
arbiter-core/
├── main.go        # Rule engine, HTTP handlers, cryptographic ledger, broker stub
├── main_test.go   # Benchmarks + tamper-detection validation test
├── index.html     # Governance console (served at /)
├── go.mod
└── README.md
```

---

## Technical Optimization Roadmap (Not Yet Built)

| Item | Description |
|------|-------------|
| **Rust Performance Daemon** | Rewriting the hot-path evaluation loop in Rust to eliminate GC pauses and maximize sustained throughput |
| **AWS Nitro Secure Enclaves** | Sealing execution context inside hardware-encrypted partitions — memory isolation guarantees for institutional deployment |
| **eBPF Network Interception** | Moving packet capture to the Linux kernel space to bypass application-layer latency entirely |
| **Live Market Feed (Redis/Kafka)** | Streaming real-time top-of-book prices into a divergence engine for slippage detection |

---

## License

Proprietary. All rights reserved.
