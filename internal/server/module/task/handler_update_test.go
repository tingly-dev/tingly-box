package taskapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
)

func createAgentTask(t *testing.T, manager coretask.Manager, payload agenttask.Payload, recurrence json.RawMessage) *coretask.Task {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Submit(context.Background(), coretask.SubmitRequest{
		Type: agenttask.TaskType, Payload: data, Recurrence: recurrence,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestUpdate_EditsConfigBetweenRuns(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil, nil)
	router := testRouter(handler)
	created := createAgentTask(t, manager, agenttask.Payload{
		Version: 2, Goal: "Ship", Agent: agenttask.AgentClaude, WorkspacePath: t.TempDir(),
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
	}, nil)

	body := `{"follow_up":{"enabled":true,"delay_seconds":120,"max_wake_ups":5},"timeout_seconds":600,` +
		`"execution":{"launch_profile":"plan","tools":["files_read"]},` +
		`"recurrence":{"cron":"0 7 * * *","timezone":"UTC"}}`
	response := performJSON(t, router, http.MethodPatch, "/tasks/"+created.ID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	view := decoded.Data
	if !view.FollowUp.Enabled || view.FollowUp.DelaySeconds != 120 || view.FollowUp.MaxWakeUps != 5 {
		t.Fatalf("follow_up = %+v", view.FollowUp)
	}
	if view.Execution.LaunchProfile != agenttask.LaunchClaudePlan {
		t.Fatalf("execution = %+v", view.Execution)
	}
	if view.Recurrence == nil || view.Recurrence.Cron != "0 7 * * *" {
		t.Fatalf("recurrence = %+v", view.Recurrence)
	}
	if view.ScheduledAt == nil {
		t.Fatal("pending task should have the new occurrence materialized")
	}

	// Clearing the schedule switches back to manual.
	response = performJSON(t, router, http.MethodPatch, "/tasks/"+created.ID, `{"clear_recurrence":true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", response.Code, response.Body.String())
	}
	decoded = TaskResponse{}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Data.Recurrence != nil {
		t.Fatalf("recurrence should be cleared: %+v", decoded.Data.Recurrence)
	}
}

func TestUpdate_ReplacesFutureStepsKeepsCompleted(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil, nil)
	router := testRouter(handler)
	payload := agenttask.Payload{
		Version: 2, Goal: "Ship", Agent: agenttask.AgentClaude, WorkspacePath: t.TempDir(),
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
		Steps: []agenttask.Step{
			{ID: "step-1", Title: "Inspect", Instruction: "Inspect"},
			{ID: "step-2", Title: "Publish", Instruction: "Publish"},
		},
		CurrentStep: 1,
		StepOutcomes: []agenttask.StepOutcome{{StepID: "step-1", Result: agenttask.Result{State: "done", Summary: "inspected"}}},
	}
	created := createAgentTask(t, manager, payload, nil)

	body := `{"steps":[{"instruction":"Publish to staging first"},{"instruction":"Then publish to prod"}]}`
	response := performJSON(t, router, http.MethodPatch, "/tasks/"+created.ID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	view := decoded.Data
	if len(view.Steps) != 3 || view.Steps[0].ID != "step-1" || !strings.Contains(view.Steps[1].Instruction, "staging") {
		t.Fatalf("steps = %+v", view.Steps)
	}
	if view.CurrentStep != 1 || len(view.StepOutcomes) != 1 {
		t.Fatalf("cursor moved: current=%d outcomes=%d", view.CurrentStep, len(view.StepOutcomes))
	}
}
