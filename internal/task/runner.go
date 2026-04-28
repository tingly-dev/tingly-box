package task

import (
	"context"
	"sync"
)

// Handler is implemented by each task type and registered with the Manager.
type Handler interface {
	// Type returns the task type string this handler handles.
	Type() string
	// Run executes the task. Returning a non-nil error causes the task to be
	// failed or retried depending on whether the error is retryable and
	// whether attempts remain. A context.Canceled error marks the task cancelled.
	Run(ctx context.Context, t *Task, ctl Controller) (*TaskResult, error)
}

// Controller is passed to Handler.Run so handlers can report progress and
// check cancellation without importing the full manager.
type Controller interface {
	// UpdateProgress stores a human-readable progress string on the task.
	UpdateProgress(ctx context.Context, text string) error
	// IsCancelled reports whether the task's context has been cancelled.
	IsCancelled(ctx context.Context) bool
}

// handlerRegistry stores handlers keyed by task type.
type handlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func newHandlerRegistry() *handlerRegistry {
	return &handlerRegistry{handlers: make(map[string]Handler)}
}

func (r *handlerRegistry) register(h Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[h.Type()]; exists {
		return ErrDuplicateHandler
	}
	r.handlers[h.Type()] = h
	return nil
}

func (r *handlerRegistry) get(typ string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[typ]
	return h, ok
}

// taskController implements Controller for a single running task.
type taskController struct {
	store  Store
	taskID string
}

func (c *taskController) UpdateProgress(ctx context.Context, text string) error {
	return c.store.UpdateStatus(ctx, c.taskID, map[string]interface{}{"progress": text})
}

func (c *taskController) IsCancelled(ctx context.Context) bool {
	return ctx.Err() != nil
}
