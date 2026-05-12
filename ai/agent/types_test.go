package agent

import (
	"testing"
)

func TestAgentTypeString(t *testing.T) {
	tests := []struct {
		name      string
		agentType AgentType
		want      string
	}{
		{
			name:      "ClaudeCode",
			agentType: AgentTypeClaudeCode,
			want:      "claude-code",
		},
		{
			name:      "OpenCode",
			agentType: AgentTypeOpenCode,
			want:      "opencode",
		},
		{
			name:      "Codex",
			agentType: AgentTypeCodex,
			want:      "codex",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agentType.String(); got != tt.want {
				t.Errorf("AgentType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentTypeIsValid(t *testing.T) {
	tests := []struct {
		name      string
		agentType AgentType
		want      bool
	}{
		{
			name:      "ClaudeCode valid",
			agentType: AgentTypeClaudeCode,
			want:      true,
		},
		{
			name:      "OpenCode valid",
			agentType: AgentTypeOpenCode,
			want:      true,
		},
		{
			name:      "Codex valid",
			agentType: AgentTypeCodex,
			want:      true,
		},
		{
			name:      "empty invalid",
			agentType: AgentType(""),
			want:      false,
		},
		{
			name:      "random string invalid",
			agentType: AgentType("random"),
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agentType.IsValid(); got != tt.want {
				t.Errorf("AgentType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListAgentInfo(t *testing.T) {
	infos := ListAgentInfo()

	if len(infos) != 3 {
		t.Errorf("ListAgentInfo() returned %d items, want 3", len(infos))
	}

	// Verify Claude Code
	ccInfo, ok := GetAgentInfo(AgentTypeClaudeCode)
	if !ok {
		t.Fatal("GetAgentInfo(ClaudeCode) returned false")
	}
	if ccInfo.Name != "Claude Code" {
		t.Errorf("Claude Code name = %s, want 'Claude Code'", ccInfo.Name)
	}
	if ccInfo.Scenario != "claude_code" {
		t.Errorf("Claude Code scenario = %s, want 'claude_code'", ccInfo.Scenario)
	}

	// Verify OpenCode
	ocInfo, ok := GetAgentInfo(AgentTypeOpenCode)
	if !ok {
		t.Fatal("GetAgentInfo(OpenCode) returned false")
	}
	if ocInfo.Name != "OpenCode" {
		t.Errorf("OpenCode name = %s, want 'OpenCode'", ocInfo.Name)
	}

	// Verify Codex
	cxInfo, ok := GetAgentInfo(AgentTypeCodex)
	if !ok {
		t.Fatal("GetAgentInfo(Codex) returned false")
	}
	if cxInfo.Name != "Codex" {
		t.Errorf("Codex name = %s, want 'Codex'", cxInfo.Name)
	}
}

func TestGetAgentInfoNotFound(t *testing.T) {
	_, ok := GetAgentInfo(AgentType("invalid"))
	if ok {
		t.Error("GetAgentInfo(invalid) returned true, want false")
	}
}
