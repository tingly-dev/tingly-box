package taskapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	router.GET("/tasks", handler.List)
	router.GET("/tasks/:id", handler.Get)
	router.PATCH("/tasks/:id", handler.Update)
	router.POST("/tasks/:id/wake", handler.Wake)
	router.GET("/tasks/:id/runs", handler.ListRuns)
	router.GET("/tasks/:id/runs/:runID", handler.GetRun)
	return router
}

func TestGet_SurfacesOnlyActiveRun(t *testing.T) {
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
	if err := store.CreateRun(context.Background(), &coretask.TaskRun{
		ID: "run-1", TaskID: created.ID, Attempt: 1, Status: coretask.RunStatusRunning,
		Input: data, StartedAt: now, CreatedAt: now, UpdatedAt: now,
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
	if decoded.Data.ActiveRunID != "run-1" {
		t.Fatalf("task view = %+v", decoded.Data)
	}
	listResponse := performJSON(t, router, http.MethodGet, "/tasks", "")
	var listed TaskListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ActiveRunID != "run-1" {
		t.Fatalf("task list = %+v", listed.Data)
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

func TestCreate_UsesCanonicalExistingWorkspace(t *testing.T) {
	configDir := t.TempDir()
	root := t.TempDir()
	workspace := filepath.Join(root, "project")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "project-alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(CreateRequest{
		Goal: "Update the project", Agent: agenttask.AgentCodex, WorkspacePath: alias,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := coretask.NewManager(coretask.NewMemoryStore())
	response := performJSON(t, testRouter(NewHandler(manager, configDir, nil)), http.MethodPost, "/tasks", string(request))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Data.WorkspacePath != canonicalWorkspace {
		t.Fatalf("workspace = %q, want %q", decoded.Data.WorkspacePath, canonicalWorkspace)
	}
	stored, err := manager.Get(context.Background(), decoded.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SerializationKey != canonicalWorkspace {
		t.Fatalf("serialization key = %q, want %q", stored.SerializationKey, canonicalWorkspace)
	}
	generated := filepath.Join(configDir, "tasks", decoded.Data.ID, "workspace")
	if _, err := os.Stat(generated); !os.IsNotExist(err) {
		t.Fatalf("custom workspace should not create generated directory; stat error = %v", err)
	}
}

func TestCreate_RejectsInvalidExistingWorkspace(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "workspace-file")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
	}{
		{name: "relative", path: "relative/path"},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing")},
		{name: "file", path: file.Name()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := json.Marshal(CreateRequest{
				Goal: "Update the project", Agent: agenttask.AgentClaude, WorkspacePath: test.path,
			})
			if err != nil {
				t.Fatal(err)
			}
			handler := NewHandler(coretask.NewManager(coretask.NewMemoryStore()), t.TempDir(), nil)
			response := performJSON(t, testRouter(handler), http.MethodPost, "/tasks", string(request))
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "workspace") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
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

func TestUpdate_EditsDurableDefinitionAndPreservesRuntimeState(t *testing.T) {
	store := coretask.NewMemoryStore()
	manager := coretask.NewManager(store)
	payload := agenttask.Payload{
		Version: 2, Title: "Old title", Goal: "Old goal", Agent: agenttask.AgentClaude,
		WorkspacePath: t.TempDir(), SessionID: "session-1", PendingInput: "one-time input",
		Steps:     []agenttask.Step{{ID: "step-1", Title: "Inspect", Instruction: "Inspect the build"}},
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: data})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(context.Background(), created.ID, map[string]interface{}{"status": string(coretask.StatusNeedsInput)}); err != nil {
		t.Fatal(err)
	}

	response := performJSON(t, testRouter(NewHandler(manager, t.TempDir(), nil)), http.MethodPatch, "/tasks/"+created.ID, `{"title":" New title ","goal":" New goal "}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Data.Title != "New title" || decoded.Data.Goal != "New goal" || decoded.Data.Status != coretask.StatusNeedsInput {
		t.Fatalf("updated view = %+v", decoded.Data)
	}
	stored, err := manager.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var updated agenttask.Payload
	if err := json.Unmarshal(stored.Payload, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.WorkspacePath != payload.WorkspacePath || updated.SessionID != payload.SessionID || updated.PendingInput != payload.PendingInput || len(updated.Steps) != 1 || updated.Execution.LaunchProfile != payload.Execution.LaunchProfile {
		t.Fatalf("runtime state changed: %+v", updated)
	}
}

func TestUpdate_ValidatesGoalAndState(t *testing.T) {
	for _, status := range []coretask.TaskStatus{coretask.StatusRunning, coretask.StatusQueued} {
		t.Run(string(status), func(t *testing.T) {
			store := coretask.NewMemoryStore()
			manager := coretask.NewManager(store)
			data, _ := json.Marshal(agenttask.Payload{Version: 2, Goal: "Old goal", Agent: agenttask.AgentCodex, WorkspacePath: t.TempDir()})
			created, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: data})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateStatus(context.Background(), created.ID, map[string]interface{}{"status": string(status)}); err != nil {
				t.Fatal(err)
			}
			response := performJSON(t, testRouter(NewHandler(manager, t.TempDir(), nil)), http.MethodPatch, "/tasks/"+created.ID, `{"goal":"New goal"}`)
			if response.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	manager := coretask.NewManager(coretask.NewMemoryStore())
	data, _ := json.Marshal(agenttask.Payload{Version: 2, Goal: "Old goal", Agent: agenttask.AgentCodex, WorkspacePath: t.TempDir()})
	created, err := manager.Submit(context.Background(), coretask.SubmitRequest{Type: agenttask.TaskType, Payload: data})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(manager, t.TempDir(), nil)
	for _, body := range []string{`{}`, `{"goal":"   "}`} {
		response := performJSON(t, testRouter(handler), http.MethodPatch, "/tasks/"+created.ID, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
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
			want: "cd '/tmp/user'\"'\"'s task' && codex resume 'thread-1'",
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

func TestToView_ResumeCommandIsInteractiveNativeHandoff(t *testing.T) {
	payload := agenttask.Payload{
		Version: 2, Goal: "Work", Agent: agenttask.AgentClaude, WorkspacePath: "/tmp/task", SessionID: "session-1",
		Execution: agenttask.DefaultExecutionPolicy(agenttask.AgentClaude),
	}
	data, _ := json.Marshal(payload)
	view, err := toView(&coretask.Task{ID: "task-1", Payload: data})
	if err != nil {
		t.Fatal(err)
	}
	if view.ResumeCommand != "cd '/tmp/task' && claude --resume 'session-1'" {
		t.Fatalf("resume command = %q", view.ResumeCommand)
	}
}

func TestCreate_ShellTask(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil)
	router := testRouter(handler)

	workspace := t.TempDir()
	body := `{"goal":"echo hi","agent":"shell","workspace_path":"` + workspace + `"}`
	response := performJSON(t, router, http.MethodPost, "/tasks", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded TaskResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	view := decoded.Data
	if view.Agent != agentShell || view.Goal != "echo hi" || view.WorkspacePath == "" {
		t.Fatalf("view = %+v", view)
	}
	if view.SessionID != "" || view.ResumeCommand != "" || len(view.Steps) != 0 {
		t.Fatalf("shell view leaked agent-only fields: %+v", view)
	}

	// List merges shell tasks with agent tasks.
	listResponse := performJSON(t, router, http.MethodGet, "/tasks", "")
	var list TaskListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].Agent != agentShell {
		t.Fatalf("list = %+v", list.Data)
	}
}

func TestCreate_ShellTaskRejectsAgentOnlyOptions(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil)
	router := testRouter(handler)

	body := `{"goal":"echo hi","agent":"shell","steps":[{"instruction":"x"}]}`
	response := performJSON(t, router, http.MethodPost, "/tasks", body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "do not apply") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWake_ShellTaskRejectsInstruction(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil)
	router := testRouter(handler)

	created := performJSON(t, router, http.MethodPost, "/tasks", `{"goal":"true","agent":"shell","workspace_path":"`+t.TempDir()+`"}`)
	var decoded TaskResponse
	if err := json.Unmarshal(created.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, router, http.MethodPost, "/tasks/"+decoded.Data.ID+"/wake", `{"instruction":"again"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAgents_IncludesShellExecutor(t *testing.T) {
	manager := coretask.NewManager(coretask.NewMemoryStore())
	handler := NewHandler(manager, t.TempDir(), nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/agents", handler.Agents)
	response := performJSON(t, router, http.MethodGet, "/agents", "")
	var decoded AgentListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range decoded.Data {
		if item.Agent == agentShell {
			found = item.Available && item.Unattended
		}
	}
	if !found {
		t.Fatalf("shell executor missing or unavailable: %+v", decoded.Data)
	}
}
