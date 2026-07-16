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
	ScheduledAt    *time.Time                 `json:"scheduled_at,omitempty"`
	Recurrence     *coretask.RecurrenceSpec   `json:"recurrence,omitempty"`
	FollowUp       agenttask.FollowUpPolicy   `json:"follow_up"`
	TimeoutSeconds int                        `json:"timeout_seconds,omitempty"`
	Steps          []CreateStep               `json:"steps,omitempty"`
	Execution      *agenttask.ExecutionPolicy `json:"execution,omitempty"`
}

type CreateStep struct {
	Instruction string `json:"instruction" binding:"required"`
}

type WakeRequest struct {
	Instruction       string                     `json:"instruction,omitempty"`
	ExecutionOverride *agenttask.ExecutionPolicy `json:"execution_override,omitempty"`
}

type AgentAvailability struct {
	Agent              agenttask.AgentKind       `json:"agent"`
	Available          bool                      `json:"available"`
	LaunchProfiles     []agenttask.LaunchProfile `json:"launch_profiles"`
	DefaultProfile     agenttask.LaunchProfile   `json:"default_profile"`
	ToolFiltering      bool                      `json:"tool_filtering"`
	InteractiveControl bool                      `json:"interactive_control"`
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
	Execution     agenttask.ExecutionPolicy `json:"execution"`
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
	ID          string                    `json:"id"`
	TaskID      string                    `json:"task_id"`
	Attempt     int                       `json:"attempt"`
	Status      coretask.RunStatus        `json:"status"`
	Trigger     string                    `json:"trigger"`
	StepID      string                    `json:"step_id,omitempty"`
	StepIndex   *int                      `json:"step_index,omitempty"`
	Instruction string                    `json:"instruction,omitempty"`
	Execution   agenttask.ExecutionPolicy `json:"execution"`
	Progress    string                    `json:"progress,omitempty"`
	Result      *agenttask.Result         `json:"result,omitempty"`
	Error       string                    `json:"error,omitempty"`
	StartedAt   time.Time                 `json:"started_at"`
	FinishedAt  *time.Time                `json:"finished_at,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}

type RunResponse struct {
	Data RunView `json:"data"`
}

type RunListResponse struct {
	Data []RunView `json:"data"`
}
