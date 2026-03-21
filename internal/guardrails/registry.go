package guardrails

// Dependencies provides external services needed by some policy kinds.
type Dependencies struct {
	Judge Judge
}

// BuildEvaluators instantiates runtime evaluators from policy configuration.
func BuildEvaluators(cfg Config, deps Dependencies) ([]Evaluator, error) {
	resolvedCfg, err := ResolveConfig(cfg)
	if err != nil {
		return nil, err
	}
	// The runtime is policy-based now. Dependencies are still threaded through
	// this entrypoint so future policy kinds can opt into external services
	// without changing the build signature again.
	_ = deps
	groupByID := make(map[string]PolicyGroup, len(resolvedCfg.Groups))
	for _, group := range resolvedCfg.Groups {
		groupByID[group.ID] = group
	}
	evaluators := make([]Evaluator, 0, len(resolvedCfg.Policies))
	for _, policy := range resolvedCfg.Policies {
		evaluator, err := buildPolicyEvaluator(policy, groupByID)
		if err != nil {
			return nil, err
		}
		evaluators = append(evaluators, evaluator)
	}
	return evaluators, nil
}

// BuildEngine creates an Engine from configuration and dependencies.
func BuildEngine(cfg Config, deps Dependencies, opts ...Option) (*Engine, error) {
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

	return NewEngine(options...), nil
}
