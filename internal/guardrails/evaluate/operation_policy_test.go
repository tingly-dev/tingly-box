package evaluate

import (
	"context"
	"testing"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

func TestOperationPolicyBlocksSSHRead(t *testing.T) {
	policy, err := NewOperationPolicy(
		"block-ssh-read",
		"Block SSH read",
		true,
		guardrailscore.Scope{
			Directions: []guardrailscore.Direction{guardrailscore.DirectionResponse},
			Content:    []guardrailscore.ContentType{guardrailscore.ContentTypeCommand},
		},
		CommandPolicyConfig{
			Actions:       []string{"read"},
			Resources:     []string{"~/.ssh", "/.ssh"},
			ResourceMatch: ResourceMatchPrefix,
			Verdict:       guardrailscore.VerdictBlock,
			Reason:        "ssh directory access blocked",
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), guardrailscore.Input{
		Direction: guardrailscore.DirectionResponse,
		Content: guardrailscore.Content{
			Command: &guardrailscore.Command{
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
	if res.Verdict != guardrailscore.VerdictBlock {
		t.Fatalf("expected block, got %s", res.Verdict)
	}
}

func TestOperationPolicyDoesNotBlockNonSSHRead(t *testing.T) {
	policy, err := NewOperationPolicy(
		"block-ssh-read",
		"Block SSH read",
		true,
		guardrailscore.Scope{},
		CommandPolicyConfig{
			Actions:   []string{"read"},
			Resources: []string{"~/.ssh", "/.ssh"},
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), guardrailscore.Input{
		Direction: guardrailscore.DirectionResponse,
		Content: guardrailscore.Content{
			Command: &guardrailscore.Command{
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
	if res.Verdict != guardrailscore.VerdictAllow {
		t.Fatalf("expected allow, got %s", res.Verdict)
	}
}
