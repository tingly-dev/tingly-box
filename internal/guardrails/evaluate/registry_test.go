package evaluate

import (
	"testing"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

func TestBuildEvaluatorsCreatesResourceAccessPolicyEvaluator(t *testing.T) {
	enabled := true
	evaluators, err := BuildEvaluators(guardrailscore.Config{
		Groups: []guardrailscore.PolicyGroup{
			{
				ID:      "high-risk",
				Enabled: &enabled,
			},
		},
		Policies: []guardrailscore.Policy{
			{
				ID:      "block-ssh-read",
				Kind:    guardrailscore.PolicyKindResourceAccess,
				Groups:  []string{"high-risk"},
				Enabled: &enabled,
				Scope: guardrailscore.Scope{
					Scenarios:  []string{"claude_code"},
					Directions: []guardrailscore.Direction{guardrailscore.DirectionResponse},
				},
				Match: guardrailscore.PolicyMatch{
					ToolNames: []string{"bash"},
					Actions: &guardrailscore.ActionSelector{
						Include: []string{"read"},
					},
					Resources: &guardrailscore.ResourceMatcher{
						Type:   "path",
						Mode:   "prefix",
						Values: []string{"~/.ssh"},
					},
				},
				Reason: "Reading SSH directory content is blocked.",
			},
		},
	}, Dependencies{})
	if err != nil {
		t.Fatalf("BuildEvaluators() error = %v", err)
	}
	if len(evaluators) != 1 {
		t.Fatalf("BuildEvaluators() built %d evaluators, want 1", len(evaluators))
	}
	if evaluators[0].Type() != PolicyTypeOperation {
		t.Fatalf("evaluators[0].Type() = %s, want %s", evaluators[0].Type(), PolicyTypeOperation)
	}
	policyRule, ok := evaluators[0].(*OperationPolicy)
	if !ok {
		t.Fatalf("evaluators[0] type = %T, want *OperationPolicy", evaluators[0])
	}
	if got := policyRule.scope.Scenarios; len(got) != 1 || got[0] != "claude_code" {
		t.Fatalf("policyRule.scope.Scenarios = %v", got)
	}
	if got := policyRule.scope.Content; len(got) != 1 || got[0] != guardrailscore.ContentTypeCommand {
		t.Fatalf("policyRule.scope.Content = %v", got)
	}
	if got := policyRule.config.ToolNames; len(got) != 1 || got[0] != "bash" {
		t.Fatalf("policyRule.config.ToolNames = %#v", got)
	}
}

func TestBuildEvaluatorsCreatesContentPolicyEvaluator(t *testing.T) {
	evaluators, err := BuildEvaluators(guardrailscore.Config{
		Policies: []guardrailscore.Policy{
			{
				ID:   "block-secret-output",
				Kind: guardrailscore.PolicyKindContent,
				Match: guardrailscore.PolicyMatch{
					Patterns:    []string{"BEGIN OPENSSH PRIVATE KEY"},
					PatternMode: "regex",
				},
			},
		},
	}, Dependencies{})
	if err != nil {
		t.Fatalf("BuildEvaluators() error = %v", err)
	}
	if len(evaluators) != 1 {
		t.Fatalf("BuildEvaluators() built %d evaluators, want 1", len(evaluators))
	}
	if evaluators[0].Type() != PolicyTypeContent {
		t.Fatalf("evaluators[0].Type() = %s, want %s", evaluators[0].Type(), PolicyTypeContent)
	}
	policyRule, ok := evaluators[0].(*ContentPolicy)
	if !ok {
		t.Fatalf("evaluators[0] type = %T, want *ContentPolicy", evaluators[0])
	}
	if got := policyRule.scope.Content; len(got) != 1 || got[0] != guardrailscore.ContentTypeText {
		t.Fatalf("policyRule.scope.Content = %v", got)
	}
	if !policyRule.config.UseRegex {
		t.Fatalf("policyRule.config.UseRegex = false, want true")
	}
}
