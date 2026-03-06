package guardrails

import (
	"context"
	"testing"
)

func TestTextMatchRuleMatchesContains(t *testing.T) {
	cfg := RuleConfig{
		ID:      "dangerous",
		Name:    "Dangerous Ops",
		Type:    RuleTypeTextMatch,
		Enabled: true,
		Params: map[string]interface{}{
			"patterns":       []string{"rm -rf", "format c:"},
			"mode":           "any",
			"case_sensitive": false,
			"use_regex":      false,
			"verdict":        "block",
		},
	}

	rule, err := NewTextMatchRuleFromConfig(cfg)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}

	res, err := rule.Evaluate(context.Background(), Input{
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

func TestTextMatchRuleScopeMismatch(t *testing.T) {
	cfg := RuleConfig{
		ID:      "dangerous",
		Name:    "Dangerous Ops",
		Type:    RuleTypeTextMatch,
		Enabled: true,
		Scope: Scope{
			Scenarios: []string{"openai"},
		},
		Params: map[string]interface{}{
			"patterns": []string{"rm -rf"},
		},
	}

	rule, err := NewTextMatchRuleFromConfig(cfg)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}

	res, err := rule.Evaluate(context.Background(), Input{
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

func TestTextMatchRuleTargetsCommand(t *testing.T) {
	cfg := RuleConfig{
		ID:      "cmd-only",
		Name:    "Command Only",
		Type:    RuleTypeTextMatch,
		Enabled: true,
		Params: map[string]interface{}{
			"patterns": []string{"rm -rf"},
			"targets":  []string{"command"},
			"verdict":  "block",
		},
	}

	rule, err := NewTextMatchRuleFromConfig(cfg)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}

	res, err := rule.Evaluate(context.Background(), Input{
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
