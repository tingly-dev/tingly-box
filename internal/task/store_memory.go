package task

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// MemoryStore is a thread-safe in-memory implementation of Store.
// Intended for tests; do not use in production.
type MemoryStore struct {
	mu    sync.Mutex
	tasks map[string]*Task
	runs  map[string]*TaskRun
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[string]*Task), runs: make(map[string]*TaskRun)}
}

func (s *MemoryStore) Create(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *MemoryStore) Get(_ context.Context, taskID string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[taskID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (s *MemoryStore) Update(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return ErrNotFound
	}
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *MemoryStore) List(_ context.Context, filter ListFilter) ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	statusSet := make(map[TaskStatus]bool, len(filter.Status))
	for _, st := range filter.Status {
		statusSet[st] = true
	}

	var result []Task
	for _, t := range s.tasks {
		if filter.OwnerType != "" && t.OwnerType != filter.OwnerType {
			continue
		}
		if filter.OwnerID != "" && t.OwnerID != filter.OwnerID {
			continue
		}
		if filter.Type != "" && t.Type != filter.Type {
			continue
		}
		if len(statusSet) > 0 && !statusSet[t.Status] {
			continue
		}
		cp := *t
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return nil, nil
		}
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (s *MemoryStore) MarkInterruptedOnStartup(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, t := range s.tasks {
		switch t.Status {
		case StatusRunning:
			t.Status = StatusInterrupted
			t.UpdatedAt = now
		case StatusQueued:
			t.Status = StatusPending
			t.ScheduledAt = nil
			t.UpdatedAt = now
		}
	}
	for _, run := range s.runs {
		if run.Status.IsActive() {
			run.Status = RunStatusInterrupted
			run.FinishedAt = &now
			run.PendingControl = nil
			run.UpdatedAt = now
		}
	}
	return nil
}

func (s *MemoryStore) FindDueTasks(_ context.Context, now time.Time, limit int) ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []Task
	for _, t := range s.tasks {
		if t.Status != StatusPending {
			continue
		}
		if t.ScheduledAt != nil && t.ScheduledAt.After(now) {
			continue
		}
		cp := *t
		result = append(result, cp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) FindQueuedForKey(_ context.Context, key string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldest *Task
	for _, t := range s.tasks {
		if t.SerializationKey != key || t.Status != StatusQueued {
			continue
		}
		if oldest == nil || t.CreatedAt.Before(oldest.CreatedAt) {
			cp := *t
			oldest = &cp
		}
	}
	return oldest, nil
}

func (s *MemoryStore) UpdateStatus(_ context.Context, taskID string, fields map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[taskID]
	if !ok {
		return ErrNotFound
	}
	for k, v := range fields {
		switch k {
		case "status":
			t.Status = TaskStatus(v.(string))
		case "attempt":
			t.Attempt = v.(int)
		case "progress":
			t.Progress = v.(string)
		case "error":
			t.Error = v.(string)
		case "result":
			if sv, ok := v.(string); ok {
				if sv == "" {
					t.Result = nil
				} else {
					t.Result = json.RawMessage(sv)
				}
			}
		case "recurrence":
			if sv, ok := v.(string); ok {
				if sv == "" {
					t.Recurrence = nil
				} else {
					t.Recurrence = json.RawMessage(sv)
				}
			}
		case "payload":
			if sv, ok := v.(string); ok {
				if sv == "" {
					t.Payload = nil
				} else {
					t.Payload = json.RawMessage(sv)
				}
			}
		case "started_at":
			if v == nil {
				t.StartedAt = nil
			} else if ts, ok := v.(time.Time); ok {
				t.StartedAt = &ts
			}
		case "finished_at":
			if v == nil {
				t.FinishedAt = nil
			} else if ts, ok := v.(time.Time); ok {
				t.FinishedAt = &ts
			}
		case "cancelled_at":
			if v == nil {
				t.CancelledAt = nil
			} else if ts, ok := v.(time.Time); ok {
				t.CancelledAt = &ts
			}
		case "scheduled_at":
			if v == nil {
				t.ScheduledAt = nil
			} else if ts, ok := v.(time.Time); ok {
				t.ScheduledAt = &ts
			} else if ts, ok := v.(*time.Time); ok {
				t.ScheduledAt = ts
			}
		}
	}
	t.UpdatedAt = time.Now()
	return nil
}

func (s *MemoryStore) AppendRunEvent(_ context.Context, runID string, event RunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return ErrNotFound
	}
	run.Events = append(run.Events, event)
	if len(run.Events) > 200 {
		run.Events = append([]RunEvent(nil), run.Events[len(run.Events)-200:]...)
	}
	run.UpdatedAt = time.Now()
	return nil
}

func (s *MemoryStore) CreateRun(_ context.Context, run *TaskRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := cloneRun(run)
	s.runs[run.ID] = cp
	return nil
}

func (s *MemoryStore) GetRun(_ context.Context, taskID, runID string) (*TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.TaskID != taskID {
		return nil, ErrNotFound
	}
	return cloneRun(run), nil
}

func (s *MemoryStore) ListRuns(_ context.Context, filter RunListFilter) ([]TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	statuses := make(map[RunStatus]bool, len(filter.Status))
	for _, status := range filter.Status {
		statuses[status] = true
	}
	result := make([]TaskRun, 0)
	for _, run := range s.runs {
		if filter.TaskID != "" && run.TaskID != filter.TaskID {
			continue
		}
		if len(statuses) > 0 && !statuses[run.Status] {
			continue
		}
		result = append(result, *cloneRun(run))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return nil, nil
		}
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (s *MemoryStore) UpdateRun(_ context.Context, runID string, fields map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return ErrNotFound
	}
	for key, value := range fields {
		switch key {
		case "status":
			run.Status = RunStatus(value.(string))
		case "progress":
			run.Progress = value.(string)
		case "error":
			run.Error = value.(string)
		case "result":
			if text, ok := value.(string); ok {
				run.Result = json.RawMessage(text)
			}
		case "finished_at":
			if value == nil {
				run.FinishedAt = nil
			} else if ts, ok := value.(time.Time); ok {
				run.FinishedAt = &ts
			}
		case "pending_control":
			text, _ := value.(string)
			if text == "" {
				run.PendingControl = nil
			} else {
				var control PendingControl
				if err := json.Unmarshal([]byte(text), &control); err != nil {
					return err
				}
				run.PendingControl = &control
			}
		}
	}
	run.UpdatedAt = time.Now()
	return nil
}

func cloneRun(run *TaskRun) *TaskRun {
	cp := *run
	cp.Input = append(json.RawMessage(nil), run.Input...)
	cp.Result = append(json.RawMessage(nil), run.Result...)
	if run.PendingControl != nil {
		control := *run.PendingControl
		control.Input = append(json.RawMessage(nil), run.PendingControl.Input...)
		cp.PendingControl = &control
	}
	cp.Events = append([]RunEvent(nil), run.Events...)
	return &cp
}
