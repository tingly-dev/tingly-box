package task

import (
	"context"
	"time"
)

// Store is the persistence interface for the task manager.
// The concrete implementation lives in internal/data/db.
type Store interface {
	// Create persists a new task. The task must have a non-empty ID.
	Create(ctx context.Context, t *Task) error

	// Get retrieves a task by its ID. Returns ErrNotFound if absent.
	Get(ctx context.Context, taskID string) (*Task, error)

	// Update overwrites all fields of an existing task record.
	Update(ctx context.Context, t *Task) error

	// List returns tasks matching filter, ordered by created_at DESC.
	List(ctx context.Context, filter ListFilter) ([]Task, error)

	// MarkInterruptedOnStartup transitions stale in-progress rows atomically:
	//   running → interrupted
	//   queued  → pending (so the scheduler rebuilds order from DB)
	// Called once during Manager.Start before the scheduler loop begins.
	MarkInterruptedOnStartup(ctx context.Context) error

	// FindDueTasks returns up to limit pending tasks whose scheduled_at is
	// NULL or <= now, ordered by created_at ASC (oldest first).
	FindDueTasks(ctx context.Context, now time.Time, limit int) ([]Task, error)

	// FindQueuedForKey returns the oldest queued task for a serialization key,
	// or nil if none exist.
	FindQueuedForKey(ctx context.Context, key string) (*Task, error)

	// UpdateStatus applies a partial update to the named task using the
	// provided column→value map. Only the listed columns are written;
	// other columns are left unchanged. This prevents concurrent goroutines
	// from stomping each other's fields.
	UpdateStatus(ctx context.Context, taskID string, fields map[string]interface{}) error
}
