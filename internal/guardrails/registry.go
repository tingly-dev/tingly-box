package guardrails

import (
	"encoding/json"
	"fmt"
)

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
