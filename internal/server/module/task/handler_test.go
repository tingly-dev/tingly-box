package taskapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
)

func testRouter(handler *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/tasks", handler.Create)
	router.GET("/tasks/:id", handler.Get)
	router.POST("/tasks/:id/wake", handler.Wake)
	router.GET("/tasks/:id/runs", handler.ListRuns)
	router.GET("/tasks/:id/runs/:runID", handler.GetRun)
	router.POST("/tasks/:id/runs/:runID/control/:controlID/respond", handler.RespondControl)
	return router
}

func TestGet_SurfacesActiveControlAndRejectsStaleDelivery(t *testing.T) {
	store := coretask.NewMemoryStore()
	manager := coretask.NewManager(store)
	payload := agenttask.Payload{
		Version: 2, Goal: "Release", Agent: agenttask.AgentClaude, WorkspacePath: t.TempDir(),
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
	}
	data, _ := json.Marshal(payload)
	created, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: data})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	control := &coretask.PendingControl{
		ID: "control-1", Kind: coretask.ControlKindApproval, ToolName: "Bash",
		Input: json.RawMessage(`{"command":"go test ./..."}`), CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := store.CreateRun(context.Background(), &coretask.TaskRun{
		ID: "run-1", TaskID: created.ID, Attempt: 1, Status: coretask.RunStatusWaitingApproval,
		Input: data, PendingControl: control, StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(manager, t.TempDir(), nil)
	router := testRouter(handler)
	response := performJSON(t, router, http.MethodGet, "/tasks/"+created.ID, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Data.ActiveRunID != "run-1" || decoded.Data.Attention == nil || decoded.Data.Attention.ID != control.ID {
		t.Fatalf("task view = %+v", decoded.Data)
	}

	// A persisted control without a live broker is deliberately not actionable
	// after process restart.
	response = performJSON(t, router, http.MethodPost,
		"/tasks/"+created.ID+"/runs/run-1/control/control-1/respond", `{"action":"approve"}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), agenttask.ErrControlNotActive.Error()) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuns_ReturnsExecutionHistory(t *testing.T) {
	store := coretask.NewMemoryStore()
	manager := coretask.NewManager(store)
	payload := agenttask.Payload{
		Version: 2, Goal: "Ship", Agent: agenttask.AgentClaude, WorkspacePath: t.TempDir(),
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
		Steps:     []agenttask.Step{{ID: "step-1", Title: "Inspect", Instruction: "Inspect the build"}},
	}
	data, _ := json.Marshal(payload)
	created, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: data})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	result := json.RawMessage(`{"state":"done","summary":"complete"}`)
	if err := store.CreateRun(context.Background(), &coretask.TaskRun{
		ID: "run-1", TaskID: created.ID, Attempt: 1, Status: coretask.RunStatusSucceeded,
		Input: data, Result: result, StartedAt: now, FinishedAt: &now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(manager, t.TempDir(), nil)
	response := performJSON(t, testRouter(handler), http.MethodGet, "/tasks/"+created.ID+"/runs", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded RunListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Data) != 1 || decoded.Data[0].StepID != "step-1" || decoded.Data[0].Execution.LaunchProfile != agenttask.LaunchClaudeEdits || decoded.Data[0].Result == nil {
		t.Fatalf("runs = %+v", decoded.Data)
	}
}

func performJSON(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestCreate_GeneratesStableWorkspaceAndTask(t *testing.T) {
	configDir := t.TempDir()
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, configDir, nil)
	response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks", `{
		"title":"Build report",
		"goal":"Generate a report",
		"agent":"claude",
		"follow_up":{"enabled":true,"delay_seconds":120,"max_wake_ups":4}
	}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	view := decoded.Data
	canonicalConfigDir, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		t.Fatal(err)
	}
	wantParent := filepath.Join(canonicalConfigDir, "tasks", view.ID)
	if view.Status != coretask.StatusPending || view.Agent != agenttask.AgentClaude {
		t.Fatalf("view = %+v", view)
	}
	if view.Execution.LaunchProfile != agenttask.LaunchClaudeEdits {
		t.Fatalf("execution = %+v", view.Execution)
	}
	if !strings.HasPrefix(view.WorkspacePath, wantParent+string(filepath.Separator)) {
		t.Fatalf("workspace %q is not under %q", view.WorkspacePath, wantParent)
	}
	stored, err := manager.Get(context.Background(), view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SerializationKey != view.WorkspacePath || string(stored.Payload) == "" {
		t.Fatalf("stored task = %+v", stored)
	}
}

func TestCreate_RejectsUnsupportedAgent(t *testing.T) {
	handler := NewHandler(coretask.NewManager(coretask.NewMemoryStore()), t.TempDir(), nil)
	response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks", `{
		"goal":"Do work",
		"agent":"other"
	}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreate_RejectsUnsupportedExecutionPolicy(t *testing.T) {
	handler := NewHandler(coretask.NewManager(coretask.NewMemoryStore()), t.TempDir(), nil)
	response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks", `{
		"goal":"Do work",
		"agent":"codex",
		"execution":{"launch_profile":"workspace_write","tools":["terminal"]}
	}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "does not support per-task tool filtering") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreate_NormalizesSequentialSteps(t *testing.T) {
	handler := NewHandler(coretask.NewManager(coretask.NewMemoryStore()), t.TempDir(), nil)
	response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks", `{
		"goal":"Ship the release",
		"agent":"codex",
		"steps":[
			{"instruction":"Inspect the build\nand summarize failures"},
			{"instruction":"Publish the artifacts"}
		]
	}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Data.Steps) != 2 || decoded.Data.Steps[0].ID != "step-1" || decoded.Data.Steps[0].Title != "Inspect the build" || decoded.Data.CurrentStep != 0 {
		t.Fatalf("steps = %+v", decoded.Data.Steps)
	}
}

func TestCreate_RejectsBlankSequentialStep(t *testing.T) {
	handler := NewHandler(coretask.NewManager(coretask.NewMemoryStore()), t.TempDir(), nil)
	response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks", `{
		"goal":"Ship the release",
		"agent":"codex",
		"steps":[{"instruction":"   "}]
	}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "step 1 instruction is required") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWake_WithInstructionResumesPausedTask(t *testing.T) {
	store := coretask.NewMemoryStore()
	manager := coretask.NewManager(store)
	payload, _ := json.Marshal(agenttask.Payload{
		Version: 1, Goal: "Deploy", Agent: agenttask.AgentCodex,
		WorkspacePath: t.TempDir(), SessionID: "thread-1",
	})
	task, err := manager.Submit(context.Background(), coretask.SubmitRequest{
		Type: agenttask.TaskType, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(context.Background(), task.ID, map[string]interface{}{
		"status": string(coretask.StatusNeedsInput),
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(manager, t.TempDir(), nil)
	response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks/"+task.ID+"/wake", `{
		"instruction":"Use staging"
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	updated, err := manager.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got agenttask.Payload
	if err := json.Unmarshal(updated.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if updated.Status != coretask.StatusPending || got.PendingInput != "Use staging" || got.SessionID != "thread-1" {
		t.Fatalf("updated status=%s payload=%+v", updated.Status, got)
	}
}

func TestWake_AcceptsOneRunExecutionOverride(t *testing.T) {
	store := coretask.NewMemoryStore()
	manager := coretask.NewManager(store)
	payload, _ := json.Marshal(agenttask.Payload{
		Version: 2, Goal: "Inspect", Agent: agenttask.AgentClaude,
		WorkspacePath: t.TempDir(), Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
	})
	task, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(context.Background(), task.ID, map[string]interface{}{"status": string(coretask.StatusNeedsInput)}); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(manager, t.TempDir(), nil)
	response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks/"+task.ID+"/wake", `{
		"execution_override":{"launch_profile":"plan","tools":["files_read"]}
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	updated, err := manager.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got agenttask.Payload
	if err := json.Unmarshal(updated.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.PendingExecution == nil || got.PendingExecution.LaunchProfile != agenttask.LaunchClaudePlan {
		t.Fatalf("payload = %+v", got)
	}
}

func TestToView_ResumeCommandStartsInWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		agent     agenttask.AgentKind
		workspace string
		sessionID string
		want      string
	}{
		{
			name:  "claude",
			agent: agenttask.AgentClaude, workspace: "/tmp/task workspace", sessionID: "session-1",
			want: "cd '/tmp/task workspace' && claude --resume 'session-1'",
		},
		{
			name:  "codex quotes shell values",
			agent: agenttask.AgentCodex, workspace: "/tmp/user's task", sessionID: "thread-1",
			want: "cd '/tmp/user'\"'\"'s task' && codex exec resume 'thread-1'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(agenttask.Payload{
				Version: 1, Goal: "Resume work", Agent: tt.agent,
				WorkspacePath: tt.workspace, SessionID: tt.sessionID,
			})
			if err != nil {
				t.Fatal(err)
			}
			view, err := toView(&coretask.Task{ID: "task-1", Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			if view.ResumeCommand != tt.want {
				t.Fatalf("resume command = %q, want %q", view.ResumeCommand, tt.want)
			}
		})
	}
}

func TestToView_ResumeCommandIncludesExecutionPolicy(t *testing.T) {
	tests := []struct {
		name     string
		payload  agenttask.Payload
		contains []string
	}{
		{
			name: "Claude tools and permission mode",
			payload: agenttask.Payload{
				Version: 2, Goal: "Work", Agent: agenttask.AgentClaude, WorkspacePath: "/tmp/task", SessionID: "session-1",
				Execution: agenttask.ExecutionPolicy{LaunchProfile: agenttask.LaunchClaudeManual, Tools: []agenttask.ToolCapability{agenttask.ToolFilesRead, agenttask.ToolTerminal}},
			},
			contains: []string{"--permission-mode 'manual'", "--tools 'Read,Glob,Grep,Bash'"},
		},
		{
			name: "Codex sandbox",
			payload: agenttask.Payload{
				Version: 2, Goal: "Inspect", Agent: agenttask.AgentCodex, WorkspacePath: "/tmp/task", SessionID: "thread-1",
				Execution: agenttask.ExecutionPolicy{LaunchProfile: agenttask.LaunchCodexReadOnly},
			},
			contains: []string{"codex exec -s 'read-only' resume 'thread-1'"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := json.Marshal(tt.payload)
			view, err := toView(&coretask.Task{ID: "task-1", Payload: data})
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range tt.contains {
				if !strings.Contains(view.ResumeCommand, value) {
					t.Fatalf("resume command %q does not contain %q", view.ResumeCommand, value)
				}
			}
		})
	}
}
