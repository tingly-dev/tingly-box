package agent

import (
	"testing"
)

// Tests for ParseAgentType function

func TestParseAgentType(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    AgentType
		expectError bool
	}{
		// Valid Claude Code aliases
		{"cc alias", "cc", AgentTypeClaudeCode, false},
		{"claude alias", "claude", AgentTypeClaudeCode, false},
		{"claude-code full", "claude-code", AgentTypeClaudeCode, false},
		{"claudecode combined", "claudecode", AgentTypeClaudeCode, false},
		{"CC uppercase", "CC", AgentTypeClaudeCode, false},
		{"Claude-Code mixed case", "Claude-Code", AgentTypeClaudeCode, false},

		// Valid OpenCode aliases
		{"oc alias", "oc", AgentTypeOpenCode, false},
		{"opencode full", "opencode", AgentTypeOpenCode, false},
		{"open-code with dash", "open-code", AgentTypeOpenCode, false},
		{"OC uppercase", "OC", AgentTypeOpenCode, false},

		// Whitespace handling
		{"cc with leading space", "  cc", AgentTypeClaudeCode, false},
		{"cc with trailing space", "cc  ", AgentTypeClaudeCode, false},

		// Invalid inputs
		{"empty string", "", "", true},
		{"invalid type", "invalid", "", true},
		{"codex not supported", "codex", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAgentType(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("ParseAgentType(%q) expected error, got nil", tt.input)
				}
				if result != "" {
					t.Errorf("ParseAgentType(%q) error case should return empty AgentType, got %q", tt.input, result)
				}
			} else {
				if err != nil {
					t.Errorf("ParseAgentType(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("ParseAgentType(%q) = %q, want %q", tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestAgentTypeIsValid(t *testing.T) {
	tests := []struct {
		name      string
		agentType AgentType
		valid     bool
	}{
		{"ClaudeCode valid", AgentTypeClaudeCode, true},
		{"OpenCode valid", AgentTypeOpenCode, true},
		{"empty invalid", "", false},
		{"random string invalid", "random", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.agentType.IsValid()
			if result != tt.valid {
				t.Errorf("AgentType.IsValid(%q) = %v, want %v", tt.agentType, result, tt.valid)
			}
		})
	}
}

func TestListAgentInfo(t *testing.T) {
	info := ListAgentInfo()

	if len(info) != 2 {
		t.Errorf("ListAgentInfo() returned %d items, want 2", len(info))
	}

	// Check Claude Code info
	var ccInfo *AgentInfo
	for _, i := range info {
		if i.Type == AgentTypeClaudeCode {
			ccInfo = &i
			break
		}
	}
	if ccInfo == nil {
		t.Fatal("Claude Code agent info not found")
	}
	if ccInfo.Name != "Claude Code" {
		t.Errorf("Claude Code name = %q, want 'Claude Code'", ccInfo.Name)
	}
	if len(ccInfo.ConfigFiles) != 2 {
		t.Errorf("Claude Code config files = %d, want 2", len(ccInfo.ConfigFiles))
	}
}
