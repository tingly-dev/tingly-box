package agent

import (
	"testing"
)

func TestParseAgentType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    AgentType
		wantErr bool
	}{
		// Claude Code aliases
		{
			name:    "cc alias",
			input:   "cc",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		{
			name:    "claude alias",
			input:   "claude",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		{
			name:    "claude-code full",
			input:   "claude-code",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		{
			name:    "claudecode combined",
			input:   "claudecode",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		{
			name:    "CC uppercase",
			input:   "CC",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		{
			name:    "Claude-Code mixed case",
			input:   "Claude-Code",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		// OpenCode aliases
		{
			name:    "oc alias",
			input:   "oc",
			want:    AgentTypeOpenCode,
			wantErr: false,
		},
		{
			name:    "opencode full",
			input:   "opencode",
			want:    AgentTypeOpenCode,
			wantErr: false,
		},
		{
			name:    "open-code with dash",
			input:   "open-code",
			want:    AgentTypeOpenCode,
			wantErr: false,
		},
		{
			name:    "OC uppercase",
			input:   "OC",
			want:    AgentTypeOpenCode,
			wantErr: false,
		},
		// Codex aliases
		{
			name:    "cx alias",
			input:   "cx",
			want:    AgentTypeCodex,
			wantErr: false,
		},
		{
			name:    "codex full",
			input:   "codex",
			want:    AgentTypeCodex,
			wantErr: false,
		},
		{
			name:    "CODEX uppercase",
			input:   "CODEX",
			want:    AgentTypeCodex,
			wantErr: false,
		},
		// Edge cases
		{
			name:    "cc with leading space",
			input:   "  cc",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		{
			name:    "cc with trailing space",
			input:   "cc  ",
			want:    AgentTypeClaudeCode,
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid type",
			input:   "invalid",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAgentType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAgentType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseAgentType() = %v, want %v", got, tt.want)
			}
		})
	}
}
