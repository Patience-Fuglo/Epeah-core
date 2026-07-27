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

// Portfolio holds current $ exposure per ticker for already-executed positions.
// Seeded here for demonstration; in production this would be read from the
// fund's live position feed rather than tracked in-process.
var Portfolio = map[string]float64{
	"TSLA": 32000.0,
	"RIVN": 28000.0,
}

// SectorMap groups tickers so concentration can be detected across
// correlated names, not just within a single ticker.
var SectorMap = map[string]string{
	"TSLA": "EV",
	"RIVN": "EV",
	"NIO":  "EV",
	"LCID": "EV",
}

// ==========================================
// GRADUATED DECISION MATRIX
// ==========================================

func evaluatePayload(payload TradePayload) *EngineDecision {
	env := envelopeFor(payload.AgentID)

	decision := &EngineDecision{
		ID:              fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		Payload:         payload,
		Outcome:         "PASS",
		ConfidenceScore: 100,
		RuleDetails:     make([]RuleResult, 0),
		Timestamp:       time.Now().UnixNano(),
	}

	// Rule 1: Banned Registry — sourced from this agent's Autonomy Envelope,
	// not a global constant, so different agents can carry different
	// restricted-instrument lists.
	bannedRule := RuleResult{
		RuleName: "BannedAssetCheck",
		Severity: "OK",
		Reason:   "Asset is cleared for transaction tracking.",
	}
	if isRestrictedUnder(env, payload.Ticker) {
		bannedRule.Triggered = true
		bannedRule.Severity = "HARD"
		bannedRule.Reason = fmt.Sprintf("Asset %s is explicitly restricted from autonomous deployment.", payload.Ticker)
		decision.ConfidenceScore -= 50
	}
	decision.RuleDetails = append(decision.RuleDetails, bannedRule)

	// Rule 2: Position Allocation Sizing — thresholds come from the
	// agent's envelope (MaxOrderValue, SoftEscalationFraction).
	calculatedValue := payload.Quantity * payload.Price
	if payload.TotalValue > 0 {
		calculatedValue = payload.TotalValue
	}

	sizeRule := RuleResult{
		RuleName: "GraduatedSizeCheck",
		Severity: "OK",
		Reason:   "Position parameters fall within standard allocations.",
	}
	softThreshold := env.MaxOrderValue * env.SoftEscalationFraction
	if calculatedValue > env.MaxOrderValue {
		sizeRule.Triggered = true
		sizeRule.Severity = "HARD"
		sizeRule.Reason = fmt.Sprintf(
			"Trade value of $%.2f exceeds the $%.2f hard boundary cap.",
			calculatedValue, env.MaxOrderValue,
		)
		decision.ConfidenceScore -= 40
	} else if calculatedValue > softThreshold {
		sizeRule.Triggered = true
		sizeRule.Severity = "SOFT"
		sizeRule.Reason = fmt.Sprintf(
			"Trade value of $%.2f enters the borderline risk escalation band (> $%.2f).",
			calculatedValue, softThreshold,
		)
		decision.ConfidenceScore -= 15
	}
	decision.RuleDetails = append(decision.RuleDetails, sizeRule)

	// Rule 3: Concentration Risk — thresholds come from the agent's
	// envelope (MaxSingleTickerExposure, MaxSectorExposure). This is the
	// rule that can escalate a trade EVEN WHEN it passes every check
	// above. A trade can be a normal size on an unrestricted ticker
	// (fully "authorized" by rules 1 and 2) and still push the fund into
	// dangerous concentration once portfolio context is considered.
	// Static permission checks can't see this; Epeah can.
	stateMutex.Lock()
	existingTickerExposure := Portfolio[strings.ToUpper(payload.Ticker)]
	sector, hasSector := SectorMap[strings.ToUpper(payload.Ticker)]
	sectorExposure := 0.0
	if hasSector {
		for ticker, value := range Portfolio {
			if SectorMap[ticker] == sector {
				sectorExposure += value
			}
		}
	}
	stateMutex.Unlock()

	projectedTickerExposure := existingTickerExposure + calculatedValue
	projectedSectorExposure := sectorExposure + calculatedValue

	concentrationRule := RuleResult{
		RuleName: "ConcentrationRiskCheck",
		Severity: "OK",
		Reason:   "Portfolio concentration remains within acceptable bounds.",
	}
	if projectedTickerExposure > env.MaxSingleTickerExposure {
		concentrationRule.Triggered = true
		concentrationRule.Severity = "SOFT"
		concentrationRule.Reason = fmt.Sprintf(
			"This trade would bring %s exposure to $%.2f, above the $%.2f single-position concentration limit.",
			strings.ToUpper(payload.Ticker), projectedTickerExposure, env.MaxSingleTickerExposure,
		)
		decision.ConfidenceScore -= 20
	} else if hasSector && projectedSectorExposure > env.MaxSectorExposure {
		concentrationRule.Triggered = true
		concentrationRule.Severity = "SOFT"
		concentrationRule.Reason = fmt.Sprintf(
			"This trade is authorized on its own, but combined %s-sector exposure would reach $%.2f, above the $%.2f sector concentration limit — a human should weigh whether this correlated risk is acceptable.",
			sector, projectedSectorExposure, env.MaxSectorExposure,
		)
		decision.ConfidenceScore -= 20
	}
	decision.RuleDetails = append(decision.RuleDetails, concentrationRule)

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
	if decision.Outcome == "PASS" {
		// Auto-cleared trades execute immediately, so portfolio exposure
		// updates right away — this is what the NEXT trade's concentration
		// check will see.
		Portfolio[strings.ToUpper(payload.Ticker)] += calculatedValue
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
	} else if p.Action == "ALLOW" {
		calculatedValue := targetDecision.Payload.TotalValue
		if calculatedValue == 0 {
			calculatedValue = targetDecision.Payload.Quantity * targetDecision.Payload.Price
		}
		Portfolio[strings.ToUpper(targetDecision.Payload.Ticker)] += calculatedValue
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "RESOLVED", "hash": res.Hash})
}

func executeEmergencyFlatten(ticker, reason string) {
	fmt.Printf("[ALPACA BROKER ACTUATION] DELETE /v2/positions/%s triggered. Trace Rationale: %s\n", ticker, reason)
}

func main() {
	LoadEnvelopes("envelopes.json")

	envMode := os.Getenv("EPEAH_ENV")
	if envMode == "PROD" {
		fmt.Println("[ROADMAP NOTICE] AWS Nitro hardware enclaves / eBPF kernel hooks are flagged under the engineering roadmap — not active in this build.")
	}

	http.HandleFunc("/v1/risk/check", handleRiskCheck)
	http.HandleFunc("/escalations", handleGetEscalations)
	http.HandleFunc("/escalations/", handleResolveEscalation)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	fmt.Println("Epeah Core Production Node active on port :8080...")
	_ = http.ListenAndServe(":8080", nil)
}
