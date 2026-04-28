package task

import (
	"encoding/json"
	"time"
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	StatusPending     TaskStatus = "pending"
	StatusQueued      TaskStatus = "queued"
	StatusRunning     TaskStatus = "running"
	StatusSucceeded   TaskStatus = "succeeded"
	StatusFailed      TaskStatus = "failed"
	StatusCancelled   TaskStatus = "cancelled"
	StatusInterrupted TaskStatus = "interrupted"
)

// IsTerminal reports whether the status is a terminal (non-actionable) state.
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusInterrupted:
		return true
	}
	return false
}

// Task is the domain representation of a unit of persistent work.
type Task struct {
	ID               string
	Type             string
	Status           TaskStatus
	OwnerType        string
	OwnerID          string
	Source           string
	SerializationKey string

	Payload json.RawMessage
	Result  json.RawMessage

	Progress string
	Error    string

	Attempt     int
	MaxAttempts int

	ScheduledAt *time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	CancelledAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Recurrence holds cron spec JSON. Reserved for Phase 4; nil in Phase 1.
	Recurrence json.RawMessage
	// ParentTaskID links a recurring child instance to its recurrence parent.
	ParentTaskID string
}

// TaskResult is returned by Handler.Run on success.
type TaskResult struct {
	Result json.RawMessage
}

// SubmitRequest describes the parameters for creating a new task.
type SubmitRequest struct {
	Type             string
	OwnerType        string
	OwnerID          string
	Source           string
	SerializationKey string
	Payload          json.RawMessage
	MaxAttempts      int        // defaults to 1 if zero or negative
	ScheduledAt      *time.Time // nil = run as soon as possible
}

// ListFilter restricts the result set of Manager.List.
type ListFilter struct {
	OwnerType string
	OwnerID   string
	Type      string
	Status    []TaskStatus
	Limit     int
	Offset    int
}
