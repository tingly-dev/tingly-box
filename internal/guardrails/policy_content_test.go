package guardrails

import (
	"context"
	"testing"
)

func TestContentPolicyMatchesContains(t *testing.T) {
	policy, err := NewContentPolicy(
		"dangerous",
		"Dangerous Ops",
		true,
		Scope{},
		TextMatchConfig{
			Patterns:      []string{"rm -rf", "format c:"},
			Mode:          MatchAny,
			CaseSensitive: false,
			UseRegex:      false,
			Verdict:       VerdictBlock,
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), Input{
		Scenario:  "openai",
		Model:     "gpt-4.1-mini",
		Direction: DirectionRequest,
		Tags:      []string{"ops", "cli"},
		Content: Content{
			Text: "Please run RM -RF / now",
			Messages: []Message{
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
	if res.Verdict != VerdictBlock {
		t.Fatalf("expected block verdict, got %s", res.Verdict)
	}
}

func TestContentPolicyScopeMismatch(t *testing.T) {
	policy, err := NewContentPolicy(
		"dangerous",
		"Dangerous Ops",
		true,
		Scope{Scenarios: []string{"openai"}},
		TextMatchConfig{Patterns: []string{"rm -rf"}},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), Input{
		Scenario:  "anthropic",
		Direction: DirectionRequest,
		Content:   Content{Text: "rm -rf /"},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != VerdictAllow {
		t.Fatalf("expected allow verdict, got %s", res.Verdict)
	}
}

func TestContentPolicyTargetsCommand(t *testing.T) {
	policy, err := NewContentPolicy(
		"cmd-only",
		"Command Only",
		true,
		Scope{},
		TextMatchConfig{
			Patterns: []string{"rm -rf"},
			Targets:  []ContentType{ContentTypeCommand},
			Verdict:  VerdictBlock,
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), Input{
		Scenario:  "anthropic",
		Model:     "claude-3.7-sonnet",
		Direction: DirectionRequest,
		Tags:      []string{"tooling"},
		Content: Content{
			Text: "Use the tool to cleanup",
			Command: &Command{
				Name: "rm -rf",
				Arguments: map[string]interface{}{
					"path": "/",
				},
			},
			Messages: []Message{
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
	if res.Verdict != VerdictBlock {
		t.Fatalf("expected block verdict, got %s", res.Verdict)
	}
}

func TestContentPolicyTargetsCommandIgnoresDescriptionNoise(t *testing.T) {
	policy, err := NewContentPolicy(
		"cmd-shell-only",
		"Command Shell Only",
		true,
		Scope{},
		TextMatchConfig{
			Patterns: []string{"ssh directory"},
			Targets:  []ContentType{ContentTypeCommand},
			Verdict:  VerdictBlock,
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), Input{
		Scenario:  "anthropic",
		Model:     "claude-3.7-sonnet",
		Direction: DirectionResponse,
		Content: Content{
			Command: &Command{
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
	if res.Verdict != VerdictAllow {
		t.Fatalf("expected allow verdict, got %s", res.Verdict)
	}
}
