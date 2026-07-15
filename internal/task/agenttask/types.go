package agenttask

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TaskType = "agent"

	DefaultFollowUpDelaySeconds = 5 * 60
	DefaultMaxWakeUps           = 20
	DefaultTimeoutSeconds       = 30 * 60
	MaxSteps                    = 50
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

type Step struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Instruction string `json:"instruction"`
}

type StepOutcome struct {
	StepID      string    `json:"step_id"`
	Result      Result    `json:"result"`
	CompletedAt time.Time `json:"completed_at"`
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
	Steps          []Step         `json:"steps,omitempty"`
	CurrentStep    int            `json:"current_step,omitempty"`
	StepOutcomes   []StepOutcome  `json:"step_outcomes,omitempty"`
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
	if len(p.Steps) > MaxSteps {
		return fmt.Errorf("steps cannot exceed %d", MaxSteps)
	}
	if p.CurrentStep < 0 || p.CurrentStep > len(p.Steps) {
		return errors.New("current_step is outside the step list")
	}
	if len(p.StepOutcomes) != p.CurrentStep {
		return errors.New("step_outcomes must match completed steps")
	}
	for i, step := range p.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("step %d id is required", i+1)
		}
		if strings.TrimSpace(step.Title) == "" {
			return fmt.Errorf("step %d title is required", i+1)
		}
		if strings.TrimSpace(step.Instruction) == "" {
			return fmt.Errorf("step %d instruction is required", i+1)
		}
		if i < len(p.StepOutcomes) && p.StepOutcomes[i].StepID != step.ID {
			return fmt.Errorf("step outcome %d does not match its step", i+1)
		}
	}
	return nil
}

func (p Payload) HasCurrentStep() bool {
	return p.CurrentStep >= 0 && p.CurrentStep < len(p.Steps)
}

type Result struct {
	State             string   `json:"state"`
	Summary           string   `json:"summary"`
	Question          string   `json:"question,omitempty"`
	Artifacts         []string `json:"artifacts,omitempty"`
	NativeSessionID   string   `json:"native_session_id"`
	SuggestedDelaySec int      `json:"suggested_delay_seconds,omitempty"`
}
