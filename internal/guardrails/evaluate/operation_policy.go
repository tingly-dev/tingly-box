package evaluate

import (
	"context"
	"fmt"
	"strings"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// PolicyTypeOperation identifies operation policies backed by normalized command semantics.
const PolicyTypeOperation guardrailscore.PolicyType = "command_policy"

// ResourceMatchMode controls how resources are compared.
type ResourceMatchMode string

const (
	ResourceMatchExact    ResourceMatchMode = "exact"
	ResourceMatchPrefix   ResourceMatchMode = "prefix"
	ResourceMatchContains ResourceMatchMode = "contains"
)

// CommandPolicyConfig configures semantic command matching.
type CommandPolicyConfig struct {
	ToolNames     []string          `json:"tool_names,omitempty" yaml:"tool_names,omitempty"`
	Actions       []string          `json:"actions,omitempty" yaml:"actions,omitempty"`
	Resources     []string          `json:"resources,omitempty" yaml:"resources,omitempty"`
	Terms         []string          `json:"terms,omitempty" yaml:"terms,omitempty"`
	ResourceMatch ResourceMatchMode `json:"resource_match,omitempty" yaml:"resource_match,omitempty"`
	Verdict       guardrailscore.Verdict `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	Reason        string            `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// OperationPolicy evaluates operation policies against normalized command semantics.
type OperationPolicy struct {
	id      string
	name    string
	enabled bool
	scope   guardrailscore.Scope
	config  CommandPolicyConfig
}

// NewOperationPolicy creates an operation policy from typed policy data.
func NewOperationPolicy(id, name string, enabled bool, scope guardrailscore.Scope, params CommandPolicyConfig) (*OperationPolicy, error) {
	if len(params.ToolNames) == 0 && len(params.Actions) == 0 && len(params.Resources) == 0 && len(params.Terms) == 0 {
		return nil, fmt.Errorf("at least one of tool_names, actions, resources, or terms is required")
	}
	if params.ResourceMatch == "" {
		params.ResourceMatch = ResourceMatchPrefix
	}
	if params.Verdict == "" {
		params.Verdict = guardrailscore.VerdictBlock
	}

	return &OperationPolicy{
		id:      id,
		name:    name,
		enabled: enabled,
		scope:   scope,
		config:  params,
	}, nil
}

func (r *OperationPolicy) ID() string { return r.id }

func (r *OperationPolicy) Name() string { return r.name }

func (r *OperationPolicy) Type() guardrailscore.PolicyType { return PolicyTypeOperation }

func (r *OperationPolicy) Enabled() bool { return r.enabled }

// Evaluate checks whether a normalized command violates semantic policy constraints.
func (r *OperationPolicy) Evaluate(_ context.Context, input guardrailscore.Input) (guardrailscore.PolicyResult, error) {
	if !r.enabled {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	if !r.scope.Matches(input) {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	if input.Content.Command == nil {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}

	cmd := input.Content.Command
	if cmd.Normalized == nil {
		cloned := *cmd
		cloned.AttachDerivedFields()
		cmd = &cloned
	}
	if cmd.Normalized == nil {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	norm := cmd.Normalized

	if len(r.config.ToolNames) > 0 && !stringSliceIntersects(cmd.Name, r.config.ToolNames) {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	if len(r.config.Actions) > 0 && !sliceIntersects(norm.Actions, r.config.Actions) {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	if len(r.config.Resources) > 0 && !resourcesMatch(norm.Resources, r.config.Resources, r.config.ResourceMatch) {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	if len(r.config.Terms) > 0 && !sliceContainsPattern(norm.Terms, r.config.Terms) {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}

	reason := r.config.Reason
	if reason == "" {
		reason = "command policy violation"
	}

	return guardrailscore.PolicyResult{
		PolicyID:   r.id,
		PolicyName: r.name,
		PolicyType: r.Type(),
		Verdict:    r.config.Verdict,
		Reason:     reason,
		Evidence: map[string]interface{}{
			"tool_name": cmd.Name,
			"actions":   norm.Actions,
			"resources": norm.Resources,
			"terms":     norm.Terms,
		},
	}, nil
}

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
