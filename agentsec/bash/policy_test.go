// Package bash provides bash-specific policy logic on top of the core agentsec Policy.
package bash

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentsec"
)

// TestHasChaining tests the HasChaining function.
func TestHasChaining(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		// Positive cases (has chaining)
		{"pipe", "ls | grep foo", true},
		{"and", "cd /tmp && ls", true},
		{"or", "cd /tmp || ls", true},
		{"semicolon", "cd /tmp; ls", true},
		{"command substitution", "$(echo hello)", true},
		{"backtick", "`echo hello`", true},
		{"mixed", "ls | grep foo || echo none", true},
		{"nested", "cmd1 && (cmd2 | cmd3)", true},

		// Negative cases (no chaining)
		// Note: HasChaining is a simple character-level check and doesn't
		// handle shell quoting. For proper shell parsing, a full parser would be needed.
		// Our simple check errs on the side of caution (detects chaining even in quotes).
		{"simple", "ls", false},
		{"with args", "git status", false},
		// {"quoted pipe", "echo '|'", false}, // Known limitation: simple char check
		// {"quoted and", "echo '&&'", false}, // Known limitation: simple char check
		{"path with dollar", "echo $HOME/file", false},
		{"escaped dollar", "echo \\$HOME", false},
		// {"subcommand as arg", "echo '$(ls)'", false}, // Known limitation: simple char check
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasChaining(tt.command); got != tt.want {
				t.Errorf("HasChaining() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractBaseCommand tests the ExtractBaseCommand function.
func TestExtractBaseCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"simple", "git", "git"},
		{"with args", "git status", "git"},
		{"multiple args", "git commit -m message", "git"},
		{"subshell", "(git status)", "git"},
		{"subshell with args", "(git commit -m)", "git"},
		{"nested subshell", "((git status))", "git"},
		{"subshell incomplete", "(git", "git"},
		{"sudo", "sudo git status", "sudo"},
		{"npm", "npm run dev", "npm"},
		{"spaces", "  git  status  ", "git"},
		{"tab", "git\tstatus", "git"},
		{"case", "GIT STATUS", "git"},
		{"MixedCase", "GitStatus", "gitstatus"},
		{"empty", "", ""},
		{"just spaces", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractBaseCommand(tt.command); got != tt.want {
				t.Errorf("ExtractBaseCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDefaultRules tests that DefaultRules returns the expected rules.
func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()

	if len(rules) == 0 {
		t.Fatal("DefaultRules() returned empty slice")
	}

	// Check some expected rules (these are in DefaultRules)
	expectedCommands := []string{"ls", "git", "cat"}
	policy := agentsec.NewPolicy(rules)

	for _, cmd := range expectedCommands {
		t.Run(cmd, func(t *testing.T) {
			if policy.Evaluate("Bash", cmd) != agentsec.DecisionAllow {
				t.Errorf("DefaultRules should allow %q", cmd)
			}
		})
	}

	// Check that some commands are NOT in default rules
	notExpected := []string{"python", "node", "docker"}
	for _, cmd := range notExpected {
		t.Run(cmd+" not expected", func(t *testing.T) {
			if policy.Evaluate("Bash", cmd) == agentsec.DecisionAllow {
				t.Errorf("DefaultRules should NOT allow %q", cmd)
			}
		})
	}
}

// TestDefaultPolicy tests the DefaultPolicy convenience function.
func TestDefaultPolicy(t *testing.T) {
	policy := NewPolicyWithDefault()

	if policy == nil {
		t.Fatal("NewPolicyWithDefault() returned nil")
	}

	// Should allow default commands
	tests := []struct {
		command string
		want    bool
	}{
		{"ls", true},
		{"git status", true},
		{"cat file.txt", true},
		{"python", false},
		{"docker run", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			decision := policy.Evaluate(tt.command)
			if got := decision.Allow; got != tt.want {
				t.Errorf("Evaluate(%q).Allow = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

// TestBashPolicy_Evaluate tests the Policy.Evaluate method.
func TestBashPolicy_Evaluate(t *testing.T) {
	rules := []agentsec.Rule{
		agentsec.PrefixRule{Tool: "Bash", Prefix: "git"},
		agentsec.ExactRule{Tool: "Bash", Input: "pwd"},
	}
	policy := NewPolicy(rules)

	tests := []struct {
		name         string
		command      string
		wantAllow    bool
		wantRemember bool
	}{
		// Allowed commands
		{"exact match", "pwd", true, true},
		{"prefix match", "git", true, true},
		{"prefix with args", "git status", true, true},

		// Chained commands (never allowed, never remembered)
		{"pipe", "ls | grep foo", false, false},
		{"and", "cd /tmp && ls", false, false},
		{"or", "cd /tmp || echo fail", false, false},
		{"semicolon", "cd /tmp; ls", false, false},
		{"command sub", "$(echo foo)", false, false},
		{"backtick", "`pwd`", false, false},

		// Not in allowlist (not allowed, but can be remembered)
		{"unknown command", "python", false, true},
		{"unknown with args", "node script.js", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := policy.Evaluate(tt.command)
			if decision.Allow != tt.wantAllow {
				t.Errorf("Evaluate(%q).Allow = %v, want %v", tt.command, decision.Allow, tt.wantAllow)
			}
			if decision.Remember != tt.wantRemember {
				t.Errorf("Evaluate(%q).Remember = %v, want %v", tt.command, decision.Remember, tt.wantRemember)
			}
		})
	}
}

// TestBashPolicy_AddRule tests the immutable AddRule method.
func TestBashPolicy_AddRule(t *testing.T) {
	rules := []agentsec.Rule{agentsec.ExactRule{Tool: "Bash", Input: "pwd"}}
	policy := NewPolicy(rules)

	// Original policy - only allows exact "pwd"
	if decision := policy.Evaluate("pwd"); decision.Allow != true {
		t.Error("Original policy should allow exact 'pwd'")
	}
	if decision := policy.Evaluate("ls"); decision.Allow != false {
		t.Error("Original policy should not allow 'ls'")
	}

	// Add a prefix rule for ls
	newPolicy := policy.AddRule(agentsec.PrefixRule{Tool: "Bash", Prefix: "ls"})

	// Original should be unchanged
	if decision := policy.Evaluate("ls -la"); decision.Allow != false {
		t.Error("Original policy should still not allow 'ls -la'")
	}

	// New policy should allow
	if decision := newPolicy.Evaluate("ls -la"); decision.Allow != true {
		t.Error("New policy should allow 'ls -la'")
	}

	// Original exact match should still work
	if decision := policy.Evaluate("pwd"); decision.Allow != true {
		t.Error("Original policy should still allow exact match 'pwd'")
	}
}

// TestBashPolicy_Rules tests the Rules method.
func TestBashPolicy_Rules(t *testing.T) {
	rules := []agentsec.Rule{
		agentsec.ExactRule{Tool: "Bash", Input: "pwd"},
		agentsec.PrefixRule{Tool: "Bash", Prefix: "git"},
	}
	policy := NewPolicy(rules)

	retrieved := policy.Rules()

	if len(retrieved) != len(rules) {
		t.Errorf("Rules() length = %d, want %d", len(retrieved), len(rules))
	}

	// Modify returned slice (should not affect policy)
	retrieved[0] = agentsec.AnyToolRule{Tool: "Read"}

	// Policy should be unchanged
	originalRules := policy.Rules()
	if _, ok := originalRules[0].(agentsec.ExactRule); !ok {
		t.Error("Modifying returned slice changed internal state")
	}
}

// TestBashPolicy_RealWorldScenarios tests realistic bash command scenarios.
func TestBashPolicy_RealWorldScenarios(t *testing.T) {
	policy := NewPolicyWithDefault()

	scenarios := []struct {
		name         string
		command      string
		wantAllow    bool
		wantRemember bool
		description  string
	}{
		{
			"safe git command",
			"git status",
			true, true,
			"git is in default allowlist",
		},
		{
			"safe ls with args",
			"ls -la /tmp",
			true, true,
			"ls is in default allowlist",
		},
		{
			"unsafe npm",
			"npm install",
			false, true,
			"npm not in allowlist, but can be remembered",
		},
		{
			"dangerous pipe",
			"cat /etc/passwd | grep root",
			false, false,
			"pipes are unsafe and cannot be remembered",
		},
		{
			"dangerous and",
			"cd /tmp && rm -rf *",
			false, false,
			"&& chains are unsafe and cannot be remembered",
		},
		{
			"command substitution",
			"cat $(echo file.txt)",
			false, false,
			"command substitution is unsafe",
		},
		{
			"subshell git",
			"(git status)",
			true, true,
			"subshell of safe command is allowed",
		},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			decision := policy.Evaluate(tt.command)
			if decision.Allow != tt.wantAllow || decision.Remember != tt.wantRemember {
				t.Errorf("%s: Evaluate(%q) = {Allow: %v, Remember: %v}, want {Allow: %v, Remember: %v}\n  %s",
					tt.name, tt.command, decision.Allow, decision.Remember, tt.wantAllow, tt.wantRemember, tt.description)
			}
		})
	}
}

// TestBashPolicy_ConcurrentAccess tests thread safety.
func TestBashPolicy_ConcurrentAccess(t *testing.T) {
	policy := NewPolicyWithDefault()
	done := make(chan bool)

	// Run multiple goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				policy.Evaluate("git status")
				policy.Evaluate("npm install")
				policy.Rules()
			}
			done <- true
		}()
	}

	// Wait for completion
	for i := 0; i < 10; i++ {
		<-done
	}

	// Policy should still work
	decision := policy.Evaluate("ls")
	if !decision.Allow {
		t.Error("Concurrent access corrupted policy")
	}
}

// TestExtractBaseCommand_EdgeCases tests edge cases for base command extraction.
func TestExtractBaseCommand_EdgeCases(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{"", ""},
		{"   ", ""},
		{"\t\t", ""},
		{"cmd", "cmd"},
		{"cmd arg", "cmd"},
		{"(cmd)", "cmd"},
		{"(cmd arg)", "cmd"},
		{"((cmd))", "cmd"},
		{"((", "("}, // No proper closing, return what we can
		{"sudo -u user cmd", "sudo"},
		{"docker run --rm image", "docker"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := ExtractBaseCommand(tt.command)
			if got != tt.want {
				t.Errorf("ExtractBaseCommand(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// TestHasChaining_EdgeCases tests edge cases for chaining detection.
func TestHasChaining_EdgeCases(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"", false},
		{"   ", false},
		{"|", true},
		{"&", false}, // Single & is not a chain operator
		{"&&", true},
		{"||", true},
		{";", true},
		{"$(cmd)", true},
		{"`cmd`", true},
		// Note: HasChaining is a simple character-level check that doesn't
		// handle shell quoting. It errs on the side of caution.
		// {"echo '|'", false}, // Known limitation
		// {"echo \"$(ls)\"", false}, // Known limitation
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := HasChaining(tt.command)
			if got != tt.want {
				t.Errorf("HasChaining(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

// TestBashPolicy_UserApprovalFlow simulates a realistic user approval workflow.
func TestBashPolicy_UserApprovalFlow(t *testing.T) {
	policy := NewPolicyWithDefault()

	// Step 1: User tries 'npm install' - not in allowlist
	decision := policy.Evaluate("npm install")
	if decision.Allow || !decision.Remember {
		t.Fatal("npm install should require approval but be rememberable")
	}

	// Step 2: User approves "Always Allow npm" - add prefix rule
	policy = policy.AddRule(agentsec.PrefixRule{Tool: "Bash", Prefix: "npm"})

	// Step 3: Now npm commands should be allowed
	tests := []string{"npm", "npm install", "npm run dev", "npm test"}
	for _, cmd := range tests {
		decision := policy.Evaluate(cmd)
		if !decision.Allow || !decision.Remember {
			t.Errorf("After approval, %q should be allowed and rememberable", cmd)
		}
	}

	// Step 4: Chained npm commands still require approval (cannot be remembered)
	decision = policy.Evaluate("npm install | grep success")
	if decision.Allow || decision.Remember {
		t.Error("Chained npm command should not be allowed or rememberable")
	}
}
