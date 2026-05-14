package server

import "github.com/tingly-dev/tingly-box/internal/typ"

// shouldIncludeRuleInModelList reports whether a rule should appear in the
// model list for the requested scenario. Each scenario — base or profiled —
// is an isolated scope: it only lists rules bound to that exact scenario.
// Transport compatibility does not grant cross-scenario visibility.
func shouldIncludeRuleInModelList(requestedScenario typ.RuleScenario, ruleScenario typ.RuleScenario) bool {
	return requestedScenario == ruleScenario
}
