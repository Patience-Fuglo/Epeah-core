# Arbiter Core

An independent governance layer and pre-trade safety gate for autonomous AI agents executing financial transactions.

---

## What Is Actually Built and Shipped

This codebase is maintained with strict engineering honesty. Every feature listed below compiles, runs, and can be tested locally with a single command.

| Component | Status | Description |
|-----------|--------|-------------|
| **Go Rule Engine** | Shipped | High-speed, type-safe transaction parsing and evaluation |
| **Graduated Evaluation (PASS / BLOCK / ESCALATE)** | Shipped | Three-outcome risk decision matrix replacing a binary kill switch |
| **0–100 Confidence Scorer** | Shipped | Per-transaction risk confidence derived from rule severity weights |
| **Human-In-The-Loop Escalation** | Shipped | Borderline trades enter a compliance review queue with mandatory written rationale |
| **Cryptographic Audit Trail** | Shipped | SHA-256 Merkle-linked chain of every decision and resolution — tamper-evident |
| **Broker Flatten Placeholder** | Shipped | `executeEmergencyFlatten()` prints the Alpaca DELETE call — ready for a real key |

---

## The Evaluation Pipeline

```
Inbound trade payload (POST /v1/risk/check)
            ↓
┌───────────────────────────────────────┐
│  Rule 1: Banned Asset Check (HARD)    │
│  Rule 2: Graduated Position Size      │
│    > $50,000           → HARD BREACH  │
│    $40,000 – $50,000   → SOFT BREACH  │
│    < $40,000           → OK           │
└───────────────────────────────────────┘
            ↓
  HARD breach → BLOCK (confidence −40 to −50)
  SOFT breach → ESCALATE (confidence −15)
  No breach   → PASS (confidence 100)
            ↓
  SHA-256 hash computed, chained to previous block
  Decision written to in-memory registry
  ESCALATE → enters human review queue
```

Every outcome — PASS, BLOCK, ESCALATE — is hashed and chained. Human resolutions are also hashed and appended to the same chain, creating a complete immutable record of every action taken by both the engine and the compliance team.

---

## Quick Start

```bash
git clone https://github.com/Patience-Fuglo/arbiter-core.git
cd arbiter-core
go run main.go
```

Open `http://localhost:8080` in a browser to use the governance console.

---

## Benchmarks & Verification

No configuration needed — just clone and run:

```bash
# Measure real handler latency on your hardware
go test -bench=. -benchmem

# Verify the tamper-evident chain catches mutations
go test -v -run=TestCryptographicTamperDetection
```

Expected benchmark output (indicative — varies by hardware):
```
BenchmarkEngineRiskEvaluation-8        ~500,000 ops/sec    ~2,400 ns/op
BenchmarkEngineRiskEvaluationParallel-8  ~2,000,000 ops/sec    ~600 ns/op
```

Expected tamper detection output:
```
[INTEGRITY VERIFY] Baseline audit chain passes cryptographic linkage validation.
[TAMPER SIMULATION] Altering historic block price field...
[TAMPER CAUGHT] Audit engine rejected altered state: hash mismatch at block tx_...
PASS
```

---

## API Reference

### `POST /v1/risk/check`

Submit a trade payload for evaluation.

**Request:**
```json
{
  "agent_id":    "quant_bot_1",
  "ticker":      "ETH",
  "quantity":    1.0,
  "price":       3000.0,
  "total_value": 42000.0
}
```

**Response:**
```json
{
  "id":               "tx_1719543200000000000",
  "outcome":          "ESCALATE",
  "confidence_score": 85,
  "rule_details": [
    { "rule_name": "BannedAssetCheck",  "triggered": false, "severity": "OK",   "reason": "Asset is cleared." },
    { "rule_name": "GraduatedSizeCheck","triggered": true,  "severity": "SOFT", "reason": "Trade value of $42000.00 enters the escalation band (> $40000.00)." }
  ],
  "prev_hash": "a3f9...",
  "hash":      "7c2b..."
}
```

Possible `outcome` values:

| Outcome | Condition | Next Step |
|---------|-----------|-----------|
| `PASS` | All rules clear | Trade proceeds |
| `BLOCK` | Hard rule triggered | Trade rejected immediately |
| `ESCALATE` | Soft rule triggered | Enters human review queue |

### `GET /escalations`

Returns the current list of trades awaiting human review.

### `POST /escalations/{id}/resolve`

Resolve an escalated trade. A written reason is mandatory.

```json
{
  "action":     "ALLOW",
  "reviewer":   "Jane Smith",
  "credential": "CQF",
  "reason":     "Reviewed counterparty exposure — within fund mandate for this asset class."
}
```

| Action | Effect |
|--------|--------|
| `ALLOW` | Trade is approved and logged to the audit chain |
| `REJECT` | Trade is rejected; `executeEmergencyFlatten()` is called |

---

## File Structure

```
arbiter-core/
├── main.go          # Rule engine, HTTP handlers, cryptographic ledger
├── main_test.go     # Benchmarks + tamper-detection test
├── index.html       # Governance console (served at /)
├── go.mod
└── README.md
```

---

## Optimization Roadmap (Not Yet Built)

The following are planned engineering upgrades — none are presented as currently active:

- **Live Market Feeds** — Redis pub/sub or Kafka integration to stream real-time top-of-book prices into the divergence engine
- **Rust Validation Daemon** — Rewriting the hot-path evaluation loop in Rust to eliminate GC pauses and maximize throughput under sustained load
- **AWS Nitro Secure Enclaves** — Sealing the execution context inside hardware-isolated computing environments for institutional memory encryption guarantees

---

## License

Proprietary. All rights reserved.
