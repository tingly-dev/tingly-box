package guardrails

import "context"

// Engine evaluates inputs against a set of guardrail policies.
type Engine struct {
	evaluators    []Evaluator
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

// WithEvaluators sets the policy evaluators for the Engine.
func WithEvaluators(evaluators ...Evaluator) Option {
	return func(e *Engine) {
		e.evaluators = append(e.evaluators, evaluators...)
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

// Evaluators returns a copy of current policy evaluators.
func (e *Engine) Evaluators() []Evaluator {
	if len(e.evaluators) == 0 {
		return nil
	}
	cpy := make([]Evaluator, len(e.evaluators))
	copy(cpy, e.evaluators)
	return cpy
}

// Evaluate runs all policy evaluators and returns the aggregated result.
func (e *Engine) Evaluate(ctx context.Context, input Input) (Result, error) {
	result := Result{Verdict: VerdictAllow}

	for _, evaluator := range e.evaluators {
		if evaluator == nil {
			continue
		}

		policyResult, err := evaluator.Evaluate(ctx, input)
		if err != nil {
			result.Errors = append(result.Errors, PolicyError{
				PolicyID:   evaluator.ID(),
				PolicyName: evaluator.Name(),
				PolicyType: evaluator.Type(),
				Error:      err.Error(),
			})

			errorVerdict := verdictForErrorStrategy(e.errorStrategy)
			result.Verdict = mergeVerdict(result.Verdict, errorVerdict, e.strategy)

			if e.shortCircuit && result.Verdict == VerdictBlock {
				break
			}
			continue
		}

		if policyResult.Verdict == "" {
			policyResult.Verdict = VerdictAllow
		}

		if policyResult.Verdict != VerdictAllow {
			result.Reasons = append(result.Reasons, policyResult)
			result.Verdict = mergeVerdict(result.Verdict, policyResult.Verdict, e.strategy)

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
