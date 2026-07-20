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

func TestClone_CopiesDefinitionFreshRuntime(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	configDir := t.TempDir()
	handler := NewHandler(manager, configDir, nil, nil)
	router := testRouter(handler)
	router.POST("/tasks/:id/clone", handler.Clone)

	custom := t.TempDir()
	payload := agenttask.Payload{
		Version: 2, Title: "Nightly", Goal: "Ship", Agent: agenttask.AgentClaude, WorkspacePath: custom,
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
		Steps:     []agenttask.Step{{ID: "step-1", Title: "A", Instruction: "do a"}},
		SessionID: "live-session", WakeCount: 5, CurrentStep: 1,
		StepOutcomes: []agenttask.StepOutcome{{StepID: "step-1", Result: agenttask.Result{State: "done"}}},
	}
	data, _ := json.Marshal(payload)
	src, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: data})
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSON(t, router, http.MethodPost, "/tasks/"+src.ID+"/clone", "")
	if resp.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	clone := decoded.Data
	if clone.ID == src.ID {
		t.Fatal("clone must have a new id")
	}
	if clone.Title != "Nightly (copy)" || clone.Goal != "Ship" {
		t.Fatalf("definition not copied: %+v", clone)
	}
	// Custom workspace is reused; runtime state is fresh.
	if clone.WorkspacePath != custom {
		t.Fatalf("custom workspace should be reused: %s vs %s", clone.WorkspacePath, custom)
	}
	if clone.SessionID != "" || clone.WakeCount != 0 || clone.CurrentStep != 0 || len(clone.StepOutcomes) != 0 {
		t.Fatalf("runtime state leaked into clone: %+v", clone)
	}
	if len(clone.Steps) != 1 || clone.Steps[0].Instruction != "do a" {
		t.Fatalf("steps not copied: %+v", clone.Steps)
	}
}

func TestClone_GeneratedWorkspaceGetsFreshDir(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	configDir := t.TempDir()
	handler := NewHandler(manager, configDir, nil, nil)
	router := testRouter(handler)
	router.POST("/tasks/:id/clone", handler.Clone)

	created := performJSON(t, router, http.MethodPost, "/tasks", `{"goal":"echo hi","agent":"shell"}`)
	var srcResp TaskResponse
	_ = json.Unmarshal(created.Body.Bytes(), &srcResp)

	resp := performJSON(t, router, http.MethodPost, "/tasks/"+srcResp.Data.ID+"/clone", "")
	var cloneResp TaskResponse
	_ = json.Unmarshal(resp.Body.Bytes(), &cloneResp)
	if cloneResp.Data.WorkspacePath == "" || cloneResp.Data.WorkspacePath == srcResp.Data.WorkspacePath {
		t.Fatalf("generated workspace must be a fresh dir: src=%s clone=%s", srcResp.Data.WorkspacePath, cloneResp.Data.WorkspacePath)
	}
	if !strings.Contains(cloneResp.Data.WorkspacePath, cloneResp.Data.ID) {
		t.Fatalf("clone workspace should be under its own id: %s", cloneResp.Data.WorkspacePath)
	}
}
