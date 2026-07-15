package agenttask

import (
	"errors"
	"fmt"
	"strings"
)

const (
	TaskType = "agent"

	DefaultFollowUpDelaySeconds = 5 * 60
	DefaultMaxWakeUps           = 20
	DefaultTimeoutSeconds       = 30 * 60
)

type AgentKind string

const (
	AgentClaude AgentKind = "claude"
	AgentCodex  AgentKind = "codex"
)

func (k AgentKind) IsValid() bool {
	return k == AgentClaude || k == AgentCodex
}

type FollowUpPolicy struct {
	Enabled      bool `json:"enabled"`
	DelaySeconds int  `json:"delay_seconds"`
	MaxWakeUps   int  `json:"max_wake_ups"`
}

type Payload struct {
	Version        int            `json:"version"`
	Title          string         `json:"title"`
	Goal           string         `json:"goal"`
	Agent          AgentKind      `json:"agent"`
	WorkspacePath  string         `json:"workspace_path"`
	SessionID      string         `json:"session_id,omitempty"`
	PendingInput   string         `json:"pending_input,omitempty"`
	FollowUp       FollowUpPolicy `json:"follow_up"`
	WakeCount      int            `json:"wake_count"`
	TimeoutSeconds int            `json:"timeout_seconds"`
}

func (p *Payload) ApplyDefaults() {
	if p.Version == 0 {
		p.Version = 1
	}
	if p.FollowUp.DelaySeconds <= 0 {
		p.FollowUp.DelaySeconds = DefaultFollowUpDelaySeconds
	}
	if p.FollowUp.MaxWakeUps <= 0 {
		p.FollowUp.MaxWakeUps = DefaultMaxWakeUps
	}
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = DefaultTimeoutSeconds
	}
}

func (p Payload) Validate() error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported payload version %d", p.Version)
	}
	if strings.TrimSpace(p.Goal) == "" {
		return errors.New("goal is required")
	}
	if !p.Agent.IsValid() {
		return fmt.Errorf("unsupported agent %q", p.Agent)
	}
	if strings.TrimSpace(p.WorkspacePath) == "" {
		return errors.New("workspace_path is required")
	}
	if p.WakeCount < 0 {
		return errors.New("wake_count cannot be negative")
	}
	return nil
}

type Result struct {
	State             string   `json:"state"`
	Summary           string   `json:"summary"`
	Question          string   `json:"question,omitempty"`
	Artifacts         []string `json:"artifacts,omitempty"`
	NativeSessionID   string   `json:"native_session_id"`
	SuggestedDelaySec int      `json:"suggested_delay_seconds,omitempty"`
}
