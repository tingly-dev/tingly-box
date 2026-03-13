package guardrails

import (
	"context"
	"testing"
)

func TestCommandPolicyRuleBlocksSSHRead(t *testing.T) {
	cfg := RuleConfig{
		ID:      "block-ssh-read",
		Name:    "Block SSH read",
		Type:    RuleTypeCommandPolicy,
		Enabled: true,
		Scope: Scope{
			Directions: []Direction{DirectionResponse},
			Content:    []ContentType{ContentTypeCommand},
		},
		Params: map[string]interface{}{
			"kinds":          []string{"shell"},
			"actions":        []string{"read"},
			"resources":      []string{"~/.ssh", "/.ssh"},
			"resource_match": "prefix",
			"verdict":        "block",
			"reason":         "ssh directory access blocked",
		},
	}

	rule, err := NewCommandPolicyRuleFromConfig(cfg)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}

	res, err := rule.Evaluate(context.Background(), Input{
		Direction: DirectionResponse,
		Content: Content{
			Command: &Command{
				Name: "bash",
				Arguments: map[string]interface{}{
					"command": "ls -la ~/.ssh",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != VerdictBlock {
		t.Fatalf("expected block, got %s", res.Verdict)
	}
}

func TestCommandPolicyRuleDoesNotBlockNonSSHRead(t *testing.T) {
	cfg := RuleConfig{
		ID:      "block-ssh-read",
		Name:    "Block SSH read",
		Type:    RuleTypeCommandPolicy,
		Enabled: true,
		Params: map[string]interface{}{
			"actions":   []string{"read"},
			"resources": []string{"~/.ssh", "/.ssh"},
		},
	}

	rule, err := NewCommandPolicyRuleFromConfig(cfg)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}

	res, err := rule.Evaluate(context.Background(), Input{
		Direction: DirectionResponse,
		Content: Content{
			Command: &Command{
				Name: "bash",
				Arguments: map[string]interface{}{
					"command": "ls -la ~/workspace",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Verdict != VerdictAllow {
		t.Fatalf("expected allow, got %s", res.Verdict)
	}
}
