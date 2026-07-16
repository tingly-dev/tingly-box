package task

import (
	"encoding/json"
	"time"
)

// RunStatus describes one bounded handler invocation. Task status remains the
// long-lived scheduling projection; Run status is the durable execution record.
type RunStatus string

const (
	RunStatusRunning         RunStatus = "running"
	RunStatusWaitingApproval RunStatus = "waiting_approval"
	RunStatusWaitingInput    RunStatus = "waiting_input"
	RunStatusSucceeded       RunStatus = "succeeded"
	RunStatusRescheduled     RunStatus = "rescheduled"
	RunStatusNeedsInput      RunStatus = "needs_input"
	RunStatusFailed          RunStatus = "failed"
	RunStatusCancelled       RunStatus = "cancelled"
	RunStatusInterrupted     RunStatus = "interrupted"
)

func (s RunStatus) IsActive() bool {
	return s == RunStatusRunning || s == RunStatusWaitingApproval || s == RunStatusWaitingInput
}

type ControlKind string

const (
	ControlKindApproval ControlKind = "approval"
	ControlKindQuestion ControlKind = "question"
)

type PendingControl struct {
	ID        string          `json:"id"`
	Kind      ControlKind     `json:"kind"`
	ToolName  string          `json:"tool_name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Message   string          `json:"message,omitempty"`
	Reason    string          `json:"reason,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt time.Time       `json:"expires_at"`
}

type RunEvent struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Summary   string          `json:"summary"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// TaskRun is one durable invocation of a Task handler.
type TaskRun struct {
	ID      string
	TaskID  string
	Attempt int
	Status  RunStatus

	Input          json.RawMessage
	Result         json.RawMessage
	Progress       string
	Error          string
	PendingControl *PendingControl
	Events         []RunEvent

	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type RunListFilter struct {
	TaskID string
	Status []RunStatus
	Limit  int
	Offset int
}
