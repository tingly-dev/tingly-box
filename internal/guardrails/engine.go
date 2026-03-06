package guardrails

import "context"

// Engine evaluates inputs against a set of guardrail rules.
type Engine struct {
	rules         []Rule
	strategy      CombineStrategy
	errorStrategy ErrorStrategy
	shortCircuit  bool
}

// Option configures an Engine.
type Option func(*Engine)

// NewEngine creates a new Engine with provided options.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{
		strategy:      StrategyMostSevere,
		errorStrategy: ErrorStrategyReview,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WithRules sets the rules for the Engine.
func WithRules(rules ...Rule) Option {
	return func(e *Engine) {
		e.rules = append(e.rules, rules...)
	}
}

// WithStrategy sets the combining strategy.
func WithStrategy(strategy CombineStrategy) Option {
	return func(e *Engine) {
		if strategy != "" {
			e.strategy = strategy
		}
	}
}

// WithErrorStrategy sets the error strategy.
func WithErrorStrategy(strategy ErrorStrategy) Option {
	return func(e *Engine) {
		if strategy != "" {
			e.errorStrategy = strategy
		}
	}
}

// WithShortCircuit enables early stop on block verdicts.
func WithShortCircuit(enable bool) Option {
	return func(e *Engine) {
		e.shortCircuit = enable
	}
}

// Rules returns a copy of current rules.
func (e *Engine) Rules() []Rule {
	if len(e.rules) == 0 {
		return nil
	}
	cpy := make([]Rule, len(e.rules))
	copy(cpy, e.rules)
	return cpy
}

// Evaluate runs all rules and returns the aggregated result.
func (e *Engine) Evaluate(ctx context.Context, input Input) (Result, error) {
	result := Result{Verdict: VerdictAllow}

	for _, rule := range e.rules {
		if rule == nil {
			continue
		}

		ruleResult, err := rule.Evaluate(ctx, input)
		if err != nil {
			result.Errors = append(result.Errors, RuleError{
				RuleID:   rule.ID(),
				RuleName: rule.Name(),
				RuleType: rule.Type(),
				Error:    err.Error(),
			})

			errorVerdict := verdictForErrorStrategy(e.errorStrategy)
			result.Verdict = mergeVerdict(result.Verdict, errorVerdict, e.strategy)

			if e.shortCircuit && result.Verdict == VerdictBlock {
				break
			}
			continue
		}

		if ruleResult.Verdict == "" {
			ruleResult.Verdict = VerdictAllow
		}

		if ruleResult.Verdict != VerdictAllow {
			result.Reasons = append(result.Reasons, ruleResult)
			result.Verdict = mergeVerdict(result.Verdict, ruleResult.Verdict, e.strategy)

			if e.shortCircuit && result.Verdict == VerdictBlock {
				break
			}
		}
	}

	return result, nil
}

func verdictForErrorStrategy(strategy ErrorStrategy) Verdict {
	switch strategy {
	case ErrorStrategyAllow:
		return VerdictAllow
	case ErrorStrategyBlock:
		return VerdictBlock
	default:
		return VerdictReview
	}
}

func mergeVerdict(current, candidate Verdict, strategy CombineStrategy) Verdict {
	if candidate == "" || candidate == VerdictAllow {
		return current
	}

	if strategy == StrategyBlockOnAny && candidate == VerdictBlock {
		return VerdictBlock
	}

	if severity(candidate) > severity(current) {
		return candidate
	}
	return current
}

func severity(v Verdict) int {
	switch v {
	case VerdictBlock:
		return 4
	case VerdictRedact:
		return 3
	case VerdictReview:
		return 2
	case VerdictAllow:
		return 1
	default:
		return 0
	}
}
