package guardrails

import (
	"context"
	"errors"
	"testing"
)

type staticEvaluator struct {
	id         string
	name       string
	policyType PolicyType
	result     PolicyResult
	err        error
}

func (r staticEvaluator) ID() string {
	return r.id
}

func (r staticEvaluator) Name() string {
	return r.name
}

func (r staticEvaluator) Type() PolicyType {
	return r.policyType
}

func (r staticEvaluator) Enabled() bool {
	return true
}

func (r staticEvaluator) Evaluate(_ context.Context, _ Input) (PolicyResult, error) {
	return r.result, r.err
}

func TestEngineAggregatesVerdicts(t *testing.T) {
	engine := NewEngine(
		WithEvaluators(
			staticEvaluator{policyType: "policy_a", result: PolicyResult{Verdict: VerdictReview}},
			staticEvaluator{policyType: "policy_b", result: PolicyResult{Verdict: VerdictBlock}},
		),
	)

	res, err := engine.Evaluate(context.Background(), Input{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != VerdictBlock {
		t.Fatalf("expected block verdict, got %s", res.Verdict)
	}
	if len(res.Reasons) != 2 {
		t.Fatalf("expected 2 reasons, got %d", len(res.Reasons))
	}
}

func TestEngineErrorStrategyReview(t *testing.T) {
	engine := NewEngine(
		WithEvaluators(staticEvaluator{policyType: "policy_error", err: errors.New("boom")}),
		WithErrorStrategy(ErrorStrategyReview),
	)

	res, err := engine.Evaluate(context.Background(), Input{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != VerdictReview {
		t.Fatalf("expected review verdict, got %s", res.Verdict)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(res.Errors))
	}
}
