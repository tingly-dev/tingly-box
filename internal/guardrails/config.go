package guardrails

import (
	"encoding/json"
	"fmt"
)

// Config is the top-level guardrails configuration.
type Config struct {
	Version       string          `json:"version,omitempty" yaml:"version,omitempty"`
	Strategy      CombineStrategy `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	ErrorStrategy ErrorStrategy   `json:"error_strategy,omitempty" yaml:"error_strategy,omitempty"`
	Rules         []RuleConfig    `json:"rules" yaml:"rules"`
}

// RuleConfig defines a single rule with flexible parameters.
type RuleConfig struct {
	ID      string                 `json:"id" yaml:"id"`
	Name    string                 `json:"name" yaml:"name"`
	Type    RuleType               `json:"type" yaml:"type"`
	Enabled bool                   `json:"enabled" yaml:"enabled"`
	Scope   Scope                  `json:"scope,omitempty" yaml:"scope,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty" yaml:"params,omitempty"`
}

// Scope limits when a rule is applied.
type Scope struct {
	Scenarios  []string      `json:"scenarios,omitempty" yaml:"scenarios,omitempty"`
	Models     []string      `json:"models,omitempty" yaml:"models,omitempty"`
	Directions []Direction   `json:"directions,omitempty" yaml:"directions,omitempty"`
	Tags       []string      `json:"tags,omitempty" yaml:"tags,omitempty"`
	Content    []ContentType `json:"content_types,omitempty" yaml:"content_types,omitempty"`
}

// Matches returns true when the input matches scope constraints.
func (s Scope) Matches(input Input) bool {
	if len(s.Scenarios) > 0 && !stringInSlice(input.Scenario, s.Scenarios) {
		return false
	}
	if len(s.Models) > 0 && !stringInSlice(input.Model, s.Models) {
		return false
	}
	if len(s.Directions) > 0 && !directionInSlice(input.Direction, s.Directions) {
		return false
	}
	if len(s.Tags) > 0 && !anyTagMatches(input.Tags, s.Tags) {
		return false
	}
	if len(s.Content) > 0 && !input.Content.HasAny(s.Content) {
		return false
	}
	return true
}

// Dependencies provides external services needed by some rule types.
type Dependencies struct {
	Judge Judge
}

// Factory creates a rule instance from config and dependencies.
type Factory func(cfg RuleConfig, deps Dependencies) (Rule, error)

var registry = map[RuleType]Factory{}

// RegisterRule registers a rule type factory.
func RegisterRule(ruleType RuleType, factory Factory) {
	registry[ruleType] = factory
}

// BuildRules instantiates rules from configuration.
func BuildRules(cfg Config, deps Dependencies) ([]Rule, error) {
	rules := make([]Rule, 0, len(cfg.Rules))
	for _, ruleCfg := range cfg.Rules {
		factory, ok := registry[ruleCfg.Type]
		if !ok {
			return nil, fmt.Errorf("unknown rule type: %s", ruleCfg.Type)
		}
		rule, err := factory(ruleCfg, deps)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", ruleCfg.ID, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// BuildEngine creates an Engine from configuration and dependencies.
func BuildEngine(cfg Config, deps Dependencies, opts ...Option) (*Engine, error) {
	rules, err := BuildRules(cfg, deps)
	if err != nil {
		return nil, err
	}

	options := []Option{WithRules(rules...)}
	if cfg.Strategy != "" {
		options = append(options, WithStrategy(cfg.Strategy))
	}
	if cfg.ErrorStrategy != "" {
		options = append(options, WithErrorStrategy(cfg.ErrorStrategy))
	}
	options = append(options, opts...)

	return NewEngine(options...), nil
}

// DecodeParams unmarshals params into a typed struct.
func DecodeParams(params map[string]interface{}, out interface{}) error {
	if len(params) == 0 {
		return nil
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func stringInSlice(value string, items []string) bool {
	for _, item := range items {
		if value == item {
			return true
		}
	}
	return false
}

func directionInSlice(value Direction, items []Direction) bool {
	for _, item := range items {
		if value == item {
			return true
		}
	}
	return false
}

func anyTagMatches(inputTags, scopeTags []string) bool {
	for _, tag := range inputTags {
		if stringInSlice(tag, scopeTags) {
			return true
		}
	}
	return false
}
