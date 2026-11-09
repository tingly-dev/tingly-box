package servertest

import (
	"fmt"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// svcState is one service's full runtime state: config (Active), health
// monitor (429/auth channel), and circuit breaker (5xx channel).
type svcState struct {
	active    bool
	unhealthy bool
	tripped   bool
}

func (s svcState) String() string {
	code := func(b bool, y, n byte) byte {
		if b {
			return y
		}
		return n
	}
	return string([]byte{code(s.active, 'A', 'a'), code(s.unhealthy, 'U', 'u'), code(s.tripped, 'T', 't')})
}

func allSvcStates() []svcState {
	out := make([]svcState, 0, 8)
	for _, active := range []bool{true, false} {
		for _, unhealthy := range []bool{true, false} {
			for _, tripped := range []bool{true, false} {
				out = append(out, svcState{active, unhealthy, tripped})
			}
		}
	}
	return out
}

// TestExhaustive_SelectionDegradeInvariant enumerates every combination of
// service runtime state across the selection pipeline's dimensions and asserts
// the degrade invariant on the REAL stack (ServiceSelector + LoadBalancer +
// HealthFilter + breaker store + affinity store):
//
//	If the scope a request resolves to (base pool for non-matching requests,
//	base ∪ partition for partition-matching requests) contains at least one
//	ACTIVE service, selection must return an active service from that scope —
//	no combination of health-monitor, breaker, or affinity-pin state may fail
//	the rule or leak a selection outside the scope. Only a scope with zero
//	active services may error (a genuine config problem).
//
// Dimensions:
//   - 1 or 2 base services × each service in 8 states (Active × unhealthy ×
//     breaker-tripped)
//   - no smart partition | 1 partition service in 8 states
//   - request: does not match any partition | matches the partition
//   - session affinity: no pin | a live pin on each of the case's services
//     (including pins to inactive or out-of-scope services)
//   - tactic: random | tier
func TestExhaustive_SelectionDegradeInvariant(t *testing.T) {
	// The breaker store is process-global; this test trips thousands of
	// (rule, service) breakers, so start clean and leave clean.
	loadbalance.DefaultBreakerStore().Reset()
	t.Cleanup(loadbalance.DefaultBreakerStore().Reset)

	cfg := newTestGlobalConfig(t)
	providers := map[string]string{
		"base1": addTestProvider(t, cfg, "base1"),
		"base2": addTestProvider(t, cfg, "base2"),
		"part":  addTestProvider(t, cfg, "part"),
	}

	// An op that matches every request (token count >= 0).
	alwaysMatchOp := smartrouting.SmartOp{
		Position:  smartrouting.PositionToken,
		Operation: smartrouting.OpTokenGe,
		Value:     "0",
	}
	matchingReq := &anthropic.MessageNewParams{
		Model:    "req-model",
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))},
	}

	states := allSvcStates()
	tactics := map[string]typ.Tactic{
		"random": {Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
		"tier":   {Type: loadbalance.TacticTier, Params: typ.DefaultTierParams()},
	}

	// All 1- and 2-service base pools.
	baseCombos := make([][]svcState, 0, len(states)*(len(states)+1))
	for _, b1 := range states {
		baseCombos = append(baseCombos, []svcState{b1})
	}
	for _, b1 := range states {
		for _, b2 := range states {
			baseCombos = append(baseCombos, []svcState{b1, b2})
		}
	}

	// partitionStates[0] == nil means "no partition".
	partitionStates := make([]*svcState, 0, len(states)+1)
	partitionStates = append(partitionStates, nil)
	for i := range states {
		partitionStates = append(partitionStates, &states[i])
	}

	caseCount := 0
	// pinIdx: -1 = no pin; 0..len(base)-1 = pin to that base service;
	// len(base) = pin to the partition service.
	runCase := func(t *testing.T, tactic typ.Tactic, baseStates []svcState, partState *svcState, requestMatches bool, pinIdx int) {
		caseCount++
		ruleUUID := fmt.Sprintf("ex-%d", caseCount)

		baseSvcs := make([]*loadbalance.Service, len(baseStates))
		for i, st := range baseStates {
			baseSvcs[i] = routing.ServiceForTest(providers[fmt.Sprintf("base%d", i+1)], fmt.Sprintf("base-model-%d", i+1), st.active)
		}
		rule := &typ.Rule{
			Scenario:     typ.ScenarioClaudeCode,
			RequestModel: "req-model",
			UUID:         ruleUUID,
			LBTactic:     tactic,
			Services:     baseSvcs,
			Active:       true,
		}
		var partSvc *loadbalance.Service
		if partState != nil {
			partSvc = routing.ServiceForTest(providers["part"], "part-model", partState.active)
			rule.SmartEnabled = true
			rule.SmartRouting = []smartrouting.SmartRouting{{
				Description: "always matches",
				Ops:         []smartrouting.SmartOp{alwaysMatchOp},
				Services:    []*loadbalance.Service{partSvc},
			}}
		}

		healthMonitor, affinity, selector := newSelectorStack(cfg)

		applyState := func(svc *loadbalance.Service, st svcState) {
			if st.unhealthy {
				healthMonitor.ReportRateLimit(svc.ServiceID())
			}
			if st.tripped {
				for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
					loadbalance.RecordServiceFailure(ruleUUID, svc.ServiceID())
				}
			}
		}
		for i, st := range baseStates {
			applyState(baseSvcs[i], st)
		}
		if partState != nil {
			applyState(partSvc, *partState)
		}

		ctx := &routing.SelectionContext{Rule: rule, MatchedSmartRuleIndex: -1}
		if requestMatches {
			ctx.Request = matchingReq
		}

		if pinIdx >= 0 {
			pinSvc := partSvc
			if pinIdx < len(baseSvcs) {
				pinSvc = baseSvcs[pinIdx]
			}
			rule.Flags.SessionAffinity = 1800
			ctx.SessionID = typ.SessionID{Source: typ.SessionSourceHeader, Value: "sess"}
			// Seed the pin under both the bare session key and the partition
			// scope so matching and non-matching requests both see it.
			for _, key := range []string{
				ctx.SessionID.String(),
				routing.AffinitySessionKey(ctx.SessionID.String(), 0),
			} {
				affinity.Set(ruleUUID, key, &routing.AffinityEntry{
					Service:   pinSvc,
					LockedAt:  time.Now(),
					ExpiresAt: time.Now().Add(time.Hour),
				})
			}
		}

		// The scope this request resolves to. Non-matching requests must be
		// served from the base pool; matching requests may be served from the
		// partition or degrade back to the base pool.
		scope := baseSvcs
		if requestMatches && partState != nil {
			scope = append(append([]*loadbalance.Service{}, baseSvcs...), partSvc)
		}
		activeInScope := len(routing.FilterActiveServices(scope))

		res, err := selector.Select(ctx)
		if activeInScope == 0 {
			require.Error(t, err, "no active service in scope is a config error and must be reported")
			return
		}
		require.NoErrorf(t, err,
			"selection must not fail while %d active services are in scope", activeInScope)
		require.NotNil(t, res)
		require.True(t, res.Service.Active, "an inactive service must never be selected")
		require.Truef(t, routing.ContainsService(scope, res.Service),
			"selected %s outside the request's scope", res.Service.ServiceID())
	}

	for tacticName, tactic := range tactics {
		for _, combo := range baseCombos {
			for _, part := range partitionStates {
				matchModes := []bool{false}
				pinCount := len(combo)
				if part != nil {
					matchModes = []bool{false, true}
					pinCount++
				}
				for _, matches := range matchModes {
					for pinIdx := -1; pinIdx < pinCount; pinIdx++ {
						name := fmt.Sprintf("%s/base=%v/part=%v/match=%v/pin=%d", tacticName, combo, part, matches, pinIdx)
						t.Run(name, func(t *testing.T) {
							runCase(t, tactic, combo, part, matches, pinIdx)
						})
					}
				}
			}
		}
	}
	t.Logf("exhaustive cases: %d", caseCount)
}
