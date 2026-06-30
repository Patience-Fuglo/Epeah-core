package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ==========================================
// BENCHMARK SUITE
// ==========================================

func BenchmarkEngineRiskEvaluation(b *testing.B) {
	handler := http.HandlerFunc(handleRiskCheck)
	payloadBytes := []byte(`{"agent_id":"hft_agent_0","ticker":"BTC","quantity":0.1,"price":65000.0}`)

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
// TAMPER-EVIDENT CHAIN VALIDATION
// ==========================================

func TestCryptographicTamperDetection(t *testing.T) {
	// Reset state for a clean test run
	stateMutex.Lock()
	DecisionRegistry = make(map[string]*EngineDecision)
	LastChainHash = "genesis_hash_test_vector"
	stateMutex.Unlock()

	d1 := evaluatePayload(TradePayload{AgentID: "bot", Ticker: "ETH", Quantity: 1, Price: 3000})
	_ = evaluatePayload(TradePayload{AgentID: "bot", Ticker: "SOL", Quantity: 10, Price: 150})

	// Baseline: chain must verify cleanly
	if err := verifyChainIntegrity(); err != nil {
		t.Fatalf("Baseline chain verification failed unexpectedly: %v", err)
	}
	fmt.Println("[INTEGRITY VERIFY] Baseline audit chain passes cryptographic linkage validation.")

	// Tamper: silently mutate a historic record
	fmt.Println("[TAMPER SIMULATION] Altering historic block price field...")
	stateMutex.Lock()
	DecisionRegistry[d1.ID].Payload.Price = 99999.0
	stateMutex.Unlock()

	// Post-tamper: verification MUST fail
	if err := verifyChainIntegrity(); err == nil {
		t.Fatal("SECURITY FAULT: engine did not detect tampered block!")
	} else {
		fmt.Printf("[TAMPER CAUGHT] Audit engine rejected altered state: %v\n", err)
	}
}

func verifyChainIntegrity() error {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	for _, d := range DecisionRegistry {
		if computeDecisionHash(d) != d.Hash {
			return fmt.Errorf("hash mismatch at block %s", d.ID)
		}
	}
	return nil
}
