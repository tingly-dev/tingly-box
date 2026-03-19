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

func TestJudgePolicyUsesJudge(t *testing.T) {
	judge := &stubJudge{
		result: JudgeResult{Verdict: VerdictReview, Reason: "needs review"},
	}
	policy, err := NewJudgePolicy(
		"model-check",
		"Model Judge",
		true,
		Scope{},
		ModelJudgeConfig{Model: "mini-judge"},
		judge,
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), Input{
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

func TestJudgePolicyTargetsFilterContent(t *testing.T) {
	judge := &stubJudge{
		result: JudgeResult{Verdict: VerdictAllow},
	}
	policy, err := NewJudgePolicy(
		"model-check",
		"Model Judge",
		true,
		Scope{},
		ModelJudgeConfig{
			Model:   "mini-judge",
			Targets: []ContentType{ContentTypeText},
		},
		judge,
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	_, err = policy.Evaluate(context.Background(), Input{
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
