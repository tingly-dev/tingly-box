package guardrails

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// PolicyTypeContent identifies content policies backed by pattern matching.
const PolicyTypeContent PolicyType = "text_match"

// MatchMode determines how patterns are combined.
type MatchMode string

const (
	MatchAny MatchMode = "any"
	MatchAll MatchMode = "all"
)

// TextMatchConfig configures text matching rules.
type TextMatchConfig struct {
	Patterns       []string      `json:"patterns" yaml:"patterns"`
	CredentialRefs []string      `json:"credential_refs,omitempty" yaml:"credential_refs,omitempty"`
	Targets        []ContentType `json:"targets,omitempty" yaml:"targets,omitempty"`
	Mode           MatchMode     `json:"mode,omitempty" yaml:"mode,omitempty"`
	CaseSensitive  bool          `json:"case_sensitive,omitempty" yaml:"case_sensitive,omitempty"`
	UseRegex       bool          `json:"use_regex,omitempty" yaml:"use_regex,omitempty"`
	MinMatches     int           `json:"min_matches,omitempty" yaml:"min_matches,omitempty"`
	Verdict        Verdict       `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	Reason         string        `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// ContentPolicy evaluates content policies using literal or regex pattern matching.
type ContentPolicy struct {
	id       string
	name     string
	enabled  bool
	scope    Scope
	config   TextMatchConfig
	patterns []string
	regex    []*regexp.Regexp
}

// NewContentPolicy creates a content policy from typed policy data.
func NewContentPolicy(id, name string, enabled bool, scope Scope, params TextMatchConfig) (*ContentPolicy, error) {
	if len(params.Patterns) == 0 && len(params.CredentialRefs) == 0 {
		return nil, fmt.Errorf("patterns or credential refs cannot be empty")
	}

	if params.Mode == "" {
		params.Mode = MatchAny
	}
	if params.Verdict == "" {
		params.Verdict = VerdictBlock
	}

	policy := &ContentPolicy{
		id:      id,
		name:    name,
		enabled: enabled,
		scope:   scope,
		config:  params,
	}

	if params.UseRegex {
		policy.regex = make([]*regexp.Regexp, 0, len(params.Patterns))
		for _, pattern := range params.Patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
			}
			policy.regex = append(policy.regex, re)
		}
	} else {
		policy.patterns = make([]string, 0, len(params.Patterns))
		for _, pattern := range params.Patterns {
			policy.patterns = append(policy.patterns, pattern)
		}
	}

	return policy, nil
}

// ID returns the policy ID.
func (r *ContentPolicy) ID() string {
	return r.id
}

// Name returns the policy name.
func (r *ContentPolicy) Name() string {
	return r.name
}

// Type returns the policy type.
func (r *ContentPolicy) Type() PolicyType {
	return PolicyTypeContent
}

// Enabled returns whether the policy is enabled.
func (r *ContentPolicy) Enabled() bool {
	return r.enabled
}

// Evaluate matches text against configured patterns.
func (r *ContentPolicy) Evaluate(_ context.Context, input Input) (PolicyResult, error) {
	if !r.enabled {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}
	if !r.scope.Matches(input) {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}

	if len(r.config.Targets) > 0 && !input.Content.HasAny(r.config.Targets) {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}

	// Credential-backed mask policies do their actual replacement in the request/runtime
	// pipeline. The evaluator only needs to validate config and identify the policy.
	if len(r.config.Patterns) == 0 && len(r.config.CredentialRefs) > 0 {
		return PolicyResult{
			PolicyID:   r.id,
			PolicyName: r.name,
			PolicyType: r.Type(),
			Verdict:    r.config.Verdict,
			Reason:     r.config.Reason,
			Evidence: map[string]interface{}{
				"credential_refs": append([]string(nil), r.config.CredentialRefs...),
			},
		}, nil
	}

	text := input.Content.CombinedTextFor(r.config.Targets)
	if text == "" {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}

	matched := make([]string, 0)
	matches := 0

	if r.config.UseRegex {
		for i, re := range r.regex {
			if re.MatchString(text) {
				matches++
				matched = append(matched, r.config.Patterns[i])
			}
		}
	} else {
		searchText := text
		if !r.config.CaseSensitive {
			searchText = strings.ToLower(searchText)
		}
		for _, pattern := range r.patterns {
			check := pattern
			if !r.config.CaseSensitive {
				check = strings.ToLower(check)
			}
			if strings.Contains(searchText, check) {
				matches++
				matched = append(matched, pattern)
			}
		}
	}

	if !r.isTriggered(matches, len(r.config.Patterns)) {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}

	reason := r.config.Reason
	if reason == "" {
		reason = "matched prohibited content"
	}

	return PolicyResult{
		PolicyID:   r.id,
		PolicyName: r.name,
		PolicyType: r.Type(),
		Verdict:    r.config.Verdict,
		Reason:     reason,
		Evidence: map[string]interface{}{
			"matches":          matches,
			"matched_patterns": matched,
		},
	}, nil
}

func (r *ContentPolicy) isTriggered(matches, total int) bool {
	if r.config.MinMatches > 0 {
		return matches >= r.config.MinMatches
	}
	if r.config.Mode == MatchAll {
		return total > 0 && matches == total
	}
	return matches > 0
}
