package guardrails

import "fmt"

// ResolveConfig validates and normalizes policy-based guardrails configs.
func ResolveConfig(cfg Config) (Config, error) {
	if !usesPolicyConfig(cfg) {
		return Config{}, fmt.Errorf("guardrails config must define groups or policies")
	}
	if err := validatePolicyConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// IsPolicyConfig reports whether a config uses the policy/group schema.
func IsPolicyConfig(cfg Config) bool {
	return usesPolicyConfig(cfg)
}

// StorageConfig normalizes configs before persisting them back to disk.
func StorageConfig(cfg Config) Config {
	return cfg
}

func usesPolicyConfig(cfg Config) bool {
	return len(cfg.Policies) > 0 || len(cfg.Groups) > 0
}

func validatePolicyConfig(cfg Config) error {
	groupByID := make(map[string]PolicyGroup, len(cfg.Groups))
	for _, group := range cfg.Groups {
		if group.ID == "" {
			return fmt.Errorf("policy group id is required")
		}
		if _, exists := groupByID[group.ID]; exists {
			return fmt.Errorf("duplicate policy group id: %s", group.ID)
		}
		groupByID[group.ID] = group
	}

	for _, policy := range cfg.Policies {
		if _, err := buildPolicyEvaluator(policy, groupByID); err != nil {
			return err
		}
	}

	return nil
}

func buildPolicyEvaluator(policy Policy, groups map[string]PolicyGroup) (Evaluator, error) {
	if policy.ID == "" {
		return nil, fmt.Errorf("policy id is required")
	}

	group, err := resolvePolicyGroup(policy, groups)
	if err != nil {
		return nil, err
	}

	switch policy.Kind {
	case PolicyKindResourceAccess, PolicyKindOperationLegacy:
		return buildResourceAccessPolicyEvaluator(policy, group)
	case PolicyKindCommandExecution:
		return buildCommandExecutionPolicyEvaluator(policy, group)
	case PolicyKindContent:
		return buildContentPolicyEvaluator(policy, group)
	default:
		return nil, fmt.Errorf("policy %s: unsupported kind %q", policy.ID, policy.Kind)
	}
}

func resolvePolicyGroup(policy Policy, groups map[string]PolicyGroup) (*PolicyGroup, error) {
	if policy.Group == "" {
		return nil, nil
	}
	group, ok := groups[policy.Group]
	if !ok {
		return nil, fmt.Errorf("policy %s: unknown group %q", policy.ID, policy.Group)
	}
	return &group, nil
}

func buildResourceAccessPolicyEvaluator(policy Policy, group *PolicyGroup) (Evaluator, error) {
	scope := mergePolicyScope(group, policy.Scope)
	scope.Content = []ContentType{ContentTypeCommand}

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
	params.Verdict = resolvePolicyVerdict(policy, group, VerdictBlock)
	params.Reason = policy.Reason

	return NewOperationPolicy(
		policy.ID,
		policyName(policy),
		resolvePolicyEnabled(policy, group),
		scope,
		params,
	)
}

func buildCommandExecutionPolicyEvaluator(policy Policy, group *PolicyGroup) (Evaluator, error) {
	scope := mergePolicyScope(group, policy.Scope)
	scope.Content = []ContentType{ContentTypeCommand}

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
	params.Verdict = resolvePolicyVerdict(policy, group, VerdictBlock)
	params.Reason = policy.Reason

	return NewOperationPolicy(
		policy.ID,
		policyName(policy),
		resolvePolicyEnabled(policy, group),
		scope,
		params,
	)
}

func buildContentPolicyEvaluator(policy Policy, group *PolicyGroup) (Evaluator, error) {
	if len(policy.Match.Patterns) == 0 && len(policy.Match.CredentialRefs) == 0 {
		return nil, fmt.Errorf("policy %s: content policies require patterns or credential refs", policy.ID)
	}

	contentTypes := []ContentType{ContentTypeText}
	scope := mergePolicyScope(group, policy.Scope)
	scope.Content = contentTypes

	params := TextMatchConfig{
		Patterns:       append([]string(nil), policy.Match.Patterns...),
		CredentialRefs: append([]string(nil), policy.Match.CredentialRefs...),
		Targets:        contentTypes,
		Verdict:        resolvePolicyVerdict(policy, group, VerdictBlock),
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
		resolvePolicyEnabled(policy, group),
		scope,
		params,
	)
}

func mergePolicyScope(group *PolicyGroup, policyScope Scope) Scope {
	if group == nil {
		return policyScope
	}
	scope := policyScope
	if len(scope.Scenarios) == 0 {
		scope.Scenarios = append([]string(nil), group.DefaultScope.Scenarios...)
	}
	if len(scope.Models) == 0 {
		scope.Models = append([]string(nil), group.DefaultScope.Models...)
	}
	if len(scope.Directions) == 0 {
		scope.Directions = append([]Direction(nil), group.DefaultScope.Directions...)
	}
	if len(scope.Tags) == 0 {
		scope.Tags = append([]string(nil), group.DefaultScope.Tags...)
	}
	if len(scope.Content) == 0 {
		scope.Content = append([]ContentType(nil), group.DefaultScope.Content...)
	}
	return scope
}

func resolvePolicyEnabled(policy Policy, group *PolicyGroup) bool {
	policyEnabled := true
	if policy.Enabled != nil {
		policyEnabled = *policy.Enabled
	}
	if group == nil || group.Enabled == nil {
		return policyEnabled
	}
	return *group.Enabled && policyEnabled
}

func resolvePolicyVerdict(policy Policy, group *PolicyGroup, fallback Verdict) Verdict {
	if policy.Verdict != "" {
		return policy.Verdict
	}
	if group != nil && group.DefaultVerdict != "" {
		return group.DefaultVerdict
	}
	return fallback
}

func policyName(policy Policy) string {
	if policy.Name != "" {
		return policy.Name
	}
	return policy.ID
}
