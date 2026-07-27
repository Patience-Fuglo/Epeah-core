package main

import (
	"encoding/json"
	"os"
	"strings"
)

// ==========================================
// AUTONOMY ENVELOPE
// ==========================================
//
// The Autonomy Envelope is the formal, explicit specification of what a
// given agent is authorized to do on its own — and where that authority
// ends and human judgment begins. It is the concrete artifact behind the
// architectural separation:
//
//   Identity -> Authorization -> Epeah Decision Control -> Execution -> Evidence
//
// A static authorization layer (e.g. an agent identity/permission system)
// answers "can this agent trade at all." The Autonomy Envelope goes one
// level deeper: for THIS agent specifically, what instruments, what size,
// what concentration, and what confidence band are within its independent
// authority — versus what must be escalated to a verified human, even
// when the action is otherwise permitted.
//
// Envelopes are per-agent so that different agents (a conservative
// execution bot vs. an experimental research agent) can carry different
// authority without changing engine code — only the envelope changes.

type AutonomyEnvelope struct {
	AgentID string `json:"agent_id"`

	// RestrictedInstruments are hard-banned regardless of size or context.
	RestrictedInstruments []string `json:"restricted_instruments"`

	// MaxOrderValue is the hard per-trade ceiling. Above this, BLOCK.
	MaxOrderValue float64 `json:"max_order_value"`

	// SoftEscalationFraction (of MaxOrderValue) marks the band where a
	// single trade is large enough to require human judgment, even
	// though it hasn't breached the hard cap.
	SoftEscalationFraction float64 `json:"soft_escalation_fraction"`

	// MaxSingleTickerExposure and MaxSectorExposure define concentration
	// limits — the portfolio-level context a static per-trade permission
	// check cannot see, but Epeah's Consequence Engine does.
	MaxSingleTickerExposure float64 `json:"max_single_ticker_exposure"`
	MaxSectorExposure       float64 `json:"max_sector_exposure"`
}

// DefaultEnvelope applies to any agent with no specific envelope on file.
// These values match Epeah's original hardcoded constants, so existing
// behavior is unchanged for any agent not explicitly configured otherwise.
var DefaultEnvelope = AutonomyEnvelope{
	AgentID:                 "default",
	RestrictedInstruments:   []string{"GHOST", "TEST"},
	MaxOrderValue:           50000.0,
	SoftEscalationFraction:  0.8,
	MaxSingleTickerExposure: 75000.0,
	MaxSectorExposure:       80000.0,
}

var envelopeRegistry = map[string]AutonomyEnvelope{}

// LoadEnvelopes reads per-agent envelopes from a JSON file (see
// envelopes.json for the format). Agents not present in the file simply
// fall back to DefaultEnvelope. Absence of the file is not an error —
// it just means every agent runs under the default envelope.
func LoadEnvelopes(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var list []AutonomyEnvelope
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	for _, e := range list {
		envelopeRegistry[e.AgentID] = e
	}
}

// envelopeFor returns the agent's specific envelope if one is on file,
// otherwise the default.
func envelopeFor(agentID string) AutonomyEnvelope {
	if e, ok := envelopeRegistry[strings.TrimSpace(agentID)]; ok {
		return e
	}
	return DefaultEnvelope
}

func isRestrictedUnder(env AutonomyEnvelope, ticker string) bool {
	t := strings.ToUpper(ticker)
	for _, r := range env.RestrictedInstruments {
		if strings.ToUpper(r) == t {
			return true
		}
	}
	return false
}
