package taskapi

import (
	"time"

	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
)

type CreateRequest struct {
	Title          string                   `json:"title"`
	Goal           string                   `json:"goal" binding:"required"`
	Agent          agenttask.AgentKind      `json:"agent" binding:"required"`
	ScheduledAt    *time.Time               `json:"scheduled_at,omitempty"`
	Recurrence     *coretask.RecurrenceSpec `json:"recurrence,omitempty"`
	FollowUp       agenttask.FollowUpPolicy `json:"follow_up"`
	TimeoutSeconds int                      `json:"timeout_seconds,omitempty"`
	Steps          []CreateStep             `json:"steps,omitempty"`
}

type CreateStep struct {
	Instruction string `json:"instruction" binding:"required"`
}

type WakeRequest struct {
	Instruction string `json:"instruction,omitempty"`
}

type AgentAvailability struct {
	Agent     agenttask.AgentKind `json:"agent"`
	Available bool                `json:"available"`
}

type TaskView struct {
	ID            string                   `json:"id"`
	Title         string                   `json:"title"`
	Goal          string                   `json:"goal"`
	Agent         agenttask.AgentKind      `json:"agent"`
	Status        coretask.TaskStatus      `json:"status"`
	Progress      string                   `json:"progress,omitempty"`
	Error         string                   `json:"error,omitempty"`
	LatestResult  *agenttask.Result        `json:"latest_result,omitempty"`
	WorkspacePath string                   `json:"workspace_path"`
	SessionID     string                   `json:"session_id,omitempty"`
	ResumeCommand string                   `json:"resume_command,omitempty"`
	FollowUp      agenttask.FollowUpPolicy `json:"follow_up"`
	WakeCount     int                      `json:"wake_count"`
	ScheduledAt   *time.Time               `json:"scheduled_at,omitempty"`
	StartedAt     *time.Time               `json:"started_at,omitempty"`
	FinishedAt    *time.Time               `json:"finished_at,omitempty"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
	Recurrence    *coretask.RecurrenceSpec `json:"recurrence,omitempty"`
	Steps         []agenttask.Step         `json:"steps,omitempty"`
	CurrentStep   int                      `json:"current_step"`
	StepOutcomes  []agenttask.StepOutcome  `json:"step_outcomes,omitempty"`
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
