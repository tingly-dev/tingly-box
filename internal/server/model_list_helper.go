package server

import (
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ShouldIncludeRuleInModelList reports whether a rule should appear in the
// model list for the requested scenario. Each scenario — base or profiled —
// is an isolated scope: it only lists rules bound to that exact scenario.
// Transport compatibility does not grant cross-scenario visibility.
func ShouldIncludeRuleInModelList(requestedScenario typ.RuleScenario, ruleScenario typ.RuleScenario) bool {
	return requestedScenario == ruleScenario
}

// PrimaryAuthTypeForRule returns the AuthType of the first active service's
// provider in a rule. It is used by /v1/models endpoints so the frontend can
// order picker entries oauth -> api_key -> vmodel.
//
// Returns AuthTypeAPIKey as the fallback for empty/unresolvable rules so they
// land in the middle group rather than at the head or tail.
func PrimaryAuthTypeForRule(cfg *config.Config, rule typ.Rule) typ.AuthType {
	if cfg == nil {
		return typ.AuthTypeAPIKey
	}
	for _, svc := range rule.GetServices() {
		if svc == nil || !svc.Active {
			continue
		}
		provider, err := cfg.GetProviderByUUID(svc.Provider)
		if err != nil || provider == nil {
			continue
		}
		switch provider.AuthType {
		case typ.AuthTypeOAuth, typ.AuthTypeAPIKey, typ.AuthTypeVirtual:
			return provider.AuthType
		default:
			return typ.AuthTypeAPIKey
		}
	}
	return typ.AuthTypeAPIKey
}

// AuthTypeSortWeight ranks auth types for /v1/models ordering:
// oauth (0) -> api_key (1) -> vmodel (2). Any unknown value is treated as
// api_key so legacy entries cluster with regular providers.
func AuthTypeSortWeight(a typ.AuthType) int {
	switch a {
	case typ.AuthTypeOAuth:
		return 0
	case typ.AuthTypeVirtual:
		return 2
	default:
		return 1
	}
}
