package evaluate

import (
	"context"
	"testing"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

type stubJudge struct {
	result JudgeResult
	err    error
	last   *guardrailscore.Input
}

func (s *stubJudge) Evaluate(_ context.Context, input guardrailscore.Input, _ ModelJudgeConfig) (JudgeResult, error) {
	s.last = &input
	return s.result, s.err
}

func TestJudgePolicyUsesJudge(t *testing.T) {
	judge := &stubJudge{
		result: JudgeResult{Verdict: guardrailscore.VerdictReview, Reason: "needs review"},
	}
	policy, err := NewJudgePolicy(
		"model-check",
		"Model Judge",
		true,
		guardrailscore.Scope{},
		ModelJudgeConfig{Model: "mini-judge"},
		judge,
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), guardrailscore.Input{
		Direction: guardrailscore.DirectionResponse,
		Content:   guardrailscore.Content{Text: "some output"},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != guardrailscore.VerdictReview {
		t.Fatalf("expected review verdict, got %s", res.Verdict)
	}
}

func TestJudgePolicyTargetsFilterContent(t *testing.T) {
	judge := &stubJudge{
		result: JudgeResult{Verdict: guardrailscore.VerdictAllow},
	}
	policy, err := NewJudgePolicy(
		"model-check",
		"Model Judge",
		true,
		guardrailscore.Scope{},
		ModelJudgeConfig{
			Model:   "mini-judge",
			Targets: []guardrailscore.ContentType{guardrailscore.ContentTypeText},
		},
		judge,
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	_, err = policy.Evaluate(context.Background(), guardrailscore.Input{
		Scenario:  "openai",
		Model:     "gpt-4.1-mini",
		Direction: guardrailscore.DirectionResponse,
		Tags:      []string{"safety"},
		Content: guardrailscore.Content{
			Text: "some output",
			Command: &guardrailscore.Command{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path": "/tmp/out.txt",
				},
			},
			Messages: []guardrailscore.Message{
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
