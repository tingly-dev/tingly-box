package protocoltest

// The harness wires every model through the product's one routing primitive,
// typ.Rule — same as production configuration. These helpers are the single
// place the canonical fixture shape lives (weight-1 active services with a
// 300s stats window, random LB tactic, active rule), so the setup sites
// across matrix / replay / agent / flags / failover / duo cannot drift apart
// field by field. Callers needing a different tactic or flags override the
// returned rule's fields.

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// harnessService returns a weight-1, active service with the harness's
// standard 300s stats window.
func harnessService(providerUUID, model string) *loadbalance.Service {
	return &loadbalance.Service{Provider: providerUUID, Model: model, Weight: 1, Active: true, TimeWindow: 300}
}

// tieredService is harnessService pinned to a tier (failover rules).
func tieredService(providerUUID, model string, tier int) *loadbalance.Service {
	s := harnessService(providerUUID, model)
	s.Tier = tier
	return s
}

// newHarnessRule returns an active rule with the harness's canonical
// defaults (random LB tactic). uuid usually equals requestModel (the fixture
// convention) or is a built-in rule UUID; "" lets the rule API assign one.
func newHarnessRule(uuid string, scenario typ.RuleScenario, requestModel, responseModel string, services ...*loadbalance.Service) typ.Rule {
	return typ.Rule{
		UUID:          uuid,
		Scenario:      scenario,
		RequestModel:  requestModel,
		ResponseModel: responseModel,
		Services:      services,
		LBTactic:      typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
		Active:        true,
	}
}

// tierFailoverTactic is the two-tier tactic the failover suite dispatches
// under: tiers in order, random within a tier.
func tierFailoverTactic() typ.Tactic {
	return typ.Tactic{Type: loadbalance.TacticTier, Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom}}
}

// BuiltinRuleRef returns the agent's built-in rule UUID and its fixed
// RequestModel — the one copy of the mapping shared by every setup path and
// the CLI reporting layer.
func BuiltinRuleRef(at AgentType) (uuid, requestModel string, err error) {
	switch at {
	case AgentTypeClaudeCode:
		return serverconfig.RuleUUIDCC, "tingly/cc", nil
	case AgentTypeCodex:
		return serverconfig.RuleUUIDCodex, "tingly-codex", nil
	case AgentTypeOpenCode:
		return serverconfig.RuleUUIDOpenCode, "tingly-opencode", nil
	default:
		return "", "", fmt.Errorf("unknown Agent type: %s", at)
	}
}

// BuiltinRequestModel returns the fixed RequestModel the agent's built-in
// rule matches — what the agent CLI actually sends ("" for unknown agents).
func BuiltinRequestModel(at AgentType) string {
	_, m, _ := BuiltinRuleRef(at)
	return m
}
