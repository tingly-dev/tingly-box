package evaluate

import (
	"context"
	"errors"
	"testing"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

type staticEvaluator struct {
	policyType guardrailscore.PolicyType
	result     guardrailscore.PolicyResult
	err        error
}

func (r staticEvaluator) ID() string {
	return string(r.policyType)
}

func (r staticEvaluator) Name() string {
	return string(r.policyType)
}

func (r staticEvaluator) Type() guardrailscore.PolicyType {
	return r.policyType
}

func (r staticEvaluator) Enabled() bool {
	return true
}

func (r staticEvaluator) Evaluate(_ context.Context, _ guardrailscore.Input) (guardrailscore.PolicyResult, error) {
	return r.result, r.err
}

func TestPolicyEngineCombinesMostSevere(t *testing.T) {
	engine := NewPolicyEngine(
		WithEvaluators(
			staticEvaluator{policyType: "policy_a", result: guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictReview}},
			staticEvaluator{policyType: "policy_b", result: guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictBlock}},
		),
	)

	res, err := engine.Evaluate(context.Background(), guardrailscore.Input{})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if res.Verdict != guardrailscore.VerdictBlock {
		t.Fatalf("Evaluate() verdict = %s, want %s", res.Verdict, guardrailscore.VerdictBlock)
	}
	if len(res.Reasons) != 2 {
		t.Fatalf("Evaluate() reasons = %d, want 2", len(res.Reasons))
	}
}

func TestPolicyEngineErrorStrategyReview(t *testing.T) {
	engine := NewPolicyEngine(
		WithEvaluators(staticEvaluator{policyType: "policy_error", err: errors.New("boom")}),
		WithErrorStrategy(guardrailscore.ErrorStrategyReview),
	)

	res, err := engine.Evaluate(context.Background(), guardrailscore.Input{})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if res.Verdict != guardrailscore.VerdictReview {
		t.Fatalf("Evaluate() verdict = %s, want %s", res.Verdict, guardrailscore.VerdictReview)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("Evaluate() errors = %d, want 1", len(res.Errors))
	}
}
