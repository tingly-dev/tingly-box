package evaluate

import (
	"fmt"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// Dependencies provides external services needed by some policy kinds.
type Dependencies struct {
	Judge Judge
}

// BuildEvaluators instantiates runtime evaluators from policy configuration.
func BuildEvaluators(cfg guardrailscore.Config, deps Dependencies) ([]guardrailscore.Evaluator, error) {
	resolvedCfg, err := ResolveConfig(cfg)
	if err != nil {
		return nil, err
	}
	groupByID := make(map[string]guardrailscore.PolicyGroup, len(resolvedCfg.Groups))
	for _, group := range resolvedCfg.Groups {
		groupByID[group.ID] = group
	}
	evaluators := make([]guardrailscore.Evaluator, 0, len(resolvedCfg.Policies))
	for _, policy := range resolvedCfg.Policies {
		evaluator, err := buildPolicyEvaluator(policy, groupByID, deps)
		if err != nil {
			return nil, err
		}
		evaluators = append(evaluators, evaluator)
	}
	return evaluators, nil
}

func buildPolicyEvaluator(policy guardrailscore.Policy, groups map[string]guardrailscore.PolicyGroup, deps Dependencies) (guardrailscore.Evaluator, error) {
	if policy.ID == "" {
		return nil, fmt.Errorf("policy id is required")
	}

	policyGroups, err := resolvePolicyGroups(policy, groups)
	if err != nil {
		return nil, err
	}

	switch policy.Kind {
	case guardrailscore.PolicyKindResourceAccess, guardrailscore.PolicyKindOperationLegacy:
		return buildResourceAccessPolicyEvaluator(policy, policyGroups)
	case guardrailscore.PolicyKindCommandExecution:
		return buildCommandExecutionPolicyEvaluator(policy, policyGroups)
	case guardrailscore.PolicyKindContent:
		return buildContentPolicyEvaluator(policy, policyGroups)
	default:
		return nil, fmt.Errorf("policy %s: unsupported kind %q", policy.ID, policy.Kind)
	}
}

func normalizedPolicyGroups(policy guardrailscore.Policy) []string {
	seen := make(map[string]struct{}, len(policy.Groups))
	out := make([]string, 0, len(policy.Groups))
	for _, groupID := range policy.Groups {
		if groupID == "" {
			continue
		}
		if _, exists := seen[groupID]; exists {
			continue
		}
		seen[groupID] = struct{}{}
		out = append(out, groupID)
	}
	return out
}

func resolvePolicyGroups(policy guardrailscore.Policy, groups map[string]guardrailscore.PolicyGroup) ([]guardrailscore.PolicyGroup, error) {
	ids := normalizedPolicyGroups(policy)
	if len(ids) == 0 {
		return nil, nil
	}
	resolved := make([]guardrailscore.PolicyGroup, 0, len(ids))
	for _, groupID := range ids {
		group, ok := groups[groupID]
		if !ok {
			return nil, fmt.Errorf("policy %s: unknown group %q", policy.ID, groupID)
		}
		resolved = append(resolved, group)
	}
	return resolved, nil
}

func buildResourceAccessPolicyEvaluator(policy guardrailscore.Policy, groups []guardrailscore.PolicyGroup) (guardrailscore.Evaluator, error) {
	scope := mergePolicyScope(policy.Scope)
	scope.Content = []guardrailscore.ContentType{guardrailscore.ContentTypeCommand}

	params := CommandPolicyConfig{
		ToolNames: append([]string(nil), policy.Match.ToolNames...),
		Terms:     append([]string(nil), policy.Match.Terms...),
	}
	if policy.Match.Actions != nil {
		params.Actions = append([]string(nil), policy.Match.Actions.Include...)
	}
	if policy.Match.Resources != nil {
		params.Resources = append([]string(nil), policy.Match.Resources.Values...)
		params.ResourceMatch = ResourceMatchMode(policy.Match.Resources.Mode)
	}
	params.Verdict = resolvePolicyVerdict(policy, guardrailscore.VerdictBlock)
	params.Reason = policy.Reason

	return NewOperationPolicy(
		policy.ID,
		policyName(policy),
		resolvePolicyEnabled(policy, groups),
		scope,
		params,
	)
}

func buildCommandExecutionPolicyEvaluator(policy guardrailscore.Policy, groups []guardrailscore.PolicyGroup) (guardrailscore.Evaluator, error) {
	scope := mergePolicyScope(policy.Scope)
	scope.Content = []guardrailscore.ContentType{guardrailscore.ContentTypeCommand}

	params := CommandPolicyConfig{
		ToolNames: append([]string(nil), policy.Match.ToolNames...),
		Terms:     append([]string(nil), policy.Match.Terms...),
		Actions:   []string{"execute"},
	}
	if policy.Match.Actions != nil && len(policy.Match.Actions.Include) > 0 {
		params.Actions = append([]string(nil), policy.Match.Actions.Include...)
	}
	if policy.Match.Resources != nil {
		params.Resources = append([]string(nil), policy.Match.Resources.Values...)
		params.ResourceMatch = ResourceMatchMode(policy.Match.Resources.Mode)
	}
	params.Verdict = resolvePolicyVerdict(policy, guardrailscore.VerdictBlock)
	params.Reason = policy.Reason

	return NewOperationPolicy(
		policy.ID,
		policyName(policy),
		resolvePolicyEnabled(policy, groups),
		scope,
		params,
	)
}

func buildContentPolicyEvaluator(policy guardrailscore.Policy, groups []guardrailscore.PolicyGroup) (guardrailscore.Evaluator, error) {
	if len(policy.Match.Patterns) == 0 && len(policy.Match.CredentialRefs) == 0 {
		return nil, fmt.Errorf("policy %s: content policies require patterns or credential refs", policy.ID)
	}

	contentTypes := []guardrailscore.ContentType{guardrailscore.ContentTypeText}
	scope := mergePolicyScope(policy.Scope)
	scope.Content = contentTypes

	params := TextMatchConfig{
		Patterns:       append([]string(nil), policy.Match.Patterns...),
		CredentialRefs: append([]string(nil), policy.Match.CredentialRefs...),
		Targets:        contentTypes,
		Verdict:        resolvePolicyVerdict(policy, guardrailscore.VerdictBlock),
		Mode:           MatchMode(policy.Match.MatchMode),
		MinMatches:     policy.Match.MinMatches,
		CaseSensitive:  policy.Match.CaseSensitive,
		Reason:         policy.Reason,
	}
	if policy.Match.PatternMode == "regex" {
		params.UseRegex = true
	}

	return NewContentPolicy(
		policy.ID,
		policyName(policy),
		resolvePolicyEnabled(policy, groups),
		scope,
		params,
	)
}

func mergePolicyScope(policyScope guardrailscore.Scope) guardrailscore.Scope {
	return policyScope
}

func resolvePolicyEnabled(policy guardrailscore.Policy, groups []guardrailscore.PolicyGroup) bool {
	policyEnabled := true
	if policy.Enabled != nil {
		policyEnabled = *policy.Enabled
	}
	if !policyEnabled {
		return false
	}
	if len(groups) == 0 {
		return false
	}
	for _, group := range groups {
		if group.Enabled == nil || *group.Enabled {
			return true
		}
	}
	return false
}

func resolvePolicyVerdict(policy guardrailscore.Policy, fallback guardrailscore.Verdict) guardrailscore.Verdict {
	if policy.Verdict != "" {
		return policy.Verdict
	}
	return fallback
}

func policyName(policy guardrailscore.Policy) string {
	if policy.Name != "" {
		return policy.Name
	}
	return policy.ID
}
