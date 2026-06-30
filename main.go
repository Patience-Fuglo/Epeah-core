package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ==========================================
// DATA STRUCTURES & PROTOCOL TAXONOMY
// ==========================================

type TradePayload struct {
	AgentID    string  `json:"agent_id"`
	Ticker     string  `json:"ticker"`
	Quantity   float64 `json:"quantity"`
	Price      float64 `json:"price"`
	TotalValue float64 `json:"total_value"`
}

type RuleResult struct {
	RuleName  string `json:"rule_name"`
	Triggered bool   `json:"triggered"`
	Severity  string `json:"severity"` // "OK", "SOFT", "HARD"
	Reason    string `json:"reason"`
}

type EngineDecision struct {
	ID              string       `json:"id"`
	Payload         TradePayload `json:"payload"`
	Outcome         string       `json:"outcome"` // "PASS", "BLOCK", "ESCALATE"
	ConfidenceScore int          `json:"confidence_score"`
	RuleDetails     []RuleResult `json:"rule_details"`
	Timestamp       int64        `json:"timestamp"`
	PrevHash        string       `json:"prev_hash"`
	Hash            string       `json:"hash"`
}

type HumanResolution struct {
	DecisionID string `json:"decision_id"`
	Action     string `json:"action"` // "ALLOW", "REJECT"
	Reviewer   string `json:"reviewer"`
	Credential string `json:"credential"` // e.g., "CQF"
	Reason     string `json:"reason"`
	Timestamp  int64  `json:"timestamp"`
	PrevHash   string `json:"prev_hash"`
	Hash       string `json:"hash"`
}

// ==========================================
// STATE ARCHITECTURE & LOGGING LEDGERS
// ==========================================

var (
	stateMutex         sync.Mutex
	DecisionRegistry   = make(map[string]*EngineDecision)
	EscalationQueue    = make([]*EngineDecision, 0)
	ResolutionLogs     = make([]HumanResolution, 0)
	CryptographicChain = make([]string, 0)
	LastChainHash      = "0000000000000000000000000000000000000000000000000000000000000000"
)

const MaxPositionValue = 50000.0

var BannedAssets = map[string]bool{"GHOST": true, "TEST": true}

// ==========================================
// GRADUATED DECISION MATRIX
// ==========================================

func evaluatePayload(payload TradePayload) *EngineDecision {
	decision := &EngineDecision{
		ID:              fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		Payload:         payload,
		Outcome:         "PASS",
		ConfidenceScore: 100,
		RuleDetails:     make([]RuleResult, 0),
		Timestamp:       time.Now().UnixNano(),
	}

	// Rule 1: Banned Registry
	bannedRule := RuleResult{
		RuleName: "BannedAssetCheck",
		Severity: "OK",
		Reason:   "Asset is cleared for transaction tracking.",
	}
	if BannedAssets[strings.ToUpper(payload.Ticker)] {
		bannedRule.Triggered = true
		bannedRule.Severity = "HARD"
		bannedRule.Reason = fmt.Sprintf("Asset %s is explicitly restricted from autonomous deployment.", payload.Ticker)
		decision.ConfidenceScore -= 50
	}
	decision.RuleDetails = append(decision.RuleDetails, bannedRule)

	// Rule 2: Position Allocation Sizing
	calculatedValue := payload.Quantity * payload.Price
	if payload.TotalValue > 0 {
		calculatedValue = payload.TotalValue
	}

	sizeRule := RuleResult{
		RuleName: "GraduatedSizeCheck",
		Severity: "OK",
		Reason:   "Position parameters fall within standard allocations.",
	}
	if calculatedValue > MaxPositionValue {
		sizeRule.Triggered = true
		sizeRule.Severity = "HARD"
		sizeRule.Reason = fmt.Sprintf(
			"Trade value of $%.2f exceeds the $%.2f hard boundary cap.",
			calculatedValue, MaxPositionValue,
		)
		decision.ConfidenceScore -= 40
	} else if calculatedValue > MaxPositionValue*0.8 {
		sizeRule.Triggered = true
		sizeRule.Severity = "SOFT"
		sizeRule.Reason = fmt.Sprintf(
			"Trade value of $%.2f enters the borderline risk escalation band (> $%.2f).",
			calculatedValue, MaxPositionValue*0.8,
		)
		decision.ConfidenceScore -= 15
	}
	decision.RuleDetails = append(decision.RuleDetails, sizeRule)

	hasHardBreach := false
	hasSoftBreach := false
	for _, r := range decision.RuleDetails {
		if r.Severity == "HARD" {
			hasHardBreach = true
		}
		if r.Severity == "SOFT" {
			hasSoftBreach = true
		}
	}

	if hasHardBreach {
		decision.Outcome = "BLOCK"
	} else if hasSoftBreach {
		decision.Outcome = "ESCALATE"
	}

	if decision.ConfidenceScore < 0 {
		decision.ConfidenceScore = 0
	}

	stateMutex.Lock()
	decision.PrevHash = LastChainHash
	decision.Hash = computeDecisionHash(decision)
	LastChainHash = decision.Hash
	CryptographicChain = append(CryptographicChain, decision.Hash)
	DecisionRegistry[decision.ID] = decision
	if decision.Outcome == "ESCALATE" {
		EscalationQueue = append(EscalationQueue, decision)
	}
	stateMutex.Unlock()

	return decision
}

func computeDecisionHash(d *EngineDecision) string {
	raw := fmt.Sprintf("%s:%s:%f:%f:%s:%d:%s",
		d.ID, d.Payload.Ticker, d.Payload.Quantity, d.Payload.Price,
		d.Outcome, d.Timestamp, d.PrevHash,
	)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// ==========================================
// HTTP GATEWAY ENDPOINTS
// ==========================================

func handleRiskCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method rejected", http.StatusMethodNotAllowed)
		return
	}
	var payload TradePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Malformed request taxonomy", http.StatusBadRequest)
		return
	}
	decision := evaluatePayload(payload)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(decision)
}

func handleGetEscalations(w http.ResponseWriter, r *http.Request) {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(EscalationQueue)
}

func handleResolveEscalation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method rejected", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/escalations/")
	id = strings.TrimSuffix(id, "/resolve")

	type ResolutionPayload struct {
		Action     string `json:"action"`
		Reviewer   string `json:"reviewer"`
		Credential string `json:"credential"`
		Reason     string `json:"reason"`
	}

	var p ResolutionPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Malformed resolution body", http.StatusBadRequest)
		return
	}
	if p.Reason == "" {
		http.Error(w, "Mandatory written compliance reason statement required", http.StatusBadRequest)
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	var targetDecision *EngineDecision
	foundIndex := -1
	for i, d := range EscalationQueue {
		if d.ID == id {
			targetDecision = d
			foundIndex = i
			break
		}
	}
	if targetDecision == nil {
		http.Error(w, "Escalation identifier lookup failure", http.StatusNotFound)
		return
	}

	EscalationQueue = append(EscalationQueue[:foundIndex], EscalationQueue[foundIndex+1:]...)

	res := HumanResolution{
		DecisionID: id,
		Action:     p.Action,
		Reviewer:   p.Reviewer,
		Credential: p.Credential,
		Reason:     p.Reason,
		Timestamp:  time.Now().UnixNano(),
		PrevHash:   LastChainHash,
	}
	raw := fmt.Sprintf("%s:%s:%s:%s:%d:%s",
		res.DecisionID, res.Action, res.Reviewer, res.Reason, res.Timestamp, res.PrevHash,
	)
	h := sha256.Sum256([]byte(raw))
	res.Hash = hex.EncodeToString(h[:])
	LastChainHash = res.Hash
	CryptographicChain = append(CryptographicChain, res.Hash)
	ResolutionLogs = append(ResolutionLogs, res)

	if p.Action == "REJECT" {
		executeEmergencyFlatten(targetDecision.Payload.Ticker, "Human review denial tracking event.")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "RESOLVED", "hash": res.Hash})
}

func executeEmergencyFlatten(ticker, reason string) {
	fmt.Printf("[ALPACA BROKER ACTUATION] DELETE /v2/positions/%s triggered. Trace Rationale: %s\n", ticker, reason)
}

func main() {
	envMode := os.Getenv("ARBITER_ENV")
	if envMode == "PROD" {
		fmt.Println("[ROADMAP NOTICE] AWS Nitro hardware enclaves / eBPF kernel hooks are flagged under the engineering roadmap — not active in this build.")
	}

	http.HandleFunc("/v1/risk/check", handleRiskCheck)
	http.HandleFunc("/escalations", handleGetEscalations)
	http.HandleFunc("/escalations/", handleResolveEscalation)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	fmt.Println("Arbiter Core Production Node active on port :8080...")
	_ = http.ListenAndServe(":8080", nil)
}
