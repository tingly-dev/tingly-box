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

type LaunchProfile string

const (
	LaunchLegacyInherited LaunchProfile = "legacy_inherited"
	LaunchClaudePlan      LaunchProfile = "plan"
	LaunchClaudeManual    LaunchProfile = "manual"
	LaunchClaudeEdits     LaunchProfile = "accept_edits"
	LaunchCodexReadOnly   LaunchProfile = "read_only"
	LaunchCodexWorkspace  LaunchProfile = "workspace_write"
)

type ToolCapability string

const (
	ToolFilesRead  ToolCapability = "files_read"
	ToolFilesWrite ToolCapability = "files_write"
	ToolTerminal   ToolCapability = "terminal"
	ToolWeb        ToolCapability = "web"
)

type ExecutionPolicy struct {
	LaunchProfile LaunchProfile    `json:"launch_profile"`
	Tools         []ToolCapability `json:"tools,omitempty"`
}

func DefaultExecutionPolicy(agent AgentKind) ExecutionPolicy {
	switch agent {
	case AgentClaude:
		return ExecutionPolicy{
			LaunchProfile: LaunchClaudeEdits,
			Tools:         []ToolCapability{ToolFilesRead, ToolFilesWrite},
		}
	case AgentCodex:
		return ExecutionPolicy{LaunchProfile: LaunchCodexWorkspace}
	default:
		return ExecutionPolicy{}
	}
}

func (p *ExecutionPolicy) ApplyDefaults(agent AgentKind, legacy bool) {
	if p.LaunchProfile == "" {
		if legacy {
			p.LaunchProfile = LaunchLegacyInherited
		} else {
			*p = DefaultExecutionPolicy(agent)
		}
	}
	if p.Tools == nil && agent == AgentClaude && p.LaunchProfile != LaunchLegacyInherited {
		if p.LaunchProfile == LaunchClaudePlan {
			p.Tools = []ToolCapability{ToolFilesRead}
		} else {
			p.Tools = []ToolCapability{ToolFilesRead, ToolFilesWrite}
		}
	}
}

func (p ExecutionPolicy) Validate(agent AgentKind, allowLegacy bool) error {
	if p.LaunchProfile == LaunchLegacyInherited {
		if allowLegacy {
			return nil
		}
		return errors.New("legacy_inherited cannot be selected for a new run")
	}
	switch agent {
	case AgentClaude:
		switch p.LaunchProfile {
		case LaunchClaudePlan, LaunchClaudeEdits:
		case LaunchClaudeManual:
			if !allowLegacy {
				return errors.New("manual is not available for unattended tasks")
			}
		default:
			return fmt.Errorf("unsupported Claude launch profile %q", p.LaunchProfile)
		}
		if len(p.Tools) == 0 {
			return errors.New("Claude execution requires at least one tool capability")
		}
		seen := make(map[ToolCapability]struct{}, len(p.Tools))
		for _, tool := range p.Tools {
			switch tool {
			case ToolFilesRead, ToolFilesWrite, ToolTerminal, ToolWeb:
			default:
				return fmt.Errorf("unsupported tool capability %q", tool)
			}
			if _, exists := seen[tool]; exists {
				return fmt.Errorf("duplicate tool capability %q", tool)
			}
			seen[tool] = struct{}{}
		}
	case AgentCodex:
		if p.LaunchProfile != LaunchCodexReadOnly && p.LaunchProfile != LaunchCodexWorkspace {
			return fmt.Errorf("unsupported Codex launch profile %q", p.LaunchProfile)
		}
		if len(p.Tools) != 0 {
			return errors.New("Codex does not support per-task tool filtering")
		}
	default:
		return fmt.Errorf("unsupported agent %q", agent)
	}
	return nil
}

// Automated returns the effective policy for an unattended Run. Historical
// interactive or inherited policies are narrowed instead of silently gaining
// write privileges.
func (p ExecutionPolicy) Automated(agent AgentKind) ExecutionPolicy {
	switch agent {
	case AgentClaude:
		if p.LaunchProfile == LaunchClaudeManual || p.LaunchProfile == LaunchLegacyInherited {
			return ExecutionPolicy{LaunchProfile: LaunchClaudePlan, Tools: []ToolCapability{ToolFilesRead}}
		}
	case AgentCodex:
		if p.LaunchProfile == LaunchLegacyInherited {
			return ExecutionPolicy{LaunchProfile: LaunchCodexReadOnly}
		}
	}
	return p
}

func (p ExecutionPolicy) ClaudePermissionMode() string {
	switch p.LaunchProfile {
	case LaunchClaudePlan:
		return "plan"
	case LaunchClaudeManual:
		return "manual"
	case LaunchClaudeEdits:
		return "acceptEdits"
	default:
		return ""
	}
}

func (p ExecutionPolicy) ClaudeTools() []string {
	var tools []string
	for _, capability := range p.Tools {
		switch capability {
		case ToolFilesRead:
			tools = append(tools, "Read", "Glob", "Grep")
		case ToolFilesWrite:
			tools = append(tools, "Write", "Edit")
		case ToolTerminal:
			tools = append(tools, "Bash")
		case ToolWeb:
			tools = append(tools, "WebSearch", "WebFetch")
		}
	}
	return tools
}

func (p ExecutionPolicy) CodexSandboxMode() string {
	switch p.LaunchProfile {
	case LaunchCodexReadOnly:
		return "read-only"
	case LaunchCodexWorkspace:
		return "workspace-write"
	default:
		return ""
	}
}

type FollowUpPolicy struct {
	Enabled      bool `json:"enabled"`
	DelaySeconds int  `json:"delay_seconds"`
	MaxWakeUps   int  `json:"max_wake_ups"`
}

// StepExecutor selects how a step runs. Empty inherits the task's Agent.
// "shell" runs Step.Command as a bounded shell command instead of an agent.
type StepExecutor string

const (
	StepExecutorShell StepExecutor = "shell"
)

type Step struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Instruction string `json:"instruction"`
	// Executor overrides the step's run core. Empty = the task's Agent
	// (claude/codex). "shell" runs Command deterministically.
	Executor StepExecutor `json:"executor,omitempty"`
	// Command is the shell command for shell steps (ignored otherwise).
	Command string `json:"command,omitempty"`
}

// IsShell reports whether the step runs as a shell command.
func (s Step) IsShell() bool { return s.Executor == StepExecutorShell }

type StepOutcome struct {
	StepID      string    `json:"step_id"`
	Result      Result    `json:"result"`
	CompletedAt time.Time `json:"completed_at"`
}

type Payload struct {
	Version          int              `json:"version"`
	Title            string           `json:"title"`
	Goal             string           `json:"goal"`
	Agent            AgentKind        `json:"agent"`
	WorkspacePath    string           `json:"workspace_path"`
	SessionID        string           `json:"session_id,omitempty"`
	PendingInput     string           `json:"pending_input,omitempty"`
	FollowUp         FollowUpPolicy   `json:"follow_up"`
	WakeCount        int              `json:"wake_count"`
	TimeoutSeconds   int              `json:"timeout_seconds"`
	Execution        ExecutionPolicy  `json:"execution"`
	PendingExecution *ExecutionPolicy `json:"pending_execution,omitempty"`
	Steps            []Step           `json:"steps,omitempty"`
	CurrentStep      int              `json:"current_step,omitempty"`
	StepOutcomes     []StepOutcome    `json:"step_outcomes,omitempty"`
}

func (p *Payload) ApplyDefaults() {
	legacy := p.Version == 1
	if p.Version == 0 {
		p.Version = 2
	}
	p.Execution.ApplyDefaults(p.Agent, legacy)
	if p.PendingExecution != nil {
		p.PendingExecution.ApplyDefaults(p.Agent, false)
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
	if p.Version != 1 && p.Version != 2 {
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
	if err := p.Execution.Validate(p.Agent, true); err != nil {
		return fmt.Errorf("invalid execution policy: %w", err)
	}
	if p.PendingExecution != nil {
		if err := p.PendingExecution.Validate(p.Agent, false); err != nil {
			return fmt.Errorf("invalid pending execution policy: %w", err)
		}
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
		if step.IsShell() {
			if strings.TrimSpace(step.Command) == "" {
				return fmt.Errorf("step %d shell command is required", i+1)
			}
		} else if strings.TrimSpace(step.Instruction) == "" {
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
	ExitCode          *int     `json:"exit_code,omitempty"`
	DurationMS        int64    `json:"duration_ms"`
	ExitReason        string   `json:"exit_reason,omitempty"`
}
