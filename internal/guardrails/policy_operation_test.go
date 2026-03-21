package guardrails

import (
	"context"
	"testing"
)

func TestOperationPolicyBlocksSSHRead(t *testing.T) {
	policy, err := NewOperationPolicy(
		"block-ssh-read",
		"Block SSH read",
		true,
		Scope{
			Directions: []Direction{DirectionResponse},
			Content:    []ContentType{ContentTypeCommand},
		},
		CommandPolicyConfig{
			Actions:       []string{"read"},
			Resources:     []string{"~/.ssh", "/.ssh"},
			ResourceMatch: ResourceMatchPrefix,
			Verdict:       VerdictBlock,
			Reason:        "ssh directory access blocked",
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), Input{
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

func TestOperationPolicyDoesNotBlockNonSSHRead(t *testing.T) {
	policy, err := NewOperationPolicy(
		"block-ssh-read",
		"Block SSH read",
		true,
		Scope{},
		CommandPolicyConfig{
			Actions:   []string{"read"},
			Resources: []string{"~/.ssh", "/.ssh"},
		},
	)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	res, err := policy.Evaluate(context.Background(), Input{
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
