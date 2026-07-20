package taskapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
)

func TestPauseResume_TogglesTrigger(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil, nil)
	router := testRouter(handler)
	router.POST("/tasks/:id/pause", handler.Pause)
	router.POST("/tasks/:id/resume", handler.Resume)
	router.DELETE("/tasks/:id", handler.Delete)

	payload := agenttask.Payload{
		Version: 2, Goal: "Ship", Agent: agenttask.AgentClaude, WorkspacePath: t.TempDir(),
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
	}
	data, _ := json.Marshal(payload)
	created, err := manager.Submit(context.Background(), coretask.SubmitRequest{
		Type: agenttask.TaskType, Payload: data,
		Recurrence: json.RawMessage(`{"cron":"0 9 * * *","timezone":"UTC"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSON(t, router, http.MethodPost, "/tasks/"+created.ID+"/pause", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", resp.Code, resp.Body.String())
	}
	var decoded TaskResponse
	_ = json.Unmarshal(resp.Body.Bytes(), &decoded)
	if !decoded.Data.TriggerPaused {
		t.Fatalf("task should be paused: %+v", decoded.Data)
	}

	// A paused task must not be dispatched even when due.
	stored, _ := manager.Get(context.Background(), created.ID)
	if !stored.TriggerPaused {
		t.Fatal("stored task not paused")
	}

	resp = performJSON(t, router, http.MethodPost, "/tasks/"+created.ID+"/resume", "")
	_ = json.Unmarshal(resp.Body.Bytes(), &decoded)
	if decoded.Data.TriggerPaused {
		t.Fatalf("task should be resumed: %+v", decoded.Data)
	}
}

func TestDelete_RemovesTask(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil, nil)
	router := testRouter(handler)
	router.DELETE("/tasks/:id", handler.Delete)

	payload := agenttask.Payload{
		Version: 2, Goal: "Ship", Agent: agenttask.AgentClaude, WorkspacePath: t.TempDir(),
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
	}
	data, _ := json.Marshal(payload)
	created, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: data})
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSON(t, router, http.MethodDelete, "/tasks/"+created.ID, "")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", resp.Code, resp.Body.String())
	}
	if _, err := manager.Get(context.Background(), created.ID); err == nil {
		t.Fatal("task should be gone after delete")
	}
}
