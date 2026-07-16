package task

import (
	"context"
	"encoding/json"
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

	// GetRun and ListRuns expose durable bounded execution history.
	GetRun(ctx context.Context, taskID, runID string) (*TaskRun, error)
	ListRuns(ctx context.Context, filter RunListFilter) ([]TaskRun, error)

	// Wait polls until taskID reaches a terminal status or ctx is cancelled.
	// Useful for callers (e.g. HTTP handlers) that need a synchronous result.
	Wait(ctx context.Context, taskID string) (*Task, error)

	// Wake schedules an existing non-running task for another invocation. A
	// zero time means immediately. Running and queued tasks return
	// ErrNotWakeable so a native session can never be resumed concurrently.
	Wake(ctx context.Context, taskID string, at time.Time) error

	// UpdatePayload checkpoints handler- or API-owned durable task state
	// without exposing the Store to higher layers.
	UpdatePayload(ctx context.Context, taskID string, payload json.RawMessage) error

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
	taskID := req.ID
	if taskID == "" {
		taskID = uuid.NewString()
	} else if _, err := uuid.Parse(taskID); err != nil {
		return nil, fmt.Errorf("task: invalid ID: %w", err)
	}
	scheduledAt := req.ScheduledAt
	if len(req.Recurrence) > 0 {
		next, err := NextOccurrence(req.Recurrence, now)
		if err != nil {
			return nil, err
		}
		if scheduledAt == nil {
			scheduledAt = &next
		}
	}
	t := &Task{
		ID:               taskID,
		Type:             req.Type,
		Status:           StatusPending,
		OwnerType:        req.OwnerType,
		OwnerID:          req.OwnerID,
		Source:           req.Source,
		SerializationKey: req.SerializationKey,
		Payload:          req.Payload,
		MaxAttempts:      maxAttempts,
		ScheduledAt:      scheduledAt,
		Recurrence:       req.Recurrence,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := m.store.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("task: submit: %w", err)
	}
	return t, nil
}

func (m *taskManager) Cancel(ctx context.Context, taskID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, err := m.store.Get(ctx, taskID)
	if err != nil {
		return err
	}
	now := time.Now()
	switch t.Status {
	case StatusRunning:
		m.registry.cancel(taskID)
		// The runner goroutine detects ctx cancellation and writes status=cancelled.
	case StatusPending, StatusQueued, StatusNeedsInput, StatusHandoff:
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

func (m *taskManager) GetRun(ctx context.Context, taskID, runID string) (*TaskRun, error) {
	return m.store.GetRun(ctx, taskID, runID)
}

func (m *taskManager) ListRuns(ctx context.Context, filter RunListFilter) ([]TaskRun, error) {
	return m.store.ListRuns(ctx, filter)
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

func (m *taskManager) Wake(ctx context.Context, taskID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, err := m.store.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if t.Status == StatusRunning || t.Status == StatusQueued {
		return ErrNotWakeable
	}
	if at.IsZero() {
		at = time.Now()
	}
	return m.store.UpdateStatus(ctx, taskID, map[string]interface{}{
		"status":       string(StatusPending),
		"scheduled_at": at,
		"started_at":   nil,
		"finished_at":  nil,
		"cancelled_at": nil,
		"attempt":      0,
		"error":        "",
	})
}

func (m *taskManager) UpdatePayload(ctx context.Context, taskID string, payload json.RawMessage) error {
	if _, err := m.store.Get(ctx, taskID); err != nil {
		return err
	}
	return m.store.UpdateStatus(ctx, taskID, map[string]interface{}{"payload": string(payload)})
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
	run := &TaskRun{
		ID:        uuid.NewString(),
		TaskID:    t.ID,
		Attempt:   t.Attempt,
		Status:    RunStatusRunning,
		Input:     append(json.RawMessage(nil), t.Payload...),
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := m.store.CreateRun(ctx, run); err != nil {
		_ = m.store.UpdateStatus(ctx, t.ID, map[string]interface{}{
			"status":      string(StatusFailed),
			"finished_at": now,
			"error":       fmt.Sprintf("task: create run: %v", err),
		})
		m.registry.unregister(t.ID, t.SerializationKey)
		cancel()
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	go m.runTask(taskCtx, cancel, t, run)
}

// runTask executes one handler invocation and writes its next durable state.
func (m *taskManager) runTask(ctx context.Context, cancel context.CancelFunc, t *Task, run *TaskRun) {
	runStatus := RunStatusFailed
	var runResult json.RawMessage
	var runError string
	runFinalized := false
	finishRun := func() {
		if runFinalized {
			return
		}
		now := time.Now()
		fields := map[string]interface{}{
			"status":          string(runStatus),
			"finished_at":     now,
			"error":           runError,
			"pending_control": "",
		}
		if len(runResult) > 0 {
			fields["result"] = string(runResult)
		}
		_ = m.store.UpdateRun(context.Background(), run.ID, fields)
		runFinalized = true
	}
	defer func() {
		finishRun()
		cancel()
		m.registry.unregister(t.ID, t.SerializationKey)
		m.onTaskFinished(t)
	}()

	handler, ok := m.handlers.get(t.Type)
	if !ok {
		runError = ErrHandlerNotFound.Error()
		finishRun()
		_ = m.store.UpdateStatus(ctx, t.ID, map[string]interface{}{
			"status":      string(StatusFailed),
			"error":       ErrHandlerNotFound.Error(),
			"finished_at": time.Now(),
		})
		return
	}

	ctl := &taskController{store: m.store, taskID: t.ID, runID: run.ID}
	result, err := handler.Run(ctx, t, ctl)

	now := time.Now()
	if err != nil {
		runError = err.Error()
		if ctx.Err() != nil {
			runStatus = RunStatusCancelled
			finishRun()
			_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
				"status":       string(StatusCancelled),
				"cancelled_at": now,
				"finished_at":  now,
				"error":        err.Error(),
			})
		} else if IsRetryable(err) && t.Attempt < t.MaxAttempts {
			finishRun()
			backoff := BackoffFor(err, t.Attempt)
			next := now.Add(backoff)
			_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
				"status":       string(StatusPending),
				"scheduled_at": next,
				"error":        err.Error(),
			})
		} else {
			finishRun()
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
		runResult = append(json.RawMessage(nil), result.Result...)
	}
	outcome := OutcomeComplete
	if result != nil && result.Outcome != "" {
		outcome = result.Outcome
	}

	switch outcome {
	case OutcomeComplete:
		runStatus = RunStatusSucceeded
		if len(t.Recurrence) > 0 {
			next, err := NextOccurrence(t.Recurrence, now)
			if err != nil {
				runStatus = RunStatusFailed
				runError = err.Error()
				finishRun()
				_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
					"status":      string(StatusFailed),
					"finished_at": now,
					"error":       err.Error(),
					"result":      resultJSON,
				})
				return
			}
			finishRun()
			_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
				"status":       string(StatusPending),
				"finished_at":  nil,
				"scheduled_at": next,
				"attempt":      0,
				"result":       resultJSON,
				"error":        "",
			})
			return
		}
		finishRun()
		_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
			"status":       string(StatusSucceeded),
			"finished_at":  now,
			"scheduled_at": nil,
			"result":       resultJSON,
			"error":        "",
		})
	case OutcomeReschedule:
		runStatus = RunStatusRescheduled
		if result == nil || result.NextRunAt == nil {
			runStatus = RunStatusFailed
			runError = ErrInvalidOutcome.Error() + ": reschedule requires NextRunAt"
			finishRun()
			_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
				"status":      string(StatusFailed),
				"finished_at": now,
				"error":       ErrInvalidOutcome.Error() + ": reschedule requires NextRunAt",
				"result":      resultJSON,
			})
			return
		}
		finishRun()
		_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
			"status":       string(StatusPending),
			"scheduled_at": *result.NextRunAt,
			"finished_at":  nil,
			"attempt":      0,
			"result":       resultJSON,
			"error":        "",
		})
	case OutcomeNeedsInput:
		runStatus = RunStatusNeedsInput
		finishRun()
		_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
			"status":       string(StatusNeedsInput),
			"scheduled_at": nil,
			"finished_at":  nil,
			"attempt":      0,
			"result":       resultJSON,
			"error":        "",
		})
	case OutcomeHandoff:
		runStatus = RunStatusHandoff
		finishRun()
		_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
			"status":       string(StatusHandoff),
			"scheduled_at": nil,
			"finished_at":  nil,
			"attempt":      0,
			"result":       resultJSON,
			"error":        "",
		})
	default:
		runStatus = RunStatusFailed
		runError = fmt.Sprintf("%s: %q", ErrInvalidOutcome, outcome)
		finishRun()
		_ = m.store.UpdateStatus(context.Background(), t.ID, map[string]interface{}{
			"status":      string(StatusFailed),
			"finished_at": now,
			"error":       fmt.Sprintf("%s: %q", ErrInvalidOutcome, outcome),
			"result":      resultJSON,
		})
	}
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
