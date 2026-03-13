package guardrails

import (
	"context"
	"fmt"
	"strings"
)

// RuleTypeCommandPolicy evaluates normalized command semantics.
const RuleTypeCommandPolicy RuleType = "command_policy"

// ResourceMatchMode controls how resources are compared.
type ResourceMatchMode string

const (
	ResourceMatchExact    ResourceMatchMode = "exact"
	ResourceMatchPrefix   ResourceMatchMode = "prefix"
	ResourceMatchContains ResourceMatchMode = "contains"
)

// CommandPolicyConfig configures semantic command matching.
type CommandPolicyConfig struct {
	Kinds         []string          `json:"kinds,omitempty" yaml:"kinds,omitempty"`
	Actions       []string          `json:"actions,omitempty" yaml:"actions,omitempty"`
	Resources     []string          `json:"resources,omitempty" yaml:"resources,omitempty"`
	Terms         []string          `json:"terms,omitempty" yaml:"terms,omitempty"`
	ResourceMatch ResourceMatchMode `json:"resource_match,omitempty" yaml:"resource_match,omitempty"`
	Verdict       Verdict           `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	Reason        string            `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// CommandPolicyRule matches against normalized command semantics.
type CommandPolicyRule struct {
	id      string
	name    string
	enabled bool
	scope   Scope
	config  CommandPolicyConfig
}

func init() {
	RegisterRule(RuleTypeCommandPolicy, newCommandPolicyFactory)
}

func newCommandPolicyFactory(cfg RuleConfig, _ Dependencies) (Rule, error) {
	return NewCommandPolicyRuleFromConfig(cfg)
}

// NewCommandPolicyRuleFromConfig creates a semantic command rule from config.
func NewCommandPolicyRuleFromConfig(cfg RuleConfig) (*CommandPolicyRule, error) {
	params := CommandPolicyConfig{}
	if err := DecodeParams(cfg.Params, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if len(params.Kinds) == 0 && len(params.Actions) == 0 && len(params.Resources) == 0 && len(params.Terms) == 0 {
		return nil, fmt.Errorf("at least one of kinds, actions, resources, or terms is required")
	}
	if params.ResourceMatch == "" {
		params.ResourceMatch = ResourceMatchPrefix
	}
	if params.Verdict == "" {
		params.Verdict = VerdictBlock
	}

	return &CommandPolicyRule{
		id:      cfg.ID,
		name:    cfg.Name,
		enabled: cfg.Enabled,
		scope:   cfg.Scope,
		config:  params,
	}, nil
}

func (r *CommandPolicyRule) ID() string { return r.id }

func (r *CommandPolicyRule) Name() string { return r.name }

func (r *CommandPolicyRule) Type() RuleType { return RuleTypeCommandPolicy }

func (r *CommandPolicyRule) Enabled() bool { return r.enabled }

// Evaluate checks whether a normalized command violates semantic policy constraints.
func (r *CommandPolicyRule) Evaluate(_ context.Context, input Input) (RuleResult, error) {
	if !r.enabled {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if !r.scope.Matches(input) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if input.Content.Command == nil {
		return RuleResult{Verdict: VerdictAllow}, nil
	}

	cmd := input.Content.Command
	if cmd.Normalized == nil {
		cloned := *cmd
		cloned.AttachDerivedFields()
		cmd = &cloned
	}
	// command_policy only works on derived command semantics. If the tool call
	// cannot be normalized yet, we treat it as non-match instead of guessing.
	if cmd.Normalized == nil {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	norm := cmd.Normalized

	if len(r.config.Kinds) > 0 && !stringSliceIntersects(norm.Kind, r.config.Kinds) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if len(r.config.Actions) > 0 && !sliceIntersects(norm.Actions, r.config.Actions) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if len(r.config.Resources) > 0 && !resourcesMatch(norm.Resources, r.config.Resources, r.config.ResourceMatch) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if len(r.config.Terms) > 0 && !sliceContainsPattern(norm.Terms, r.config.Terms) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}

	reason := r.config.Reason
	if reason == "" {
		reason = "command policy violation"
	}

	return RuleResult{
		RuleID:   r.id,
		RuleName: r.name,
		RuleType: r.Type(),
		Verdict:  r.config.Verdict,
		Reason:   reason,
		Evidence: map[string]interface{}{
			"kind":      norm.Kind,
			"actions":   norm.Actions,
			"resources": norm.Resources,
			"terms":     norm.Terms,
		},
	}, nil
}

// stringSliceIntersects matches a single normalized field such as Kind.
func stringSliceIntersects(value string, patterns []string) bool {
	if value == "" {
		return false
	}
	for _, pattern := range patterns {
		if strings.EqualFold(value, pattern) {
			return true
		}
	}
	return false
}

// sliceIntersects checks case-insensitive exact overlap for semantic labels
// like Actions where partial substring matches would be too loose.
func sliceIntersects(values, patterns []string) bool {
	for _, value := range values {
		for _, pattern := range patterns {
			if strings.EqualFold(value, pattern) {
				return true
			}
		}
	}
	return false
}

// resourcesMatch supports path-oriented matching with configurable strictness.
func resourcesMatch(resources, patterns []string, mode ResourceMatchMode) bool {
	for _, resource := range resources {
		resourceLower := strings.ToLower(resource)
		for _, pattern := range patterns {
			patternLower := strings.ToLower(pattern)
			switch mode {
			case ResourceMatchExact:
				if resourceLower == patternLower {
					return true
				}
			case ResourceMatchContains:
				if strings.Contains(resourceLower, patternLower) {
					return true
				}
			default:
				if strings.HasPrefix(resourceLower, patternLower) || strings.Contains(resourceLower, patternLower) {
					return true
				}
			}
		}
	}
	return false
}

// sliceContainsPattern is a looser fallback used for normalized command terms.
func sliceContainsPattern(values, patterns []string) bool {
	for _, value := range values {
		valueLower := strings.ToLower(value)
		for _, pattern := range patterns {
			if strings.Contains(valueLower, strings.ToLower(pattern)) {
				return true
			}
		}
	}
	return false
}
