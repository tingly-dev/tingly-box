package evaluate

import (
	"context"
	"testing"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

func TestContentPolicyMatchesContains(t *testing.T) {
	policy, err := NewContentPolicy(
		"dangerous",
		"Dangerous Ops",
		true,
		guardrailscore.Scope{},
		TextMatchConfig{
			Patterns:      []string{"rm -rf", "format c:"},
			Mode:          MatchAny,
			CaseSensitive: false,
			UseRegex:      false,
			Verdict:       guardrailscore.VerdictBlock,
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), guardrailscore.Input{
		Scenario:  "openai",
		Model:     "gpt-4.1-mini",
		Direction: guardrailscore.DirectionRequest,
		Tags:      []string{"ops", "cli"},
		Content: guardrailscore.Content{
			Text: "Please run RM -RF / now",
			Messages: []guardrailscore.Message{
				{Role: "user", Content: "cleanup the disk"},
				{Role: "assistant", Content: "ok, running command"},
			},
		},
		Metadata: map[string]interface{}{
			"request_id": "req_123",
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != guardrailscore.VerdictBlock {
		t.Fatalf("expected block verdict, got %s", res.Verdict)
	}
}

func TestContentPolicyScopeMismatch(t *testing.T) {
	policy, err := NewContentPolicy(
		"dangerous",
		"Dangerous Ops",
		true,
		guardrailscore.Scope{Scenarios: []string{"openai"}},
		TextMatchConfig{Patterns: []string{"rm -rf"}},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), guardrailscore.Input{
		Scenario:  "anthropic",
		Direction: guardrailscore.DirectionRequest,
		Content:   guardrailscore.Content{Text: "rm -rf /"},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != guardrailscore.VerdictAllow {
		t.Fatalf("expected allow verdict, got %s", res.Verdict)
	}
}

func TestContentPolicyTargetsCommand(t *testing.T) {
	policy, err := NewContentPolicy(
		"cmd-only",
		"Command Only",
		true,
		guardrailscore.Scope{},
		TextMatchConfig{
			Patterns: []string{"rm -rf"},
			Targets:  []guardrailscore.ContentType{guardrailscore.ContentTypeCommand},
			Verdict:  guardrailscore.VerdictBlock,
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), guardrailscore.Input{
		Scenario:  "anthropic",
		Model:     "claude-3.7-sonnet",
		Direction: guardrailscore.DirectionRequest,
		Tags:      []string{"tooling"},
		Content: guardrailscore.Content{
			Text: "Use the tool to cleanup",
			Command: &guardrailscore.Command{
				Name: "rm -rf",
				Arguments: map[string]interface{}{
					"path": "/",
				},
			},
			Messages: []guardrailscore.Message{
				{Role: "user", Content: "clean everything"},
				{Role: "assistant", Content: "calling tool"},
			},
		},
		Metadata: map[string]interface{}{
			"request_id": "req_456",
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != guardrailscore.VerdictBlock {
		t.Fatalf("expected block verdict, got %s", res.Verdict)
	}
}

func TestContentPolicyTargetsCommandIgnoresDescriptionNoise(t *testing.T) {
	policy, err := NewContentPolicy(
		"cmd-shell-only",
		"Command Shell Only",
		true,
		guardrailscore.Scope{},
		TextMatchConfig{
			Patterns: []string{"ssh directory"},
			Targets:  []guardrailscore.ContentType{guardrailscore.ContentTypeCommand},
			Verdict:  guardrailscore.VerdictBlock,
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), guardrailscore.Input{
		Scenario:  "anthropic",
		Model:     "claude-3.7-sonnet",
		Direction: guardrailscore.DirectionResponse,
		Content: guardrailscore.Content{
			Command: &guardrailscore.Command{
				Name: "bash",
				Arguments: map[string]interface{}{
					"command":     "ls -la ~/.ssh",
					"description": "Inspect the ssh directory contents",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != guardrailscore.VerdictAllow {
		t.Fatalf("expected allow verdict, got %s", res.Verdict)
	}
}
