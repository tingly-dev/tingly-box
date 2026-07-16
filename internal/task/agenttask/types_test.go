package agenttask

import "testing"

func TestExecutionPolicyDefaultsAndNativeMapping(t *testing.T) {
	claude := DefaultExecutionPolicy(AgentClaude)
	if claude.LaunchProfile != LaunchClaudeEdits || claude.ClaudePermissionMode() != "acceptEdits" {
		t.Fatalf("Claude policy = %+v", claude)
	}
	if got := claude.ClaudeTools(); len(got) != 5 || got[0] != "Read" || got[4] != "Edit" {
		t.Fatalf("Claude tools = %v", got)
	}

	codex := DefaultExecutionPolicy(AgentCodex)
	if codex.LaunchProfile != LaunchCodexWorkspace || codex.CodexSandboxMode() != "workspace-write" {
		t.Fatalf("Codex policy = %+v", codex)
	}
}

func TestExecutionPolicyValidationRejectsUnsupportedCombinations(t *testing.T) {
	tests := []struct {
		name   string
		agent  AgentKind
		policy ExecutionPolicy
	}{
		{"Codex tool filtering", AgentCodex, ExecutionPolicy{LaunchProfile: LaunchCodexWorkspace, Tools: []ToolCapability{ToolFilesRead}}},
		{"Claude Codex profile", AgentClaude, ExecutionPolicy{LaunchProfile: LaunchCodexWorkspace, Tools: []ToolCapability{ToolFilesRead}}},
		{"new legacy profile", AgentClaude, ExecutionPolicy{LaunchProfile: LaunchLegacyInherited}},
		{"duplicate tools", AgentClaude, ExecutionPolicy{LaunchProfile: LaunchClaudeManual, Tools: []ToolCapability{ToolFilesRead, ToolFilesRead}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.policy.Validate(tt.agent, false); err == nil {
				t.Fatalf("expected validation error for %+v", tt.policy)
			}
		})
	}
}

func TestPayloadVersionOneUsesLegacyInheritedPolicy(t *testing.T) {
	payload := Payload{Version: 1, Agent: AgentClaude}
	payload.ApplyDefaults()
	if payload.Execution.LaunchProfile != LaunchLegacyInherited {
		t.Fatalf("execution = %+v", payload.Execution)
	}
}
