package agentsec

import (
	"strings"
	"testing"
)

// TestBackwardCompatibility_DefaultBashAllowlist tests that the default
// allowlist can be converted and used with the new Policy.
func TestBackwardCompatibility_DefaultBashAllowlist(t *testing.T) {
	// Convert DefaultBashAllowlist strings to Rules
	var newRules []Rule
	for _, ruleStr := range DefaultBashAllowlist {
		rule, err := ParseRule(ruleStr)
		if err != nil {
			t.Fatalf("ParseRule(%q) error = %v", ruleStr, err)
		}
		newRules = append(newRules, rule)
	}

	policy := NewPolicy(newRules)

	// Test that expected commands are allowed
	tests := []struct {
		tool, input string
		want        Decision
	}{
		// From DefaultBashAllowlist
		{"Bash", "ls", DecisionAllow},
		{"Bash", "pwd", DecisionAllow},
		{"Bash", "git", DecisionAllow},
		{"Bash", "git status", DecisionAllow},
		{"Bash", "cat file.txt", DecisionAllow},
		{"Bash", "mkdir dir", DecisionAllow},
		{"Bash", "curl https://example.com", DecisionAllow},
		// Not in allowlist
		{"Bash", "npm", DecisionRequireApproval},
		{"Bash", "python", DecisionRequireApproval},
		// Different tool
		{"Read", "/file.txt", DecisionRequireApproval},
	}

	for _, tt := range tests {
		t.Run(tt.tool+"/"+tt.input, func(t *testing.T) {
			if got := policy.Evaluate(tt.tool, tt.input); got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPolicy_Immutability tests that Policy modifications don't affect
// the original policy (important for concurrent use).
func TestPolicy_Immutability(t *testing.T) {
	originalRules := []Rule{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
	}
	policy := NewPolicy(originalRules)

	// Modify the original slice (this shouldn't affect the policy)
	originalRules[0] = AnyToolRule{Tool: "Read"}

	// Policy should still use git rule, not Read
	if policy.Evaluate("Bash", "git") != DecisionAllow {
		t.Error("Modifying original rules slice affected policy")
	}
}

// TestRule_StringImmutability tests that String() returns a consistent value.
func TestRule_StringImmutability(t *testing.T) {
	rule := ExactRule{Tool: "Bash", Input: "git"}
	s1 := rule.String()
	s2 := rule.String()

	// Should be equal
	if s1 != s2 {
		t.Errorf("String() not consistent: %q != %q", s1, s2)
	}

	// Modify one string
	upper := strings.ToUpper(s1)

	// The other should be unchanged
	if s2 == upper {
		t.Error("String() returned same instance")
	}
}

// Benchmark: Rule Matches

func BenchmarkExactRule_Matches(b *testing.B) {
	rule := ExactRule{Tool: "Bash", Input: "git"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rule.Matches("Bash", "git")
	}
}

func BenchmarkPrefixRule_Matches(b *testing.B) {
	rule := PrefixRule{Tool: "Bash", Prefix: "git"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rule.Matches("Bash", "git status")
	}
}

func BenchmarkAnyToolRule_Matches(b *testing.B) {
	rule := AnyToolRule{Tool: "Read"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rule.Matches("Read", "/any/path")
	}
}

// Benchmark: Policy Evaluation

func BenchmarkPolicy_Evaluate_SingleRule(b *testing.B) {
	policy := NewPolicy([]Rule{ExactRule{Tool: "Bash", Input: "git"}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy.Evaluate("Bash", "git")
	}
}

func BenchmarkPolicy_Evaluate_TenRules(b *testing.B) {
	rules := make([]Rule, 10)
	for i := range rules {
		rules[i] = ExactRule{Tool: "Bash", Input: "cmd"}
	}
	policy := NewPolicy(rules)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy.Evaluate("Bash", "cmd")
	}
}

func BenchmarkPolicy_Evaluate_NoMatch(b *testing.B) {
	policy := NewPolicy([]Rule{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy.Evaluate("Bash", "python") // No match
	}
}

// Benchmark: ParseRule

func BenchmarkParseRule(b *testing.B) {
	input := "Bash(git *)"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseRule(input)
	}
}

// Benchmark: Round-trip serialization

func BenchmarkRuleRoundTrip(b *testing.B) {
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
		AnyToolRule{Tool: "Read"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, rule := range rules {
			s := rule.String()
			ParseRule(s)
		}
	}
}

// Benchmark: Concurrent access

func BenchmarkPolicy_ConcurrentReads(b *testing.B) {
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
		AnyToolRule{Tool: "Read"},
	}
	policy := NewPolicy(rules)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			policy.Evaluate("Bash", "git status")
			policy.Rules()
		}
	})
}

// TestPolicy_AddRulePerformance tests that AddRule doesn't become
// prohibitively expensive as rules grow.
func TestPolicy_AddRulePerformance(t *testing.T) {
	policy := NewPolicy([]Rule{})

	// Add 100 rules
	for i := 0; i < 100; i++ {
		policy = policy.AddRule(ExactRule{Tool: "Bash", Input: "cmd"})
	}

	if len(policy.Rules()) != 100 {
		t.Errorf("After adding 100 rules, got %d", len(policy.Rules()))
	}

	// Verify all rules are still there (policy is immutable, so each add created new policy)
	if policy.Evaluate("Bash", "cmd") != DecisionAllow {
		t.Error("Added rules not working")
	}
}

// TestEndToEnd_BashWorkflow simulates a realistic bash permission workflow.
func TestEndToEnd_BashWorkflow(t *testing.T) {
	// Start with default allowlist
	var rules []Rule
	for _, ruleStr := range DefaultBashAllowlist {
		rule, _ := ParseRule(ruleStr)
		rules = append(rules, rule)
	}
	policy := NewPolicy(rules)

	// Scenario 1: User tries 'npm start' (not in allowlist)
	if decision := policy.Evaluate("Bash", "npm start"); decision != DecisionRequireApproval {
		t.Errorf("npm start should require approval, got %v", decision)
	}

	// Scenario 2: User approves "Always Allow npm" with args
	policy = policy.AddRule(PrefixRule{Tool: "Bash", Prefix: "npm"})

	// Now npm commands should be allowed
	tests := []struct {
		input string
		want  Decision
	}{
		{"npm", DecisionAllow},
		{"npm start", DecisionAllow},
		{"npm install package", DecisionAllow},
		{"npm test", DecisionAllow},
	}

	for _, tt := range tests {
		if got := policy.Evaluate("Bash", tt.input); got != tt.want {
			t.Errorf("npm %q: got %v, want %v", tt.input, got, tt.want)
		}
	}

	// Scenario 3: Existing allowlist still works
	if policy.Evaluate("Bash", "git status") != DecisionAllow {
		t.Error("git status should still be allowed")
	}

	// Scenario 4: Different tool still requires approval
	if policy.Evaluate("Read", "/file.txt") != DecisionRequireApproval {
		t.Error("Read tool should require approval")
	}
}
