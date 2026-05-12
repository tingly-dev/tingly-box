package server

import "github.com/tingly-dev/tingly-box/internal/typ"

func scenarioSupportsTransport(scenario typ.RuleScenario, transport typ.ScenarioTransport) bool {
	descriptor, ok := typ.GetScenarioDescriptor(scenario)
	if !ok {
		return false
	}
	for _, supported := range descriptor.SupportedTransport {
		if supported == transport {
			return true
		}
	}
	return false
}

func shouldIncludeRuleInModelList(requestedScenario typ.RuleScenario, ruleScenario typ.RuleScenario) bool {
	// If requested scenario is a profile (e.g., "claude_code:p1"), only include exact matches.
	// Profiles are isolated scopes and should not fallback to transport-based reachability.
	if typ.IsProfiledScenario(requestedScenario) {
		return requestedScenario == ruleScenario
	}

	// If the rule belongs to a profiled scenario, it must not be visible to non-profile requests.
	// Profile rules are exclusively scoped to their own profile.
	if typ.IsProfiledScenario(ruleScenario) {
		return false
	}

	if requestedScenario == ruleScenario {
		return true
	}

	requestedDescriptor, ok := typ.GetScenarioDescriptor(requestedScenario)
	if !ok {
		return false
	}
	for _, transport := range requestedDescriptor.SupportedTransport {
		if scenarioSupportsTransport(ruleScenario, transport) {
			return true
		}
	}
	return false
}
