package server

import "github.com/tingly-dev/tingly-box/internal/typ"

// isValidRuleScenario checks if the given scenario is a valid RuleScenario
func isValidRuleScenario(scenario typ.RuleScenario) bool {
	return typ.CanUseScenarioInPath(scenario)
}
