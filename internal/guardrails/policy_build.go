package guardrails

import "fmt"

// ResolveConfig validates and normalizes policy-based guardrails configs.
func ResolveConfig(cfg Config) (Config, error) {
	cfg = normalizePolicyConfig(cfg)
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
	return normalizePolicyConfig(cfg)
}

func usesPolicyConfig(cfg Config) bool {
	return len(cfg.Policies) > 0 || len(cfg.Groups) > 0
}

func normalizePolicyConfig(cfg Config) Config {
	next := cfg
	if usesPolicyConfig(cfg) {
		hasDefaultGroup := false
		for _, group := range cfg.Groups {
			if group.ID == DefaultPolicyGroupID {
				hasDefaultGroup = true
				break
			}
		}
		if !hasDefaultGroup {
			next.Groups = append(append([]PolicyGroup(nil), cfg.Groups...), PolicyGroup{
				ID:      DefaultPolicyGroupID,
				Name:    "Default",
				Enabled: boolPtr(true),
			})
		}
	}
	if len(cfg.Policies) == 0 {
		return next
	}

	next.Policies = make([]Policy, len(cfg.Policies))
	for i, policy := range cfg.Policies {
		policy.Groups = normalizedPolicyGroups(policy)
		next.Policies[i] = policy
	}
	return next
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

	policyGroups, err := resolvePolicyGroups(policy, groups)
	if err != nil {
		return nil, err
	}

	switch policy.Kind {
	case PolicyKindResourceAccess, PolicyKindOperationLegacy:
		return buildResourceAccessPolicyEvaluator(policy, policyGroups)
	case PolicyKindCommandExecution:
		return buildCommandExecutionPolicyEvaluator(policy, policyGroups)
	case PolicyKindContent:
		return buildContentPolicyEvaluator(policy, policyGroups)
	default:
		return nil, fmt.Errorf("policy %s: unsupported kind %q", policy.ID, policy.Kind)
	}
}

func normalizedPolicyGroups(policy Policy) []string {
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

func resolvePolicyGroups(policy Policy, groups map[string]PolicyGroup) ([]PolicyGroup, error) {
	ids := normalizedPolicyGroups(policy)
	if len(ids) == 0 {
		return nil, nil
	}
	resolved := make([]PolicyGroup, 0, len(ids))
	for _, groupID := range ids {
		group, ok := groups[groupID]
		if !ok {
			return nil, fmt.Errorf("policy %s: unknown group %q", policy.ID, groupID)
		}
		resolved = append(resolved, group)
	}
	return resolved, nil
}

func buildResourceAccessPolicyEvaluator(policy Policy, groups []PolicyGroup) (Evaluator, error) {
	scope := mergePolicyScope(policy.Scope)
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
	params.Verdict = resolvePolicyVerdict(policy, VerdictBlock)
	params.Reason = policy.Reason

	return NewOperationPolicy(
		policy.ID,
		policyName(policy),
		resolvePolicyEnabled(policy, groups),
		scope,
		params,
	)
}

func buildCommandExecutionPolicyEvaluator(policy Policy, groups []PolicyGroup) (Evaluator, error) {
	scope := mergePolicyScope(policy.Scope)
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
	params.Verdict = resolvePolicyVerdict(policy, VerdictBlock)
	params.Reason = policy.Reason

	return NewOperationPolicy(
		policy.ID,
		policyName(policy),
		resolvePolicyEnabled(policy, groups),
		scope,
		params,
	)
}

func buildContentPolicyEvaluator(policy Policy, groups []PolicyGroup) (Evaluator, error) {
	if len(policy.Match.Patterns) == 0 && len(policy.Match.CredentialRefs) == 0 {
		return nil, fmt.Errorf("policy %s: content policies require patterns or credential refs", policy.ID)
	}

	contentTypes := []ContentType{ContentTypeText}
	scope := mergePolicyScope(policy.Scope)
	scope.Content = contentTypes

	params := TextMatchConfig{
		Patterns:       append([]string(nil), policy.Match.Patterns...),
		CredentialRefs: append([]string(nil), policy.Match.CredentialRefs...),
		Targets:        contentTypes,
		Verdict:        resolvePolicyVerdict(policy, VerdictBlock),
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

func mergePolicyScope(policyScope Scope) Scope {
	return policyScope
}

func resolvePolicyEnabled(policy Policy, groups []PolicyGroup) bool {
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

func boolPtr(value bool) *bool {
	return &value
}

func resolvePolicyVerdict(policy Policy, fallback Verdict) Verdict {
	if policy.Verdict != "" {
		return policy.Verdict
	}
	return fallback
}

func policyName(policy Policy) string {
	if policy.Name != "" {
		return policy.Name
	}
	return policy.ID
}
