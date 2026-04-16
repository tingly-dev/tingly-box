package evaluate

import (
	"context"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// PolicyEngine evaluates inputs against a set of guardrail policies.
type PolicyEngine struct {
	evaluators    []guardrailscore.Evaluator
	strategy      guardrailscore.CombineStrategy
	errorStrategy guardrailscore.ErrorStrategy
	shortCircuit  bool
}

// Option configures a PolicyEngine.
type Option func(*PolicyEngine)

// NewPolicyEngine creates a new PolicyEngine with provided options.
func NewPolicyEngine(opts ...Option) *PolicyEngine {
	e := &PolicyEngine{
		strategy:      guardrailscore.StrategyMostSevere,
		errorStrategy: guardrailscore.ErrorStrategyReview,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WithEvaluators sets the policy evaluators for the PolicyEngine.
func WithEvaluators(evaluators ...guardrailscore.Evaluator) Option {
	return func(e *PolicyEngine) {
		e.evaluators = append(e.evaluators, evaluators...)
	}
}

// WithStrategy sets the combining strategy.
func WithStrategy(strategy guardrailscore.CombineStrategy) Option {
	return func(e *PolicyEngine) {
		if strategy != "" {
			e.strategy = strategy
		}
	}
}

// WithErrorStrategy sets the error strategy.
func WithErrorStrategy(strategy guardrailscore.ErrorStrategy) Option {
	return func(e *PolicyEngine) {
		if strategy != "" {
			e.errorStrategy = strategy
		}
	}
}

// WithShortCircuit enables early stop on block verdicts.
func WithShortCircuit(enable bool) Option {
	return func(e *PolicyEngine) {
		e.shortCircuit = enable
	}
}

// Evaluators returns a copy of current policy evaluators.
func (e *PolicyEngine) Evaluators() []guardrailscore.Evaluator {
	if len(e.evaluators) == 0 {
		return nil
	}
	cpy := make([]guardrailscore.Evaluator, len(e.evaluators))
	copy(cpy, e.evaluators)
	return cpy
}

// Evaluate runs all policy evaluators and returns the aggregated result.
func (e *PolicyEngine) Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
	result := guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}

	for _, evaluator := range e.evaluators {
		if evaluator == nil {
			continue
		}

		policyResult, err := evaluator.Evaluate(ctx, input)
		if err != nil {
			result.Errors = append(result.Errors, guardrailscore.PolicyError{
				PolicyID:   evaluator.ID(),
				PolicyName: evaluator.Name(),
				PolicyType: evaluator.Type(),
				Error:      err.Error(),
			})

			errorVerdict := verdictForErrorStrategy(e.errorStrategy)
			result.Verdict = mergeVerdict(result.Verdict, errorVerdict, e.strategy)

			if e.shortCircuit && result.Verdict == guardrailscore.VerdictBlock {
				break
			}
			continue
		}

		if policyResult.Verdict == "" {
			policyResult.Verdict = guardrailscore.VerdictAllow
		}

		if policyResult.Verdict != guardrailscore.VerdictAllow {
			result.Reasons = append(result.Reasons, policyResult)
			result.Verdict = mergeVerdict(result.Verdict, policyResult.Verdict, e.strategy)

			if e.shortCircuit && result.Verdict == guardrailscore.VerdictBlock {
				break
			}
		}
	}

	return result, nil
}

func verdictForErrorStrategy(strategy guardrailscore.ErrorStrategy) guardrailscore.Verdict {
	switch strategy {
	case guardrailscore.ErrorStrategyAllow:
		return guardrailscore.VerdictAllow
	case guardrailscore.ErrorStrategyBlock:
		return guardrailscore.VerdictBlock
	default:
		return guardrailscore.VerdictReview
	}
}

func mergeVerdict(current, candidate guardrailscore.Verdict, strategy guardrailscore.CombineStrategy) guardrailscore.Verdict {
	if candidate == "" || candidate == guardrailscore.VerdictAllow {
		return current
	}

	if strategy == guardrailscore.StrategyBlockOnAny && candidate == guardrailscore.VerdictBlock {
		return guardrailscore.VerdictBlock
	}

	if severity(candidate) > severity(current) {
		return candidate
	}
	return current
}

func severity(v guardrailscore.Verdict) int {
	switch v {
	case guardrailscore.VerdictBlock:
		return 4
	case guardrailscore.VerdictRedact:
		return 3
	case guardrailscore.VerdictReview:
		return 2
	case guardrailscore.VerdictAllow:
		return 1
	default:
		return 0
	}
}
