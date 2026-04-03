package evaluate

import (
	"fmt"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// ResolveConfig validates and normalizes policy-based guardrails configs.
func ResolveConfig(cfg guardrailscore.Config) (guardrailscore.Config, error) {
	cfg = normalizePolicyConfig(cfg)
	if !usesPolicyConfig(cfg) {
		return guardrailscore.Config{}, fmt.Errorf("guardrails config must define groups or policies")
	}
	if err := validatePolicyConfig(cfg); err != nil {
		return guardrailscore.Config{}, err
	}
	return cfg, nil
}

// IsPolicyConfig reports whether a config uses the policy/group schema.
func IsPolicyConfig(cfg guardrailscore.Config) bool {
	return usesPolicyConfig(cfg)
}

// StorageConfig normalizes configs before persisting them back to disk.
func StorageConfig(cfg guardrailscore.Config) guardrailscore.Config {
	return normalizePolicyConfig(cfg)
}

func usesPolicyConfig(cfg guardrailscore.Config) bool {
	return len(cfg.Policies) > 0 || len(cfg.Groups) > 0
}

func normalizePolicyConfig(cfg guardrailscore.Config) guardrailscore.Config {
	next := cfg
	if usesPolicyConfig(cfg) {
		hasDefaultGroup := false
		for _, group := range cfg.Groups {
			if group.ID == guardrailscore.DefaultPolicyGroupID {
				hasDefaultGroup = true
				break
			}
		}
		if !hasDefaultGroup {
			next.Groups = append(append([]guardrailscore.PolicyGroup(nil), cfg.Groups...), guardrailscore.PolicyGroup{
				ID:      guardrailscore.DefaultPolicyGroupID,
				Name:    "Default",
				Enabled: boolPtr(true),
			})
		}
	}
	if len(cfg.Policies) == 0 {
		return next
	}

	next.Policies = make([]guardrailscore.Policy, len(cfg.Policies))
	for i, policy := range cfg.Policies {
		policy.Groups = normalizedPolicyGroups(policy)
		next.Policies[i] = policy
	}
	return next
}

func validatePolicyConfig(cfg guardrailscore.Config) error {
	groupByID := make(map[string]guardrailscore.PolicyGroup, len(cfg.Groups))
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
		if _, err := buildPolicyEvaluator(policy, groupByID, Dependencies{}); err != nil {
			return err
		}
	}

	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

