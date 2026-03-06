package guardrails

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// RuleTypeTextMatch is a rule that matches text patterns.
const RuleTypeTextMatch RuleType = "text_match"

// MatchMode determines how patterns are combined.
type MatchMode string

const (
	MatchAny MatchMode = "any"
	MatchAll MatchMode = "all"
)

// TextMatchConfig configures text matching rules.
type TextMatchConfig struct {
	Patterns      []string      `json:"patterns" yaml:"patterns"`
	Targets       []ContentType `json:"targets,omitempty" yaml:"targets,omitempty"`
	Mode          MatchMode     `json:"mode,omitempty" yaml:"mode,omitempty"`
	CaseSensitive bool          `json:"case_sensitive,omitempty" yaml:"case_sensitive,omitempty"`
	UseRegex      bool          `json:"use_regex,omitempty" yaml:"use_regex,omitempty"`
	MinMatches    int           `json:"min_matches,omitempty" yaml:"min_matches,omitempty"`
	Verdict       Verdict       `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	Reason        string        `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// TextMatchRule implements a rule-based matcher.
type TextMatchRule struct {
	id       string
	name     string
	enabled  bool
	scope    Scope
	config   TextMatchConfig
	patterns []string
	regex    []*regexp.Regexp
}

func init() {
	RegisterRule(RuleTypeTextMatch, newTextMatchFactory)
}

func newTextMatchFactory(cfg RuleConfig, _ Dependencies) (Rule, error) {
	return NewTextMatchRuleFromConfig(cfg)
}

// NewTextMatchRuleFromConfig creates a text match rule from config.
func NewTextMatchRuleFromConfig(cfg RuleConfig) (*TextMatchRule, error) {
	params := TextMatchConfig{}
	if err := DecodeParams(cfg.Params, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}

	if len(params.Patterns) == 0 {
		return nil, fmt.Errorf("patterns cannot be empty")
	}

	if params.Mode == "" {
		params.Mode = MatchAny
	}
	if params.Verdict == "" {
		params.Verdict = VerdictBlock
	}

	rule := &TextMatchRule{
		id:      cfg.ID,
		name:    cfg.Name,
		enabled: cfg.Enabled,
		scope:   cfg.Scope,
		config:  params,
	}

	if params.UseRegex {
		rule.regex = make([]*regexp.Regexp, 0, len(params.Patterns))
		for _, pattern := range params.Patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
			}
			rule.regex = append(rule.regex, re)
		}
	} else {
		rule.patterns = make([]string, 0, len(params.Patterns))
		for _, pattern := range params.Patterns {
			rule.patterns = append(rule.patterns, pattern)
		}
	}

	return rule, nil
}

// ID returns the rule ID.
func (r *TextMatchRule) ID() string {
	return r.id
}

// Name returns the rule name.
func (r *TextMatchRule) Name() string {
	return r.name
}

// Type returns the rule type.
func (r *TextMatchRule) Type() RuleType {
	return RuleTypeTextMatch
}

// Enabled returns whether the rule is enabled.
func (r *TextMatchRule) Enabled() bool {
	return r.enabled
}

// Evaluate matches text against configured patterns.
func (r *TextMatchRule) Evaluate(_ context.Context, input Input) (RuleResult, error) {
	if !r.enabled {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if !r.scope.Matches(input) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}

	if len(r.config.Targets) > 0 && !input.Content.HasAny(r.config.Targets) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}

	text := input.Content.CombinedTextFor(r.config.Targets)
	if text == "" {
		return RuleResult{Verdict: VerdictAllow}, nil
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
		return RuleResult{Verdict: VerdictAllow}, nil
	}

	reason := r.config.Reason
	if reason == "" {
		reason = "matched prohibited content"
	}

	return RuleResult{
		RuleID:   r.id,
		RuleName: r.name,
		RuleType: r.Type(),
		Verdict:  r.config.Verdict,
		Reason:   reason,
		Evidence: map[string]interface{}{
			"matches":          matches,
			"matched_patterns": matched,
		},
	}, nil
}

func (r *TextMatchRule) isTriggered(matches, total int) bool {
	if r.config.MinMatches > 0 {
		return matches >= r.config.MinMatches
	}
	if r.config.Mode == MatchAll {
		return total > 0 && matches == total
	}
	return matches > 0
}
