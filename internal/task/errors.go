package task

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound          = errors.New("task: not found")
	ErrNotCancellable    = errors.New("task: task is not in a cancellable state")
	ErrHandlerNotFound   = errors.New("task: no handler registered for type")
	ErrManagerStopped    = errors.New("task: manager is not running")
	ErrDuplicateHandler  = errors.New("task: handler already registered for type")
	ErrNotWakeable       = errors.New("task: task is already running or queued")
	ErrNotEditable       = errors.New("task: task is not editable while running or queued")
	ErrInvalidOutcome    = errors.New("task: invalid handler outcome")
	ErrInvalidRecurrence = errors.New("task: invalid recurrence")
)

// RetryableError wraps an error to signal that it is transient and the task
// should be rescheduled if attempts remain.
type RetryableError struct {
	Cause   error
	Backoff time.Duration // 0 = use default exponential backoff
}

func (e *RetryableError) Error() string { return fmt.Sprintf("retryable: %v", e.Cause) }
func (e *RetryableError) Unwrap() error { return e.Cause }

// IsRetryable reports whether err (possibly wrapped) is a RetryableError.
func IsRetryable(err error) bool {
	var r *RetryableError
	return errors.As(err, &r)
}

// BackoffFor returns the delay before the next attempt.
// Falls back to exponential backoff: 2^attempt * 5s, capped at 10 minutes.
func BackoffFor(err error, attempt int) time.Duration {
	var r *RetryableError
	if errors.As(err, &r) && r.Backoff > 0 {
		return r.Backoff
	}
	d := time.Duration(1<<uint(attempt)) * 5 * time.Second
	if d > 10*time.Minute {
		d = 10 * time.Minute
	}
	return d
}
