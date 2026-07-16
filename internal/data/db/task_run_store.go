package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/task"
)

type TaskRunRecord struct {
	ID         uint       `gorm:"primaryKey;autoIncrement;column:id"`
	RunID      string     `gorm:"uniqueIndex;column:run_id;not null;size:64"`
	TaskID     string     `gorm:"column:task_id;not null;size:64;index:idx_task_runs_task_created"`
	Attempt    int        `gorm:"column:attempt"`
	Status     string     `gorm:"column:status;not null;size:32;index"`
	Input      string     `gorm:"column:input;type:text"`
	Result     string     `gorm:"column:result;type:text"`
	Progress   string     `gorm:"column:progress;size:512"`
	Error      string     `gorm:"column:error;type:text"`
	StartedAt  time.Time  `gorm:"column:started_at"`
	FinishedAt *time.Time `gorm:"column:finished_at"`
	CreatedAt  time.Time  `gorm:"column:created_at;index:idx_task_runs_task_created"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (TaskRunRecord) TableName() string { return "task_runs" }

func (s *TaskStore) CreateRun(ctx context.Context, run *task.TaskRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.WithContext(ctx).Create(runRecordFromDomain(run)).Error
}

func (s *TaskStore) GetRun(ctx context.Context, taskID, runID string) (*task.TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var record TaskRunRecord
	err := s.db.WithContext(ctx).Where("task_id = ? AND run_id = ?", taskID, runID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, task.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return runDomainFromRecord(&record), nil
}

func (s *TaskStore) ListRuns(ctx context.Context, filter task.RunListFilter) ([]task.TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := s.db.WithContext(ctx).Model(&TaskRunRecord{})
	if filter.TaskID != "" {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	query = query.Order("created_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var records []TaskRunRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	runs := make([]task.TaskRun, len(records))
	for i := range records {
		runs[i] = *runDomainFromRecord(&records[i])
	}
	return runs, nil
}

func (s *TaskStore) UpdateRun(ctx context.Context, runID string, fields map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fields["updated_at"] = time.Now()
	return s.db.WithContext(ctx).Model(&TaskRunRecord{}).Where("run_id = ?", runID).Updates(fields).Error
}

func runRecordFromDomain(run *task.TaskRun) *TaskRunRecord {
	record := &TaskRunRecord{
		RunID: run.ID, TaskID: run.TaskID, Attempt: run.Attempt, Status: string(run.Status),
		Progress: run.Progress, Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}
	if len(run.Input) > 0 {
		record.Input = string(run.Input)
	}
	if len(run.Result) > 0 {
		record.Result = string(run.Result)
	}
	return record
}

func runDomainFromRecord(record *TaskRunRecord) *task.TaskRun {
	run := &task.TaskRun{
		ID: record.RunID, TaskID: record.TaskID, Attempt: record.Attempt, Status: task.RunStatus(record.Status),
		Progress: record.Progress, Error: record.Error, StartedAt: record.StartedAt, FinishedAt: record.FinishedAt,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if record.Input != "" {
		run.Input = json.RawMessage(record.Input)
	}
	if record.Result != "" {
		run.Result = json.RawMessage(record.Result)
	}
	return run
}
