package agentsec

import (
	"testing"
)

// TestExactRule_Matches tests the ExactRule.Matches method.
func TestExactRule_Matches(t *testing.T) {
	rule := ExactRule{Tool: "Bash", Input: "git"}

	tests := []struct {
		name        string
		tool, input string
		want        bool
	}{
		{"exact match", "Bash", "git", true},
		{"case insensitive tool", "bash", "git", true},
		{"case insensitive input", "Bash", "GIT", true},
		{"both case insensitive", "bash", "GIT", true},
		{"with arguments denied", "Bash", "git status", false},
		{"different tool", "Read", "git", false},
		{"different input", "Bash", "npm", false},
		{"empty input", "Bash", "", false},
		{"partial input", "Bash", "gitdiff", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rule.Matches(tt.tool, tt.input); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExactRule_String tests the ExactRule.String method.
func TestExactRule_String(t *testing.T) {
	tests := []struct {
		name string
		rule ExactRule
		want string
	}{
		{"bash git", ExactRule{Tool: "Bash", Input: "git"}, "Bash(git)"},
		{"read path", ExactRule{Tool: "Read", Input: "./src/main.go"}, "Read(./src/main.go)"},
		{"lowercase tool", ExactRule{Tool: "bash", Input: "ls"}, "bash(ls)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPrefixRule_Matches tests the PrefixRule.Matches method.
func TestPrefixRule_Matches(t *testing.T) {
	rule := PrefixRule{Tool: "Bash", Prefix: "git"}

	tests := []struct {
		name        string
		tool, input string
		want        bool
	}{
		{"exact prefix match", "Bash", "git", true},
		{"with single arg", "Bash", "git status", true},
		{"with multiple args", "Bash", "git commit -m message", true},
		{"case insensitive tool", "bash", "git status", true},
		{"case insensitive prefix", "Bash", "GIT status", true},
		{"both case insensitive", "bash", "GIT STATUS", true},
		{"prefix as word boundary", "Bash", "gitdiff", false}, // no space after git
		{"different tool", "Read", "git status", false},
		{"different prefix", "Bash", "npm install", false},
		{"empty input", "Bash", "", false},
		{"just space", "Bash", "git ", true}, // trailing space is valid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rule.Matches(tt.tool, tt.input); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPrefixRule_String tests the PrefixRule.String method.
func TestPrefixRule_String(t *testing.T) {
	tests := []struct {
		name string
		rule PrefixRule
		want string
	}{
		{"bash git", PrefixRule{Tool: "Bash", Prefix: "git"}, "Bash(git *)"},
		{"npm install", PrefixRule{Tool: "Bash", Prefix: "npm"}, "Bash(npm *)"},
		{"case preserved", PrefixRule{Tool: "bash", Prefix: "ls"}, "bash(ls *)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAnyToolRule_Matches tests the AnyToolRule.Matches method.
func TestAnyToolRule_Matches(t *testing.T) {
	rule := AnyToolRule{Tool: "Read"}

	tests := []struct {
		name        string
		tool, input string
		want        bool
	}{
		{"matching tool any input", "Read", "/any/path", true},
		{"matching tool empty input", "Read", "", true},
		{"case insensitive", "read", "anything", true},
		{"uppercase match", "READ", "foo", true},
		{"different tool", "Bash", "anything", false},
		{"tool substring", "Reader", "anything", false}, // not EqualFold
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rule.Matches(tt.tool, tt.input); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAnyToolRule_String tests the AnyToolRule.String method.
func TestAnyToolRule_String(t *testing.T) {
	tests := []struct {
		name string
		rule AnyToolRule
		want string
	}{
		{"Read tool", AnyToolRule{Tool: "Read"}, "Read"},
		{"Bash tool", AnyToolRule{Tool: "Bash"}, "Bash"},
		{"lowercase", AnyToolRule{Tool: "write"}, "write"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseRule tests the ParseRule function.
func TestParseRule(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantType   string // "ExactRule", "PrefixRule", "AnyToolRule"
		wantTool   string
		wantInput  string
		wantString string
		wantErr    bool
	}{
		{
			name:       "exact rule",
			input:      "Bash(git)",
			wantType:   "ExactRule",
			wantTool:   "Bash",
			wantInput:  "git",
			wantString: "Bash(git)",
		},
		{
			name:       "prefix rule",
			input:      "Bash(git *)",
			wantType:   "PrefixRule",
			wantTool:   "Bash",
			wantInput:  "git",
			wantString: "Bash(git *)",
		},
		{
			name:       "any tool rule",
			input:      "Read",
			wantType:   "AnyToolRule",
			wantTool:   "Read",
			wantString: "Read",
		},
		{
			name:       "pattern with spaces",
			input:      "Bash(rm -rf *)",
			wantType:   "PrefixRule",
			wantTool:   "Bash",
			wantInput:  "rm -rf",
			wantString: "Bash(rm -rf *)",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "Bash(",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRule(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// Check type
			switch r := got.(type) {
			case ExactRule:
				if tt.wantType != "ExactRule" {
					t.Errorf("got ExactRule, want %s", tt.wantType)
				}
				if r.Tool != tt.wantTool || r.Input != tt.wantInput {
					t.Errorf("ExactRule = {Tool: %q, Input: %q}, want {Tool: %q, Input: %q}",
						r.Tool, r.Input, tt.wantTool, tt.wantInput)
				}
			case PrefixRule:
				if tt.wantType != "PrefixRule" {
					t.Errorf("got PrefixRule, want %s", tt.wantType)
				}
				if r.Tool != tt.wantTool || r.Prefix != tt.wantInput {
					t.Errorf("PrefixRule = {Tool: %q, Prefix: %q}, want {Tool: %q, Prefix: %q}",
						r.Tool, r.Prefix, tt.wantTool, tt.wantInput)
				}
			case AnyToolRule:
				if tt.wantType != "AnyToolRule" {
					t.Errorf("got AnyToolRule, want %s", tt.wantType)
				}
				if r.Tool != tt.wantTool {
					t.Errorf("AnyToolRule.Tool = %q, want %q", r.Tool, tt.wantTool)
				}
			default:
				t.Errorf("unknown rule type: %T", got)
			}

			// Check String() round-trip
			if got.String() != tt.wantString {
				t.Errorf("String() = %q, want %q", got.String(), tt.wantString)
			}
		})
	}
}

// TestRuleRoundTrip tests that String() -> ParseRule -> String() is identity.
func TestRuleRoundTrip(t *testing.T) {
	rules := []Rule{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
		AnyToolRule{Tool: "Read"},
		ExactRule{Tool: "Write", Input: "./file.txt"},
	}

	for _, original := range rules {
		t.Run(original.String(), func(t *testing.T) {
			// Serialize to string
			s := original.String()

			// Parse back to Rule
			parsed, err := ParseRule(s)
			if err != nil {
				t.Fatalf("ParseRule(%q) error = %v", s, err)
			}

			// Serialize again
			reserialized := parsed.String()

			// Should match original
			if reserialized != s {
				t.Errorf("Round trip failed: %q -> %q", s, reserialized)
			}

			// Parsed rule should behave identically
			testCases := []struct{ tool, input string }{
				{"Bash", "git"}, {"Bash", "npm status"}, {"Read", "any/path"},
			}
			for _, tc := range testCases {
				if original.Matches(tc.tool, tc.input) != parsed.Matches(tc.tool, tc.input) {
					t.Errorf("Matches(%q, %q) differs: original=%v, parsed=%v",
						tc.tool, tc.input, original.Matches(tc.tool, tc.input), parsed.Matches(tc.tool, tc.input))
				}
			}
		})
	}
}
