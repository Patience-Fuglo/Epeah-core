package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ==========================================
// 1. UNFABRICATED LOCAL HARDWARE BENCHMARK
// ==========================================

func BenchmarkEngineRiskEvaluation(b *testing.B) {
	handler := http.HandlerFunc(handleRiskCheck)
	payloadBytes := []byte(`{"agent_id":"hft_agent_0","ticker":"BTC","quantity":0.5,"price":64500.0}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/v1/risk/check", bytes.NewBuffer(payloadBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkEngineRiskEvaluationParallel(b *testing.B) {
	handler := http.HandlerFunc(handleRiskCheck)
	payloadBytes := []byte(`{"agent_id":"hft_agent_parallel","ticker":"ETH","quantity":1.0,"price":3000.0}`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/v1/risk/check", bytes.NewBuffer(payloadBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}
	})
}

// ==========================================
// 2. TAMPER-EVIDENT CRYPTO LINK VALIDATION
// ==========================================

func TestCryptographicTamperDetection(t *testing.T) {
	stateMutex.Lock()
	DecisionRegistry = make(map[string]*EngineDecision)
	LastChainHash = "genesis_hash_test_vector"
	stateMutex.Unlock()

	fmt.Println("[TEST STEP 1] Appending valid blocks sequentially into blockchain ledger...")
	d1 := evaluatePayload(TradePayload{AgentID: "agent_1", Ticker: "ETH", Quantity: 2.0, Price: 3500.0})
	d2 := evaluatePayload(TradePayload{AgentID: "agent_2", Ticker: "SOL", Quantity: 50.0, Price: 140.0})
	_ = d2

	err := verifyChainIntegrityLocal()
	if err != nil {
		t.Fatalf("Cryptographic blockchain link failed baseline check: %v", err)
	}
	fmt.Println("[TEST STEP 2] Baseline audit ledger verification successful. Chain structure valid.")

	fmt.Println("[TEST STEP 3] Executing deliberate data tamper intercept on Block 1 historic values...")
	stateMutex.Lock()
	DecisionRegistry[d1.ID].Payload.Price = 9999999.0
	stateMutex.Unlock()

	err = verifyChainIntegrityLocal()
	if err == nil {
		t.Fatal("CRITICAL SECURITY FAULT: chain verification failed to catch record manipulation!")
	}
	fmt.Printf("[TEST STEP 4] SUCCESS. System isolated structural modification. Validation mismatch: %v\n", err)
}

func verifyChainIntegrityLocal() error {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	for _, d := range DecisionRegistry {
		if computeDecisionHash(d) != d.Hash {
			return fmt.Errorf("cryptographic corruption at block %s: re-calculated hash mismatches stored anchor", d.ID)
		}
	}
	return nil
}

// ==========================================
// 4. AUTONOMY ENVELOPE — PER-AGENT DIFFERENTIATION
// ==========================================
//
// Proves the Autonomy Envelope is a real, functioning per-agent artifact —
// not just the old global constants renamed. The identical $30,000 NIO
// trade produces two different verdicts depending on which agent's
// envelope is in force, because different agents carry different
// authorized limits.

func TestAutonomyEnvelope_PerAgentDifferentiation(t *testing.T) {
	stateMutex.Lock()
	Portfolio = map[string]float64{} // isolate from other tests' portfolio state
	stateMutex.Unlock()

	envelopeRegistry["conservative_desk_agent"] = AutonomyEnvelope{
		AgentID:                 "conservative_desk_agent",
		RestrictedInstruments:   []string{"GHOST", "TEST", "SDN"},
		MaxOrderValue:           10000.0,
		SoftEscalationFraction:  0.7,
		MaxSingleTickerExposure: 25000.0,
		MaxSectorExposure:       30000.0,
	}
	envelopeRegistry["systematic_ev_desk_agent"] = AutonomyEnvelope{
		AgentID:                 "systematic_ev_desk_agent",
		RestrictedInstruments:   []string{"GHOST", "TEST"},
		MaxOrderValue:           100000.0,
		SoftEscalationFraction:  0.8,
		MaxSingleTickerExposure: 75000.0,
		MaxSectorExposure:       80000.0,
	}

	fmt.Println("[ENVELOPE TEST] Same $30,000 NIO trade, two agents, two different Autonomy Envelopes:")

	conservativeDecision := evaluatePayload(TradePayload{
		AgentID: "conservative_desk_agent", Ticker: "NIO", TotalValue: 30000.0,
	})
	fmt.Printf("[ENVELOPE TEST] conservative_desk_agent (max order $10,000) -> %s\n", conservativeDecision.Outcome)
	if conservativeDecision.Outcome != "BLOCK" {
		t.Fatalf("expected conservative agent's $30,000 trade to BLOCK against its $10,000 cap, got: %s", conservativeDecision.Outcome)
	}

	stateMutex.Lock()
	Portfolio = map[string]float64{}
	stateMutex.Unlock()

	aggressiveDecision := evaluatePayload(TradePayload{
		AgentID: "systematic_ev_desk_agent", Ticker: "NIO", TotalValue: 30000.0,
	})
	fmt.Printf("[ENVELOPE TEST] systematic_ev_desk_agent (max order $100,000) -> %s\n", aggressiveDecision.Outcome)
	if aggressiveDecision.Outcome != "PASS" {
		t.Fatalf("expected systematic agent's $30,000 trade to PASS against its $100,000 cap with empty portfolio, got: %s", aggressiveDecision.Outcome)
	}

	fmt.Println("[ENVELOPE TEST SUCCESS] Identical trade, identical engine code, different verdicts — driven entirely by each agent's Autonomy Envelope.")
}
//
// This is the core differentiation proof: a trade that is individually
// fully authorized (not a banned asset, well within the single-trade size
// limit) is still escalated to a human, because Arbiter's concentration
// check sees portfolio-level context that a static permission check cannot.

func TestAuthorizedButUnsafe_ConcentrationEscalation(t *testing.T) {
	stateMutex.Lock()
	Portfolio = map[string]float64{
		"TSLA": 32000.0,
		"RIVN": 28000.0,
	}
	stateMutex.Unlock()

	fmt.Println("[BENCHMARK] Existing portfolio: TSLA $32,000 + RIVN $28,000 (EV sector) = $60,000 sector exposure")
	fmt.Println("[BENCHMARK] Proposing: BUY NIO, $30,000 — individually well within all static limits")

	decision := evaluatePayload(TradePayload{
		AgentID:    "hft_agent_ev_desk",
		Ticker:     "NIO",
		TotalValue: 30000.0,
	})

	var bannedResult, sizeResult, concentrationResult *RuleResult
	for i := range decision.RuleDetails {
		switch decision.RuleDetails[i].RuleName {
		case "BannedAssetCheck":
			bannedResult = &decision.RuleDetails[i]
		case "GraduatedSizeCheck":
			sizeResult = &decision.RuleDetails[i]
		case "ConcentrationRiskCheck":
			concentrationResult = &decision.RuleDetails[i]
		}
	}

	if bannedResult == nil || bannedResult.Severity != "OK" {
		t.Fatalf("expected NIO to be fully authorized on BannedAssetCheck, got: %+v", bannedResult)
	}
	if sizeResult == nil || sizeResult.Severity != "OK" {
		t.Fatalf("expected $30,000 to be fully authorized on GraduatedSizeCheck, got: %+v", sizeResult)
	}
	fmt.Println("[BENCHMARK] Confirmed: trade is fully AUTHORIZED under both static rules (banned-asset + size).")

	if concentrationResult == nil || concentrationResult.Severity != "SOFT" {
		t.Fatalf("expected ConcentrationRiskCheck to trigger SOFT on sector exposure, got: %+v", concentrationResult)
	}
	if decision.Outcome != "ESCALATE" {
		t.Fatalf("expected overall outcome ESCALATE despite passing static rules, got: %s", decision.Outcome)
	}

	fmt.Printf("[BENCHMARK] Arbiter result: %s — %s\n", decision.Outcome, concentrationResult.Reason)
	fmt.Println("[BENCHMARK SUCCESS] A fully authorized trade was still escalated for human review, based on context static permissions cannot see.")
}
