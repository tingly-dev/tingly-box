package routing

import (
	"github.com/sirupsen/logrus"
)

// AffinityStage checks if a session has a locked service from previous requests.
// If found and valid, returns the locked service; otherwise passes to next stage.
type AffinityStage struct {
	store AffinityStore
	scope string // "global" or "smart_rule"
}

// NewAffinityStage creates a new affinity stage with the given store and scope
func NewAffinityStage(store AffinityStore, scope string) *AffinityStage {
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
func (s *AffinityStage) Evaluate(ctx *SelectionContext, state *selectionState) (*SelectionResult, bool) {
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

	if state != nil && len(state.candidateServices) > 0 && !ContainsService(state.candidateServices, entry.Service) {
		logrus.Debugf("[affinity] locked service %s not in candidate set, skipping",
			entry.Service.ServiceID())
		return nil, false
	}

	result := NewResult(entry.Service, "affinity")
	result.MatchedSmartRuleIndex = ctx.MatchedSmartRuleIndex
	return result, true
}
