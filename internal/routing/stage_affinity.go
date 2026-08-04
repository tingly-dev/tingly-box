package routing

import (
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AffinityStage checks if a session has a locked service from previous requests.
// If found and valid, returns the locked service; otherwise passes to next stage.
//
// It runs AFTER SmartRoutingStage, so pins are scoped to the content partition
// smart routing matched (see AffinitySessionKey): a session pinned inside one
// smart subset cannot drag requests that match a different subset — content
// routing decides the partition, affinity provides stickiness within it.
type AffinityStage struct {
	store AffinityStore
}

// NewAffinityStage creates a new affinity stage backed by the given store
func NewAffinityStage(store AffinityStore) *AffinityStage {
	return &AffinityStage{store: store}
}

// AffinitySessionKey returns the affinity-store session key scoped to the
// content partition smart routing matched: the bare session for the rule's
// top-level pool (no match, index -1), or session + "#sr<idx>" inside a smart
// subset. Partition-scoping is what lets one session hold independent pins
// per request kind (e.g. a Claude Code main pin and a subagent pin), each
// with its own prompt-cache continuity.
//
// The partition identity is the smart rule's index, the only stable handle
// the config offers. Editing the rule list can renumber partitions and
// mis-bucket existing pins; pins are in-memory with short TTLs, so this
// self-heals within one TTL.
func AffinitySessionKey(sessionID string, matchedSmartRuleIndex int) string {
	if matchedSmartRuleIndex < 0 {
		return sessionID
	}
	return sessionID + "#sr" + strconv.Itoa(matchedSmartRuleIndex)
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

	// Look up the pin in the partition smart routing just matched (or the
	// top-level partition when nothing matched).
	entry, ok := s.store.Get(rule.UUID, AffinitySessionKey(ctx.SessionID.String(), ctx.MatchedSmartRuleIndex))
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
