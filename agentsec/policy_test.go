package agentsec

import (
	"testing"
)

// TestNewPolicy tests the Policy constructor.
func TestNewPolicy(t *testing.T) {
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
	}

	policy := NewPolicy(rules)

	// Check that rules were copied
	if len(policy.Rules()) != len(rules) {
		t.Errorf("Rules() length = %d, want %d", len(policy.Rules()), len(rules))
	}

	// Check default decision
	if policy.DefaultDecision() != DecisionRequireApproval {
		t.Errorf("DefaultDecision() = %v, want %v", policy.DefaultDecision(), DecisionRequireApproval)
	}
}

// TestNewPolicyWithDefault tests the Policy constructor with custom default.
func TestNewPolicyWithDefault(t *testing.T) {
	rules := []Rule{AnyToolRule{Tool: "Read"}}

	policy := NewPolicyWithDefault(rules, DecisionDeny)

	if policy.DefaultDecision() != DecisionDeny {
		t.Errorf("DefaultDecision() = %v, want %v", policy.DefaultDecision(), DecisionDeny)
	}
}

// TestPolicy_Evaluate_Allow tests cases where rules match.
func TestPolicy_Evaluate_Allow(t *testing.T) {
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "pwd"},
		PrefixRule{Tool: "Bash", Prefix: "git"},
		AnyToolRule{Tool: "Read"},
	}
	policy := NewPolicy(rules)

	tests := []struct {
		name        string
		tool, input string
	}{
		{"exact rule match", "Bash", "pwd"},
		{"case insensitive exact", "bash", "PWD"},
		{"prefix rule match", "Bash", "git"},
		{"prefix with args", "Bash", "git status"},
		{"any tool rule", "Read", "/any/path"},
		{"case insensitive any tool", "read", "./file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.Evaluate(tt.tool, tt.input); got != DecisionAllow {
				t.Errorf("Evaluate() = %v, want %v", got, DecisionAllow)
			}
		})
	}
}

// TestPolicy_Evaluate_RequireApproval tests cases where no rules match.
func TestPolicy_Evaluate_RequireApproval(t *testing.T) {
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "pwd"},
		PrefixRule{Tool: "Bash", Prefix: "git"},
	}
	policy := NewPolicy(rules)

	tests := []struct {
		name        string
		tool, input string
	}{
		{"different tool", "Read", "/some/path"},
		{"different bash command", "Bash", "npm"},
		{"prefix boundary", "Bash", "gitdiff"}, // no space after git
		{"empty input no rule", "Bash", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.Evaluate(tt.tool, tt.input); got != DecisionRequireApproval {
				t.Errorf("Evaluate() = %v, want %v", got, DecisionRequireApproval)
			}
		})
	}
}

// TestPolicy_Evaluate_DefaultDeny tests custom default decision.
func TestPolicy_Evaluate_DefaultDeny(t *testing.T) {
	rules := []Rule{ExactRule{Tool: "Bash", Input: "git"}}
	policy := NewPolicyWithDefault(rules, DecisionDeny)

	tests := []struct {
		name        string
		tool, input string
		want        Decision
	}{
		{"matching rule", "Bash", "git", DecisionAllow},
		{"no match", "Bash", "npm", DecisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.Evaluate(tt.tool, tt.input); got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPolicy_Evaluate_EmptyRules tests policy with no rules.
func TestPolicy_Evaluate_EmptyRules(t *testing.T) {
	policy := NewPolicy([]Rule{})

	if got := policy.Evaluate("Bash", "anything"); got != DecisionRequireApproval {
		t.Errorf("Evaluate() = %v, want %v", got, DecisionRequireApproval)
	}
}

// TestPolicy_Evaluate_RuleOrder tests that rules are evaluated in order.
func TestPolicy_Evaluate_RuleOrder(t *testing.T) {
	// More specific rule first
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "git"},
		AnyToolRule{Tool: "Bash"}, // This would match "git" too, but exact rule comes first
	}
	policy := NewPolicy(rules)

	// Should still match (doesn't matter which rule matches, as long as one does)
	if got := policy.Evaluate("Bash", "git"); got != DecisionAllow {
		t.Errorf("Evaluate() = %v, want %v", got, DecisionAllow)
	}
}

// TestPolicy_AddRule tests the immutable AddRule method.
func TestPolicy_AddRule(t *testing.T) {
	rules := []Rule{ExactRule{Tool: "Bash", Input: "pwd"}}
	policy := NewPolicy(rules)

	// Add a rule
	newPolicy := policy.AddRule(PrefixRule{Tool: "Bash", Prefix: "git"})

	// Original should be unchanged
	if len(policy.Rules()) != 1 {
		t.Errorf("Original policy Rules() length = %d, want 1", len(policy.Rules()))
	}
	if policy.Evaluate("Bash", "git status") != DecisionRequireApproval {
		t.Errorf("Original policy should not allow 'git status'")
	}

	// New policy should have both rules
	if len(newPolicy.Rules()) != 2 {
		t.Errorf("New policy Rules() length = %d, want 2", len(newPolicy.Rules()))
	}
	if newPolicy.Evaluate("Bash", "git status") != DecisionAllow {
		t.Errorf("New policy should allow 'git status'")
	}

	// Original rules should still work
	if policy.Evaluate("Bash", "pwd") != DecisionAllow {
		t.Errorf("Original policy should still allow 'pwd'")
	}
}

// TestPolicy_SetDefaultDecision tests changing the default decision.
func TestPolicy_SetDefaultDecision(t *testing.T) {
	policy := NewPolicy([]Rule{})

	// Default is RequireApproval
	if policy.DefaultDecision() != DecisionRequireApproval {
		t.Errorf("DefaultDecision() = %v, want %v", policy.DefaultDecision(), DecisionRequireApproval)
	}

	// Change to Deny
	policy.SetDefaultDecision(DecisionDeny)
	if policy.DefaultDecision() != DecisionDeny {
		t.Errorf("SetDefaultDecision(Deny) failed, got %v", policy.DefaultDecision())
	}

	if got := policy.Evaluate("Bash", "anything"); got != DecisionDeny {
		t.Errorf("Evaluate() = %v, want %v", got, DecisionDeny)
	}
}

// TestPolicy_Rules tests that Rules() returns a copy, not the internal slice.
func TestPolicy_Rules(t *testing.T) {
	rules := []Rule{ExactRule{Tool: "Bash", Input: "pwd"}}
	policy := NewPolicy(rules)

	// Get rules
	retrievedRules := policy.Rules()

	// Modify the returned slice
	retrievedRules[0] = PrefixRule{Tool: "Bash", Prefix: "npm"}

	// Policy should be unchanged
	policyRules := policy.Rules()
	if _, ok := policyRules[0].(ExactRule); !ok {
		t.Error("Modifying returned slice changed internal state")
	}
}

// TestDecision_String tests the Decision.String method.
func TestDecision_String(t *testing.T) {
	tests := []struct {
		decision Decision
		want     string
	}{
		{DecisionAllow, "Allow"},
		{DecisionDeny, "Deny"},
		{DecisionRequireApproval, "RequireApproval"},
		{Decision(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.decision.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPolicy_ConcurrentAccess tests that Policy is safe for concurrent reads.
func TestPolicy_ConcurrentAccess(t *testing.T) {
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "pwd"},
		PrefixRule{Tool: "Bash", Prefix: "git"},
		AnyToolRule{Tool: "Read"},
	}
	policy := NewPolicy(rules)

	done := make(chan bool)

	// Run multiple goroutines reading from the policy
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				policy.Evaluate("Bash", "git status")
				policy.Rules()
				policy.DefaultDecision()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Policy should still work correctly
	if policy.Evaluate("Bash", "pwd") != DecisionAllow {
		t.Error("Concurrent access corrupted policy state")
	}
}

// TestPolicy_ParseAndEvaluate tests end-to-end workflow.
func TestPolicy_ParseAndEvaluate(t *testing.T) {
	ruleStrings := []string{"Bash(pwd)", "Bash(git *)", "Read"}

	var rules []Rule
	for _, s := range ruleStrings {
		rule, err := ParseRule(s)
		if err != nil {
			t.Fatalf("ParseRule(%q) error = %v", s, err)
		}
		rules = append(rules, rule)
	}

	policy := NewPolicy(rules)

	tests := []struct {
		tool, input string
		want        Decision
	}{
		{"Bash", "pwd", DecisionAllow},
		{"Bash", "git status", DecisionAllow},
		{"Read", "/anything", DecisionAllow},
		{"Bash", "npm", DecisionRequireApproval},
		{"Write", "/file.txt", DecisionRequireApproval},
	}

	for _, tt := range tests {
		t.Run(tt.tool+"/"+tt.input, func(t *testing.T) {
			if got := policy.Evaluate(tt.tool, tt.input); got != tt.want {
				t.Errorf("Evaluate(%q, %q) = %v, want %v", tt.tool, tt.input, got, tt.want)
			}
		})
	}
}
