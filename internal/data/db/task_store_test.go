package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/task"
)

func TestTaskStore_RoundTripSupervisorFields(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	next := now.Add(time.Hour)
	original := &task.Task{
		ID:               "task-1",
		Type:             "agent",
		Status:           task.StatusPending,
		SerializationKey: "/tmp/workspace",
		Payload:          json.RawMessage(`{"session_id":"native-1"}`),
		Result:           json.RawMessage(`{"state":"continue"}`),
		ScheduledAt:      &next,
		Recurrence:       json.RawMessage(`{"cron":"0 * * * *","timezone":"UTC"}`),
		MaxAttempts:      3,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := sm.Tasks().Create(ctx, original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := sm.Tasks().Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != task.StatusPending || string(got.Payload) != string(original.Payload) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if string(got.Recurrence) != string(original.Recurrence) {
		t.Fatalf("recurrence = %s", got.Recurrence)
	}

	if err := sm.Tasks().UpdateStatus(ctx, original.ID, map[string]interface{}{
		"status":       string(task.StatusNeedsInput),
		"payload":      `{"session_id":"native-2"}`,
		"scheduled_at": nil,
		"finished_at":  nil,
		"attempt":      0,
	}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err = sm.Tasks().Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Status != task.StatusNeedsInput {
		t.Fatalf("status = %s", got.Status)
	}
	if got.ScheduledAt != nil || got.FinishedAt != nil {
		t.Fatalf("timestamps not cleared: scheduled=%v finished=%v", got.ScheduledAt, got.FinishedAt)
	}
	if string(got.Payload) != `{"session_id":"native-2"}` {
		t.Fatalf("payload = %s", got.Payload)
	}
}

func TestTaskStore_TaskRunRoundTripAndRecovery(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sm.Close() })

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	run := &task.TaskRun{
		ID: "run-1", TaskID: "task-1", Attempt: 2, Status: task.RunStatusRunning,
		Input: json.RawMessage(`{"goal":"inspect"}`), Progress: "working",
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := sm.Tasks().CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err := sm.Tasks().GetRun(ctx, "task-1", "run-1")
	if err != nil || got.Status != task.RunStatusRunning || string(got.Input) != string(run.Input) {
		t.Fatalf("run=%+v err=%v", got, err)
	}
	if err := sm.Tasks().MarkInterruptedOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = sm.Tasks().GetRun(ctx, "task-1", "run-1")
	if err != nil || got.Status != task.RunStatusInterrupted || got.FinishedAt == nil {
		t.Fatalf("recovered run=%+v err=%v", got, err)
	}
}
