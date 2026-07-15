package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/task"
)

func TestNextOccurrence_UTC(t *testing.T) {
	after := time.Date(2026, time.July, 15, 12, 1, 30, 0, time.UTC)
	next, err := task.NextOccurrence(
		json.RawMessage(`{"cron":"*/5 * * * *","timezone":"UTC"}`),
		after,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.July, 15, 12, 5, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("want %s, got %s", want, next)
	}
}

func TestNextOccurrence_Timezone(t *testing.T) {
	// 00:30 UTC is 08:30 in Asia/Shanghai. The next local 09:00 is 01:00 UTC.
	after := time.Date(2026, time.July, 15, 0, 30, 0, 0, time.UTC)
	next, err := task.NextOccurrence(
		json.RawMessage(`{"cron":"0 9 * * *","timezone":"Asia/Shanghai"}`),
		after,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.July, 15, 1, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("want %s, got %s", want, next)
	}
}

func TestNextOccurrence_DefaultsToUTC(t *testing.T) {
	after := time.Date(2026, time.July, 15, 12, 1, 0, 0, time.UTC)
	next, err := task.NextOccurrence(json.RawMessage(`{"cron":"0 13 * * *"}`), after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.July, 15, 13, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("want %s, got %s", want, next)
	}
}

func TestNextOccurrence_Invalid(t *testing.T) {
	tests := []json.RawMessage{
		json.RawMessage(`not-json`),
		json.RawMessage(`{"cron":""}`),
		json.RawMessage(`{"cron":"not a cron"}`),
		json.RawMessage(`{"cron":"@every 1s"}`),
		json.RawMessage(`{"cron":"0 9 * * *","timezone":"Mars/Olympus"}`),
	}
	for _, recurrence := range tests {
		_, err := task.NextOccurrence(recurrence, time.Now())
		if !errors.Is(err, task.ErrInvalidRecurrence) {
			t.Errorf("%s: want ErrInvalidRecurrence, got %v", recurrence, err)
		}
	}
}

func TestSubmit_InvalidRecurrence(t *testing.T) {
	mgr, _ := newManager(t)
	_, err := mgr.Submit(context.Background(), task.SubmitRequest{
		Type:       "noop",
		Recurrence: json.RawMessage(`{"cron":"bad"}`),
	})
	if !errors.Is(err, task.ErrInvalidRecurrence) {
		t.Fatalf("want ErrInvalidRecurrence, got %v", err)
	}
}

func TestRun_RecurringCompleteReusesTask(t *testing.T) {
	mgr, _ := newManager(t)
	run := make(chan struct{}, 1)
	mustRegister(t, mgr, &funcHandler{
		typ: "recurring",
		fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
			run <- struct{}{}
			return &task.TaskResult{Outcome: task.OutcomeComplete}, nil
		},
	})

	now := time.Now()
	tk := mustSubmit(t, mgr, task.SubmitRequest{
		Type:        "recurring",
		ScheduledAt: &now,
		Recurrence:  json.RawMessage(`{"cron":"0 0 * * *","timezone":"UTC"}`),
	})

	select {
	case <-run:
	case <-time.After(3 * time.Second):
		t.Fatal("recurring task did not run")
	}
	waiting := waitStatus(t, mgr, tk.ID, task.StatusPending)
	if waiting.ScheduledAt == nil || !waiting.ScheduledAt.After(time.Now()) {
		t.Fatalf("next occurrence not materialized: %v", waiting.ScheduledAt)
	}
	if waiting.Attempt != 0 {
		t.Fatalf("retry attempt should reset, got %d", waiting.Attempt)
	}

	all, err := mgr.List(context.Background(), task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != tk.ID {
		t.Fatalf("recurrence should reuse one task row, got %v", ids(all))
	}
}
