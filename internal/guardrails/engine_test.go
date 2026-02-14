package guardrails

import (
	"context"
	"errors"
	"testing"
)

type staticRule struct {
	id       string
	name     string
	ruleType RuleType
	result   RuleResult
	err      error
}

func (r staticRule) ID() string {
	return r.id
}

func (r staticRule) Name() string {
	return r.name
}

func (r staticRule) Type() RuleType {
	return r.ruleType
}

func (r staticRule) Enabled() bool {
	return true
}

func (r staticRule) Evaluate(_ context.Context, _ Input) (RuleResult, error) {
	return r.result, r.err
}

func TestEngineAggregatesVerdicts(t *testing.T) {
	engine := NewEngine(
		WithRules(
			staticRule{ruleType: "rule_a", result: RuleResult{Verdict: VerdictReview}},
			staticRule{ruleType: "rule_b", result: RuleResult{Verdict: VerdictBlock}},
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
		WithRules(staticRule{ruleType: "rule_error", err: errors.New("boom")}),
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
