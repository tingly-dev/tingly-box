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
	StatusNeedsInput  TaskStatus = "needs_input"
	StatusHandoff     TaskStatus = "handoff_required"
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

	// Recurrence holds a RecurrenceSpec. A recurring task reuses this record
	// and materializes its next occurrence in ScheduledAt.
	Recurrence json.RawMessage
	// ParentTaskID is retained for storage compatibility with the original
	// task prototype. The current recurrence model does not create child tasks.
	ParentTaskID string

	// TriggerPaused disables scheduling without changing Status. A paused
	// task keeps its schedule and history but the scheduler skips it until
	// resumed. This is the trigger's own enabled/paused axis, orthogonal to
	// the run-status projection (design §6.2 / §7.3).
	TriggerPaused bool
}

// OutcomeKind tells the manager how a successful handler invocation should
// advance the task. The zero value is treated as OutcomeComplete for backward
// compatibility with existing handlers.
type OutcomeKind string

const (
	OutcomeComplete   OutcomeKind = "complete"
	OutcomeReschedule OutcomeKind = "reschedule"
	OutcomeNeedsInput OutcomeKind = "needs_input"
	OutcomeHandoff    OutcomeKind = "handoff_required"
)

// TaskResult is returned by Handler.Run on success. A handler can complete
// the task, reschedule the same task for another bounded invocation, or pause
// it until an external caller supplies input and wakes it explicitly.
type TaskResult struct {
	Outcome   OutcomeKind
	Result    json.RawMessage
	NextRunAt *time.Time
}

// SubmitRequest describes the parameters for creating a new task.
type SubmitRequest struct {
	// ID may be supplied by a trusted service that must derive durable
	// resources (such as a workspace) before the task becomes schedulable.
	// Empty lets Manager generate the UUID as usual.
	ID               string
	Type             string
	OwnerType        string
	OwnerID          string
	Source           string
	SerializationKey string
	Payload          json.RawMessage
	MaxAttempts      int        // defaults to 1 if zero or negative
	ScheduledAt      *time.Time // nil = run as soon as possible
	Recurrence       json.RawMessage
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
