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
