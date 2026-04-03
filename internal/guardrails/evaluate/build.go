package evaluate

import guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"

// BuildPolicyEngine creates a PolicyEngine from configuration and dependencies.
func BuildPolicyEngine(cfg guardrailscore.Config, deps Dependencies, opts ...Option) (*PolicyEngine, error) {
	resolvedCfg, err := ResolveConfig(cfg)
	if err != nil {
		return nil, err
	}

	evaluators, err := BuildEvaluators(resolvedCfg, deps)
	if err != nil {
		return nil, err
	}

	options := []Option{WithEvaluators(evaluators...)}
	if resolvedCfg.Strategy != "" {
		options = append(options, WithStrategy(resolvedCfg.Strategy))
	}
	if resolvedCfg.ErrorStrategy != "" {
		options = append(options, WithErrorStrategy(resolvedCfg.ErrorStrategy))
	}
	options = append(options, opts...)

	return NewPolicyEngine(options...), nil
}
