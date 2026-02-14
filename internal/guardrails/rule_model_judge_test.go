package guardrails

import (
	"context"
	"testing"
)

type stubJudge struct {
	result JudgeResult
	err    error
	last   *Input
}

func (s *stubJudge) Evaluate(_ context.Context, input Input, _ ModelJudgeConfig) (JudgeResult, error) {
	s.last = &input
	return s.result, s.err
}

func TestModelJudgeRuleUsesJudge(t *testing.T) {
	cfg := RuleConfig{
		ID:      "model-check",
		Name:    "Model Judge",
		Type:    RuleTypeModelJudge,
		Enabled: true,
		Params: map[string]interface{}{
			"model": "mini-judge",
		},
	}

	judge := &stubJudge{
		result: JudgeResult{Verdict: VerdictReview, Reason: "needs review"},
	}
	rule, err := NewModelJudgeRuleFromConfig(cfg, judge)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}

	res, err := rule.Evaluate(context.Background(), Input{
		Direction: DirectionResponse,
		Content:   Content{Text: "some output"},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != VerdictReview {
		t.Fatalf("expected review verdict, got %s", res.Verdict)
	}
}

func TestModelJudgeRuleTargetsFilterContent(t *testing.T) {
	cfg := RuleConfig{
		ID:      "model-check",
		Name:    "Model Judge",
		Type:    RuleTypeModelJudge,
		Enabled: true,
		Params: map[string]interface{}{
			"model":   "mini-judge",
			"targets": []string{"text"},
		},
	}

	judge := &stubJudge{
		result: JudgeResult{Verdict: VerdictAllow},
	}
	rule, err := NewModelJudgeRuleFromConfig(cfg, judge)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}

	_, err = rule.Evaluate(context.Background(), Input{
		Scenario:  "openai",
		Model:     "gpt-4.1-mini",
		Direction: DirectionResponse,
		Tags:      []string{"safety"},
		Content: Content{
			Text: "some output",
			Command: &Command{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path": "/tmp/out.txt",
				},
			},
			Messages: []Message{
				{Role: "user", Content: "summarize the report"},
				{Role: "assistant", Content: "working on it"},
			},
		},
		Metadata: map[string]interface{}{
			"request_id": "req_789",
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if judge.last == nil || judge.last.Content.Command != nil {
		t.Fatalf("expected command to be filtered before judge")
	}
	if judge.last.Content.Text == "" {
		t.Fatalf("expected text to be preserved before judge")
	}
}
