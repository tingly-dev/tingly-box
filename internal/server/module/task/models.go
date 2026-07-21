package taskapi

import (
	"time"

	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
)

type CreateRequest struct {
	Title          string                     `json:"title"`
	Goal           string                     `json:"goal" binding:"required"`
	Agent          agenttask.AgentKind        `json:"agent" binding:"required"`
	WorkspacePath  string                     `json:"workspace_path,omitempty"`
	ScheduledAt    *time.Time                 `json:"scheduled_at,omitempty"`
	Recurrence     *coretask.RecurrenceSpec   `json:"recurrence,omitempty"`
	FollowUp       agenttask.FollowUpPolicy   `json:"follow_up"`
	TimeoutSeconds int                        `json:"timeout_seconds,omitempty"`
	Steps          []CreateStep               `json:"steps,omitempty"`
	Execution      *agenttask.ExecutionPolicy `json:"execution,omitempty"`
	Repeat         *agenttask.RepeatPolicy    `json:"repeat,omitempty"`
}

type CreateStep struct {
	// Executor: "" (agent) or "shell". Shell steps run Command; agent steps
	// run Instruction. (A0 heterogeneous steps.)
	Executor    string `json:"executor,omitempty"`
	Instruction string `json:"instruction,omitempty"`
	Command     string `json:"command,omitempty"`
	When        string `json:"when,omitempty"`
}

// UpdateRequest edits a non-running task's durable configuration.
// Workspace and executor are the task's identity and stay fixed; completed
// steps are immutable history. Everything else is editable between runs —
// each run snapshots its effective policy, so audit is unaffected.
type UpdateRequest struct {
	Title *string `json:"title,omitempty"`
	Goal  *string `json:"goal,omitempty"`
	// FollowUp / TimeoutSeconds / Execution / Steps apply to agent tasks.
	FollowUp       *agenttask.FollowUpPolicy  `json:"follow_up,omitempty"`
	TimeoutSeconds *int                       `json:"timeout_seconds,omitempty"`
	Execution      *agenttask.ExecutionPolicy `json:"execution,omitempty"`
	// Steps replaces the not-yet-completed tail of the step list.
	Steps *[]CreateStep `json:"steps,omitempty"`
	// Repeat replaces the repeat policy; ClearRepeat removes it.
	Repeat      *agenttask.RepeatPolicy `json:"repeat,omitempty"`
	ClearRepeat bool                    `json:"clear_repeat,omitempty"`
	// Recurrence replaces the schedule; ClearRecurrence switches to manual.
	Recurrence      *coretask.RecurrenceSpec `json:"recurrence,omitempty"`
	ClearRecurrence bool                     `json:"clear_recurrence,omitempty"`
}

type WakeRequest struct {
	Instruction       string                     `json:"instruction,omitempty"`
	ExecutionOverride *agenttask.ExecutionPolicy `json:"execution_override,omitempty"`
}

type AgentAvailability struct {
	Agent          agenttask.AgentKind       `json:"agent"`
	Available      bool                      `json:"available"`
	LaunchProfiles []agenttask.LaunchProfile `json:"launch_profiles"`
	DefaultProfile agenttask.LaunchProfile   `json:"default_profile"`
	ToolFiltering  bool                      `json:"tool_filtering"`
	Unattended     bool                      `json:"unattended"`
}

type TaskView struct {
	ID            string                    `json:"id"`
	Title         string                    `json:"title"`
	Goal          string                    `json:"goal"`
	Agent         agenttask.AgentKind       `json:"agent"`
	Status        coretask.TaskStatus       `json:"status"`
	Progress      string                    `json:"progress,omitempty"`
	Error         string                    `json:"error,omitempty"`
	LatestResult  *agenttask.Result         `json:"latest_result,omitempty"`
	WorkspacePath string                    `json:"workspace_path"`
	SessionID     string                    `json:"session_id,omitempty"`
	ResumeCommand string                    `json:"resume_command,omitempty"`
	FollowUp      agenttask.FollowUpPolicy  `json:"follow_up"`
	WakeCount     int                       `json:"wake_count"`
	ScheduledAt   *time.Time                `json:"scheduled_at,omitempty"`
	StartedAt     *time.Time                `json:"started_at,omitempty"`
	FinishedAt    *time.Time                `json:"finished_at,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Recurrence    *coretask.RecurrenceSpec  `json:"recurrence,omitempty"`
	Steps         []agenttask.Step          `json:"steps,omitempty"`
	CurrentStep   int                       `json:"current_step"`
	StepOutcomes  []agenttask.StepOutcome   `json:"step_outcomes,omitempty"`
	Repeat        *agenttask.RepeatPolicy   `json:"repeat,omitempty"`
	Execution     agenttask.ExecutionPolicy `json:"execution"`
	TriggerPaused bool                      `json:"trigger_paused"`
	ActiveRunID   string                    `json:"active_run_id,omitempty"`
}

type TaskResponse struct {
	Data TaskView `json:"data"`
}

type TaskListResponse struct {
	Data []TaskView `json:"data"`
}

type AgentListResponse struct {
	Data []AgentAvailability `json:"data"`
}

type RunView struct {
	ID             string                    `json:"id"`
	TaskID         string                    `json:"task_id"`
	Attempt        int                       `json:"attempt"`
	Status         coretask.RunStatus        `json:"status"`
	Trigger        string                    `json:"trigger"`
	StepID         string                    `json:"step_id,omitempty"`
	StepIndex      *int                      `json:"step_index,omitempty"`
	Instruction    string                    `json:"instruction,omitempty"`
	Execution      agenttask.ExecutionPolicy `json:"execution"`
	Progress       string                    `json:"progress,omitempty"`
	Result         *agenttask.Result         `json:"result,omitempty"`
	Error          string                    `json:"error,omitempty"`
	PendingControl *coretask.PendingControl  `json:"pending_control,omitempty"`
	Events         []coretask.RunEvent       `json:"events,omitempty"`
	StartedAt      time.Time                 `json:"started_at"`
	FinishedAt     *time.Time                `json:"finished_at,omitempty"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
}

// TaskUsageView aggregates gateway usage attributed to one task via the
// X-Tingly-Task-Id header its runs inject.
type TaskUsageView struct {
	TaskID           string `json:"task_id"`
	Requests         int64  `json:"requests"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheInputTokens int64  `json:"cache_input_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

type TaskUsageResponse struct {
	Data TaskUsageView `json:"data"`
}

type RunResponse struct {
	Data RunView `json:"data"`
}

type RunListResponse struct {
	Data []RunView `json:"data"`
}
