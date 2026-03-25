package routing

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/server"
)

// AffinityStage checks if a session has a locked service from previous requests.
// If found and valid, returns the locked service; otherwise passes to next stage.
type AffinityStage struct {
	store *server.AffinityStore
	scope string // "global" or "smart_rule"
}

// NewAffinityStage creates a new affinity stage with the given store and scope
func NewAffinityStage(store *server.AffinityStore, scope string) *AffinityStage {
	return &AffinityStage{
		store: store,
		scope: scope,
	}
}

// Name returns the stage identifier
func (s *AffinityStage) Name() string {
	return "affinity"
}

// Evaluate checks for locked service affinity
func (s *AffinityStage) Evaluate(ctx *SelectionContext) (*SelectionResult, bool) {
	rule := ctx.Rule

	// Skip if affinity not enabled
	if !rule.SmartEnabled || !rule.SmartAffinity || ctx.SessionID == "" {
		return nil, false
	}

	// For smart_rule scope, we need the matched rule index
	// If we're evaluating affinity BEFORE smart routing, we can't use smart_rule scope
	if s.scope == "smart_rule" && ctx.MatchedSmartRuleIndex < 0 {
		// Smart routing hasn't run yet, can't check smart_rule-scoped affinity
		return nil, false
	}

	// Check affinity store
	// Currently AffinityStore only supports global scope (ruleUUID:sessionID)
	// TODO: Extend AffinityStore to support smart_rule scope keys
	entry, ok := s.store.Get(rule.UUID, ctx.SessionID)
	if !ok {
		return nil, false
	}

	logrus.Infof("[affinity] using locked service for session %s: %s",
		ctx.SessionID, entry.Service.Model)

	result := NewResult(entry.Service, "affinity")
	result.MatchedSmartRuleIndex = ctx.MatchedSmartRuleIndex
	return result, true
}

// makeGlobalKey creates a global affinity key
func (s *AffinityStage) makeGlobalKey(ruleUUID, sessionID string) string {
	return ruleUUID + ":" + sessionID
}

// makeSmartRuleKey creates a smart rule scoped affinity key
func (s *AffinityStage) makeSmartRuleKey(ruleUUID, sessionID string, smartRuleIndex int) string {
	// Format: ruleUUID:sessionID:sr{index}
	return ruleUUID + ":" + sessionID + ":sr" + string(rune('0'+smartRuleIndex))
}
