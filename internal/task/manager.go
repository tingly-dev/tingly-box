package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Manager is the public interface for the task management system.
type Manager interface {
	// Register adds a handler for a task type. May be called before Start.
	Register(handler Handler) error

	// Submit creates a new task and persists it. The scheduler picks it up
	// within one poll interval (5s by default). Submit is durable: if the
	// process crashes after Submit returns, the task survives in the DB.
	Submit(ctx context.Context, req SubmitRequest) (*Task, error)

	// Cancel transitions a task to cancelled. For running tasks it signals
	// the handler's context; the handler is responsible for returning promptly.
	Cancel(ctx context.Context, taskID string, reason string) error

	// Get returns the current state of a task. Returns ErrNotFound if absent.
	Get(ctx context.Context, taskID string) (*Task, error)

	// List returns tasks matching the filter, ordered by created_at DESC.
	List(ctx context.Context, filter ListFilter) ([]Task, error)

	// Wait polls until taskID reaches a terminal status or ctx is cancelled.
	// Useful for callers (e.g. HTTP handlers) that need a synchronous result.
	Wait(ctx context.Context, taskID string) (*Task, error)

	// Start runs restart recovery and launches the scheduler goroutine.
	Start(ctx context.Context) error

	// Stop signals the scheduler to stop. Running tasks are not interrupted.
	Stop(ctx context.Context) error
}

// ManagerOption configures a Manager at construction time.
type ManagerOption func(*taskManager)

// WithPollInterval overrides the scheduler polling interval.
// Useful in tests; default is 5s.
func WithPollInterval(d time.Duration) ManagerOption {
	return func(m *taskManager) { m.sched.poll = d }
}

// WithWaitInterval overrides the Wait() polling interval.
// Useful in tests; default is 50ms.
func WithWaitInterval(d time.Duration) ManagerOption {
	return func(m *taskManager) { m.waitInterval = d }
}

// taskManager is the concrete implementation of Manager.
type taskManager struct {
	mu           sync.Mutex
	store        Store
	handlers     *handlerRegistry
	registry     *cancelRegistry
	queue        *serialKeyQueue
	sched        *scheduler
	stopFn       context.CancelFunc
	running      bool
	waitInterval time.Duration
}

// NewManager constructs a Manager backed by the provided Store.
func NewManager(store Store, opts ...ManagerOption) Manager {
	m := &taskManager{
		store:        store,
		handlers:     newHandlerRegistry(),
		registry:     newCancelRegistry(),
		queue:        newSerialKeyQueue(),
		waitInterval: schedulerDefaultWaitPoll,
	}
	m.sched = newScheduler(store, m.dispatchTask)
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *taskManager) Register(handler Handler) error {
	return m.handlers.register(handler)
}

func (m *taskManager) Submit(ctx context.Context, req SubmitRequest) (*Task, error) {
	if req.Type == "" {
		return nil, fmt.Errorf("task: Type is required")
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	now := time.Now()
	t := &Task{
		ID:               uuid.New().String(),
		Type:             req.Type,
		Status:           StatusPending,
		OwnerType:        req.OwnerType,
		OwnerID:          req.OwnerID,
		Source:           req.Source,
		SerializationKey: req.SerializationKey,
		Payload:          req.Payload,
		MaxAttempts:      maxAttempts,
		ScheduledAt:      req.ScheduledAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := m.store.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("task: submit: %w", err)
	}
	return t, nil
}

func (m *taskManager) Cancel(ctx context.Context, taskID string, reason string) error {
	t, err := m.store.Get(ctx, taskID)
	if err != nil {
		return err
	}
	now := time.Now()
	switch t.Status {
	case StatusRunning:
		m.registry.cancel(taskID)
		// The runner goroutine detects ctx cancellation and writes status=cancelled.
	case StatusPending, StatusQueued:
		if err := m.store.UpdateStatus(ctx, taskID, map[string]interface{}{
			"status":       string(StatusCancelled),
			"cancelled_at": now,
			"finished_at":  now,
			"error":        reason,
		}); err != nil {
			return err
		}
		if t.Status == StatusQueued && t.SerializationKey != "" {
			m.queue.remove(t.SerializationKey, taskID)
		}
	default:
		return ErrNotCancellable
	}
	return nil
}

func (m *taskManager) Get(ctx context.Context, taskID string) (*Task, error) {
	return m.store.Get(ctx, taskID)
}

func (m *taskManager) List(ctx context.Context, filter ListFilter) ([]Task, error) {
	return m.store.List(ctx, filter)
}

func (m *taskManager) Wait(ctx context.Context, taskID string) (*Task, error) {
	for {
		t, err := m.store.Get(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if t.Status.IsTerminal() {
			return t, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.waitInterval):
		}
	}
}

func (m *taskManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	if err := m.store.MarkInterruptedOnStartup(ctx); err != nil {
		return fmt.Errorf("task: start: restart recovery: %w", err)
	}

	schedCtx, stopFn := context.WithCancel(ctx)
	m.mu.Lock()
	m.stopFn = stopFn
	m.running = true
	m.mu.Unlock()

	go m.sched.run(schedCtx)
	return nil
}

func (m *taskManager) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil
	}
	m.stopFn()
	m.running = false
	return nil
}

// dispatchTask is called by the scheduler for each due pending task.
// It either launches the task immediately (key is free) or marks it queued.
func (m *taskManager) dispatchTask(ctx context.Context, t *Task) {
	m.mu.Lock()

	// If the serialization key is held, mark queued and enqueue.
	if t.SerializationKey != "" && m.registry.isLocked(t.SerializationKey) {
		if err := m.store.UpdateStatus(ctx, t.ID, map[string]interface{}{
			"status": string(StatusQueued),
		}); err != nil {
			logrus.WithError(err).WithField("taskID", t.ID).Warn("task: failed to mark queued")
		} else {
			m.queue.enqueue(t.SerializationKey, t.ID)
		}
		m.mu.Unlock()
		return
	}

	// Key is free (or task has no key): launch the task.
	taskCtx, cancel := context.WithCancel(ctx)
	if !m.registry.register(t.ID, t.SerializationKey, cancel) {
		// Concurrent registration race — should not normally happen.
		cancel()
		m.mu.Unlock()
		return
	}
	now := time.Now()
	if err := m.store.UpdateStatus(ctx, t.ID, map[string]interface{}{
		"status":     string(StatusRunning),
		"started_at": now,
		"attempt":    t.Attempt + 1,
	}); err != nil {
		m.registry.unregister(t.ID, t.SerializationKey)
		cancel()
		m.mu.Unlock()
		logrus.WithError(err).WithField("taskID", t.ID).Warn("task: failed to mark running")
		return
	}
	t.Attempt++
	t.Status = StatusRunning
	t.StartedAt = &now
	m.mu.Unlock()

	go m.runTask(taskCtx, cancel, t)
}

// runTask executes the handler and writes the terminal status to the DB.
func (m *taskManager) runTask(ctx context.Context, cancel context.CancelFunc, t *Task) {
	defer func() {
		cancel()
		m.registry.unregister(t.ID, t.SerializationKey)
		m.onTaskFinished(t)
	}()

	handler, ok := m.handlers.get(t.Type)
	if !ok {
		_ = m.store.UpdateStatus(ctx, t.ID, map[string]interface{}{
			"status":      string(StatusFailed),
			"error":       ErrHandlerNotFound.Error(),
			"finished_at": time.Now(),
		})
		return
	}

	ctl := &taskController{store: m.store, taskID: t.ID}
	result, err := handler.Run(ctx, t, ctl)

	now := time.Now()
	if err != nil {
		if ctx.Err() != nil {
			_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
				"status":       string(StatusCancelled),
				"cancelled_at": now,
				"finished_at":  now,
				"error":        err.Error(),
			})
		} else if IsRetryable(err) && t.Attempt < t.MaxAttempts {
			backoff := BackoffFor(err, t.Attempt)
			next := now.Add(backoff)
			_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
				"status":       string(StatusPending),
				"scheduled_at": next,
				"error":        err.Error(),
			})
		} else {
			_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
				"status":      string(StatusFailed),
				"finished_at": now,
				"error":       err.Error(),
			})
		}
		return
	}

	resultJSON := ""
	if result != nil && len(result.Result) > 0 {
		resultJSON = string(result.Result)
	}
	_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
		"status":      string(StatusSucceeded),
		"finished_at": now,
		"result":      resultJSON,
	})
}

// onTaskFinished is called after a task's goroutine exits. It wakes the next
// queued task for the same serialization key, if any.
func (m *taskManager) onTaskFinished(t *Task) {
	if t.SerializationKey == "" {
		return
	}
	nextID := m.queue.dequeue(t.SerializationKey)
	if nextID == "" {
		return
	}
	next, err := m.store.Get(context.Background(), nextID)
	if err != nil || next.Status != StatusQueued {
		return
	}
	m.dispatchTask(context.Background(), next)
}
