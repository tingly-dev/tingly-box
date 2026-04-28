package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/task"
)

// TaskRecord is the GORM model for the tasks table.
type TaskRecord struct {
	ID               uint       `gorm:"primaryKey;autoIncrement;column:id"`
	TaskID           string     `gorm:"uniqueIndex;column:task_id;not null;size:64"`
	Type             string     `gorm:"column:type;not null;size:128"`
	Status           string     `gorm:"column:status;not null;size:32;index:idx_tasks_status_scheduled"`
	OwnerType        string     `gorm:"column:owner_type;size:64;index:idx_tasks_owner"`
	OwnerID          string     `gorm:"column:owner_id;size:64;index:idx_tasks_owner"`
	Source           string     `gorm:"column:source;size:64"`
	SerializationKey string     `gorm:"column:serialization_key;size:256;index:idx_tasks_key_status"`
	Payload          string     `gorm:"column:payload;type:text"`
	Result           string     `gorm:"column:result;type:text"`
	Progress         string     `gorm:"column:progress;size:512"`
	Error            string     `gorm:"column:error;type:text"`
	Attempt          int        `gorm:"column:attempt;default:0"`
	MaxAttempts      int        `gorm:"column:max_attempts;default:1"`
	ScheduledAt      *time.Time `gorm:"column:scheduled_at;index:idx_tasks_status_scheduled"`
	StartedAt        *time.Time `gorm:"column:started_at"`
	FinishedAt       *time.Time `gorm:"column:finished_at"`
	CancelledAt      *time.Time `gorm:"column:cancelled_at"`
	// Recurrence and ParentTaskID are reserved for Phase 4 (recurring tasks).
	Recurrence   string `gorm:"column:recurrence;type:text"`
	ParentTaskID string `gorm:"column:parent_task_id;size:64"`
	CreatedAt    time.Time `gorm:"column:created_at;index:idx_tasks_owner;index:idx_tasks_key_status"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (TaskRecord) TableName() string { return "tasks" }

// TaskStore persists and retrieves tasks using a shared GORM DB.
type TaskStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// compile-time interface check
var _ task.Store = (*TaskStore)(nil)

func (s *TaskStore) Create(ctx context.Context, t *task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := recordFromTask(t)
	return s.db.WithContext(ctx).Create(r).Error
}

func (s *TaskStore) Get(ctx context.Context, taskID string) (*task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r TaskRecord
	err := s.db.WithContext(ctx).Where("task_id = ?", taskID).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, task.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return taskFromRecord(&r), nil
}

func (s *TaskStore) Update(ctx context.Context, t *task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := recordFromTask(t)
	return s.db.WithContext(ctx).Where("task_id = ?", t.ID).Save(r).Error
}

func (s *TaskStore) List(ctx context.Context, filter task.ListFilter) ([]task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.db.WithContext(ctx).Model(&TaskRecord{})
	if filter.OwnerType != "" {
		q = q.Where("owner_type = ?", filter.OwnerType)
	}
	if filter.OwnerID != "" {
		q = q.Where("owner_id = ?", filter.OwnerID)
	}
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	if len(filter.Status) > 0 {
		statuses := make([]string, len(filter.Status))
		for i, st := range filter.Status {
			statuses[i] = string(st)
		}
		q = q.Where("status IN ?", statuses)
	}
	q = q.Order("created_at DESC")
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	var records []TaskRecord
	if err := q.Find(&records).Error; err != nil {
		return nil, err
	}
	tasks := make([]task.Task, len(records))
	for i := range records {
		tasks[i] = *taskFromRecord(&records[i])
	}
	return tasks, nil
}

func (s *TaskStore) MarkInterruptedOnStartup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&TaskRecord{}).
			Where("status = ?", string(task.StatusRunning)).
			Updates(map[string]interface{}{
				"status":     string(task.StatusInterrupted),
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("mark interrupted: %w", err)
		}
		if err := tx.Model(&TaskRecord{}).
			Where("status = ?", string(task.StatusQueued)).
			Updates(map[string]interface{}{
				"status":       string(task.StatusPending),
				"scheduled_at": nil,
				"updated_at":   now,
			}).Error; err != nil {
			return fmt.Errorf("mark queued→pending: %w", err)
		}
		return nil
	})
}

func (s *TaskStore) FindDueTasks(ctx context.Context, now time.Time, limit int) ([]task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []TaskRecord
	err := s.db.WithContext(ctx).
		Where("status = ? AND (scheduled_at IS NULL OR scheduled_at <= ?)", string(task.StatusPending), now).
		Order("created_at ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	tasks := make([]task.Task, len(records))
	for i := range records {
		tasks[i] = *taskFromRecord(&records[i])
	}
	return tasks, nil
}

func (s *TaskStore) FindQueuedForKey(ctx context.Context, key string) (*task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r TaskRecord
	err := s.db.WithContext(ctx).
		Where("serialization_key = ? AND status = ?", key, string(task.StatusQueued)).
		Order("created_at ASC").
		First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return taskFromRecord(&r), nil
}

func (s *TaskStore) UpdateStatus(ctx context.Context, taskID string, fields map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fields["updated_at"] = time.Now()
	return s.db.WithContext(ctx).
		Model(&TaskRecord{}).
		Where("task_id = ?", taskID).
		Updates(fields).Error
}

// ---- conversion helpers ----

func taskFromRecord(r *TaskRecord) *task.Task {
	t := &task.Task{
		ID:               r.TaskID,
		Type:             r.Type,
		Status:           task.TaskStatus(r.Status),
		OwnerType:        r.OwnerType,
		OwnerID:          r.OwnerID,
		Source:           r.Source,
		SerializationKey: r.SerializationKey,
		Progress:         r.Progress,
		Error:            r.Error,
		Attempt:          r.Attempt,
		MaxAttempts:      r.MaxAttempts,
		ScheduledAt:      r.ScheduledAt,
		StartedAt:        r.StartedAt,
		FinishedAt:       r.FinishedAt,
		CancelledAt:      r.CancelledAt,
		ParentTaskID:     r.ParentTaskID,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
	if r.Payload != "" {
		t.Payload = json.RawMessage(r.Payload)
	}
	if r.Result != "" {
		t.Result = json.RawMessage(r.Result)
	}
	if r.Recurrence != "" {
		t.Recurrence = json.RawMessage(r.Recurrence)
	}
	return t
}

func recordFromTask(t *task.Task) *TaskRecord {
	r := &TaskRecord{
		TaskID:           t.ID,
		Type:             t.Type,
		Status:           string(t.Status),
		OwnerType:        t.OwnerType,
		OwnerID:          t.OwnerID,
		Source:           t.Source,
		SerializationKey: t.SerializationKey,
		Progress:         t.Progress,
		Error:            t.Error,
		Attempt:          t.Attempt,
		MaxAttempts:      t.MaxAttempts,
		ScheduledAt:      t.ScheduledAt,
		StartedAt:        t.StartedAt,
		FinishedAt:       t.FinishedAt,
		CancelledAt:      t.CancelledAt,
		ParentTaskID:     t.ParentTaskID,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
	if len(t.Payload) > 0 {
		r.Payload = string(t.Payload)
	}
	if len(t.Result) > 0 {
		r.Result = string(t.Result)
	}
	if len(t.Recurrence) > 0 {
		r.Recurrence = string(t.Recurrence)
	}
	return r
}
