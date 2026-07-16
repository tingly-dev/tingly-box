package task

import (
	"encoding/json"
	"time"
)

// RunStatus describes one bounded handler invocation. Task status remains the
// long-lived scheduling projection; Run status is the durable execution record.
type RunStatus string

const (
	RunStatusRunning     RunStatus = "running"
	RunStatusSucceeded   RunStatus = "succeeded"
	RunStatusRescheduled RunStatus = "rescheduled"
	RunStatusNeedsInput  RunStatus = "needs_input"
	RunStatusFailed      RunStatus = "failed"
	RunStatusCancelled   RunStatus = "cancelled"
	RunStatusInterrupted RunStatus = "interrupted"
)

func (s RunStatus) IsActive() bool { return s == RunStatusRunning }

// TaskRun is one durable invocation of a Task handler.
type TaskRun struct {
	ID      string
	TaskID  string
	Attempt int
	Status  RunStatus

	Input    json.RawMessage
	Result   json.RawMessage
	Progress string
	Error    string

	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type RunListFilter struct {
	TaskID string
	Limit  int
	Offset int
}
