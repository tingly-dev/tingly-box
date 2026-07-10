package routing

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/typ"
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
	return SourceAffinity
}

// Evaluate checks for locked service affinity
func (s *AffinityStage) Evaluate(ctx *SelectionContext, state *selectionState) (*SelectionResult, bool) {
	rule := ctx.Rule

	// Skip if affinity not enabled. Affinity is a load-balancing concern and
	// is independent of smart routing — it applies whenever a session can be
	// identified, regardless of rule.SmartEnabled.
	if !rule.AffinityEnabled() || ctx.SessionID.IsEmpty() {
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
	entry, ok := s.store.Get(rule.UUID, ctx.SessionID.String())
	if !ok {
		return nil, false
	}

	// Strict TTL: honor the pin only while the entry has not expired.
	// Once the lock expires, the session must re-enter the selection pipeline
	// and postProcess will create a new lock with a fresh TTL.
	if clock.Now().After(entry.ExpiresAt) {
		logrus.Infof("[affinity] affinity entry for session %s expired at %s; dropping pin so strategy re-selects",
			ctx.SessionID.String(), entry.ExpiresAt)
		return nil, false
	}

	logrus.Infof("[affinity] using locked service for session %s: %s",
		ctx.SessionID.String(), entry.Service.Model)

	if state != nil && len(state.candidateServices) > 0 && !ContainsService(state.candidateServices, entry.Service) {
		logrus.Debugf("[affinity] locked service %s not in candidate set, skipping",
			entry.Service.ServiceID())
		return nil, false
	}

	// Health scoping (breaker-aware): only honor a pin while the locked service
	// is one the strategy would actually pick right now. This is driven by the
	// rule's config shape, not its tactic label — "tier" is just the emergent
	// shape of a multi-layer rule:
	//   - many layers: drop a pin to a fallback tier once the primary recovers.
	//   - one layer, many services: drop a pin to a dead peer when healthy
	//     peers exist.
	//   - one service: always honored (nothing else to pick).
	// On decline the pipeline falls through to the strategy, which re-selects a
	// currently-valid service, and postProcess re-pins the session there.
	if state != nil && len(state.candidateServices) > 0 &&
		!typ.IsAffinityEligible(rule.UUID, state.candidateServices, entry.Service) {
		logrus.Infof("[affinity] locked service %s is not currently selectable for session %s; dropping pin so strategy re-selects",
			entry.Service.ServiceID(), ctx.SessionID.String())
		return nil, false
	}

	result := NewResult(entry.Service, SourceAffinity)
	result.MatchedSmartRuleIndex = ctx.MatchedSmartRuleIndex
	return result, true
}
