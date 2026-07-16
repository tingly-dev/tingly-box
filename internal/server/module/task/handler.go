package taskapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
	"github.com/tingly-dev/tingly-box/internal/task/shelltask"
)

// UsageReader is the slice of the usage store the task board needs for
// per-task cost attribution. Nil when usage tracking is unavailable.
type UsageReader interface {
	GetTaskUsageTotals(taskID string) (*db.TaskUsageTotals, error)
}

type Handler struct {
	manager   coretask.Manager
	configDir string
	agents    map[agenttask.AgentKind]agentboot.Agent
	usage     UsageReader
}

func NewHandler(manager coretask.Manager, configDir string, agents map[agenttask.AgentKind]agentboot.Agent, usage UsageReader) *Handler {
	return &Handler{manager: manager, configDir: configDir, agents: agents, usage: usage}
}

// Usage returns gateway usage attributed to one task's runs.
func (h *Handler) Usage(c *gin.Context) {
	ctx := c.Request.Context()
	task, err := h.manager.Get(ctx, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if !isBoardTask(task.Type) {
		writeError(c, coretask.ErrNotFound)
		return
	}
	view := TaskUsageView{TaskID: task.ID}
	if h.usage != nil {
		totals, err := h.usage.GetTaskUsageTotals(task.ID)
		if err != nil {
			writeError(c, err)
			return
		}
		view = TaskUsageView{
			TaskID: task.ID, Requests: totals.Requests,
			InputTokens: totals.InputTokens, OutputTokens: totals.OutputTokens,
			CacheInputTokens: totals.CacheInputTokens, TotalTokens: totals.TotalTokens,
		}
	}
	c.JSON(http.StatusOK, TaskUsageResponse{Data: view})
}

func (h *Handler) List(c *gin.Context) {
	tasks, err := h.manager.List(c.Request.Context(), coretask.ListFilter{Type: agenttask.TaskType, Limit: 200})
	if err != nil {
		writeError(c, err)
		return
	}
	shellTasks, err := h.manager.List(c.Request.Context(), coretask.ListFilter{Type: shelltask.TaskType, Limit: 200})
	if err != nil {
		writeError(c, err)
		return
	}
	tasks = mergeByCreatedAtDesc(tasks, shellTasks, 200)
	runs, err := h.manager.ListRuns(c.Request.Context(), coretask.RunListFilter{
		Status: []coretask.RunStatus{coretask.RunStatusRunning},
		Limit:  500,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	activeRuns := make(map[string]*coretask.TaskRun)
	for i := range runs {
		if runs[i].Status.IsActive() && activeRuns[runs[i].TaskID] == nil {
			activeRuns[runs[i].TaskID] = &runs[i]
		}
	}
	views := make([]TaskView, 0, len(tasks))
	for i := range tasks {
		view, err := toView(&tasks[i])
		if err != nil {
			writeError(c, err)
			return
		}
		attachRunAttention(&view, activeRuns[tasks[i].ID])
		views = append(views, view)
	}
	c.JSON(http.StatusOK, TaskListResponse{Data: views})
}

func (h *Handler) Get(c *gin.Context) {
	task, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if !isBoardTask(task.Type) {
		writeError(c, coretask.ErrNotFound)
		return
	}
	view, err := toView(task)
	if err != nil {
		writeError(c, err)
		return
	}
	if err := h.attachActiveRun(c.Request.Context(), task.ID, &view); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, TaskResponse{Data: view})
}

func (h *Handler) Update(c *gin.Context) {
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Title == nil && req.Goal == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title or goal is required"})
		return
	}

	ctx := c.Request.Context()
	task, err := h.manager.Get(ctx, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if !isBoardTask(task.Type) {
		writeError(c, coretask.ErrNotFound)
		return
	}
	if task.Type == shelltask.TaskType {
		var payload shelltask.Payload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			writeError(c, err)
			return
		}
		if req.Title != nil {
			payload.Title = strings.TrimSpace(*req.Title)
		}
		if req.Goal != nil {
			command := strings.TrimSpace(*req.Goal)
			if command == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "goal is required"})
				return
			}
			payload.Command = command
		}
		data, _ := json.Marshal(payload)
		if err := h.manager.UpdatePayload(ctx, task.ID, data); err != nil {
			writeError(c, err)
			return
		}
		updated, err := h.manager.Get(ctx, task.ID)
		if err != nil {
			writeError(c, err)
			return
		}
		view, _ := toView(updated)
		c.JSON(http.StatusOK, TaskResponse{Data: view})
		return
	}
	var payload agenttask.Payload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		writeError(c, err)
		return
	}
	if req.Title != nil {
		payload.Title = strings.TrimSpace(*req.Title)
	}
	if req.Goal != nil {
		goal := strings.TrimSpace(*req.Goal)
		if goal == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "goal is required"})
			return
		}
		payload.Goal = goal
	}
	data, err := json.Marshal(payload)
	if err != nil {
		writeError(c, err)
		return
	}
	if err := h.manager.UpdatePayload(ctx, task.ID, data); err != nil {
		writeError(c, err)
		return
	}
	updated, err := h.manager.Get(ctx, task.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := toView(updated)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, TaskResponse{Data: view})
}

func (h *Handler) ListRuns(c *gin.Context) {
	task, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if !isBoardTask(task.Type) {
		writeError(c, coretask.ErrNotFound)
		return
	}
	runs, err := h.manager.ListRuns(c.Request.Context(), coretask.RunListFilter{TaskID: task.ID, Limit: 200})
	if err != nil {
		writeError(c, err)
		return
	}
	views := make([]RunView, 0, len(runs))
	for i := range runs {
		view, err := toRunView(&runs[i])
		if err != nil {
			writeError(c, err)
			return
		}
		views = append(views, view)
	}
	c.JSON(http.StatusOK, RunListResponse{Data: views})
}

func (h *Handler) GetRun(c *gin.Context) {
	task, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if !isBoardTask(task.Type) {
		writeError(c, coretask.ErrNotFound)
		return
	}
	run, err := h.manager.GetRun(c.Request.Context(), task.ID, c.Param("runID"))
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := toRunView(run)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, RunResponse{Data: view})
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Goal = strings.TrimSpace(req.Goal)
	if req.Goal == "" || (!req.Agent.IsValid() && req.Agent != agentShell) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goal and a supported executor are required"})
		return
	}
	if req.Agent == agentShell {
		h.createShellTask(c, req)
		return
	}
	steps, err := normalizeSteps(req.Steps)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	execution := agenttask.DefaultExecutionPolicy(req.Agent)
	if req.Execution != nil {
		execution = *req.Execution
		execution.ApplyDefaults(req.Agent, false)
	}
	if err := execution.Validate(req.Agent, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	taskID := uuid.NewString()
	var workspace string
	if strings.TrimSpace(req.WorkspacePath) == "" {
		workspace, err = agenttask.CreateWorkspace(h.configDir, taskID)
		if err != nil {
			writeError(c, err)
			return
		}
	} else {
		workspace, err = agenttask.ResolveExistingWorkspace(req.WorkspacePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	payload := agenttask.Payload{
		Version:        2,
		Title:          strings.TrimSpace(req.Title),
		Goal:           req.Goal,
		Agent:          req.Agent,
		WorkspacePath:  workspace,
		FollowUp:       req.FollowUp,
		TimeoutSeconds: req.TimeoutSeconds,
		Steps:          steps,
		Execution:      execution,
	}
	payload.ApplyDefaults()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		writeError(c, err)
		return
	}

	var recurrence json.RawMessage
	if req.Recurrence != nil {
		recurrence, err = json.Marshal(req.Recurrence)
		if err != nil {
			writeError(c, err)
			return
		}
	}
	created, err := h.manager.Submit(c.Request.Context(), coretask.SubmitRequest{
		ID:               taskID,
		Type:             agenttask.TaskType,
		Source:           "webui",
		SerializationKey: workspace,
		Payload:          payloadJSON,
		MaxAttempts:      1,
		ScheduledAt:      req.ScheduledAt,
		Recurrence:       recurrence,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	view, _ := toView(created)
	c.JSON(http.StatusCreated, TaskResponse{Data: view})
}

func (h *Handler) Wake(c *gin.Context) {
	var req WakeRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	ctx := c.Request.Context()
	task, err := h.manager.Get(ctx, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if !isBoardTask(task.Type) {
		writeError(c, coretask.ErrNotFound)
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if task.Type == shelltask.TaskType && (instruction != "" || req.ExecutionOverride != nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "shell tasks rerun their command; instructions and execution overrides do not apply"})
		return
	}
	if instruction != "" || req.ExecutionOverride != nil {
		if task.Status != coretask.StatusNeedsInput && task.Status != coretask.StatusHandoff && !task.Status.IsTerminal() {
			c.JSON(http.StatusConflict, gin.H{"error": "instruction or execution override can only be sent to a paused or finished task"})
			return
		}
		var payload agenttask.Payload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			writeError(c, err)
			return
		}
		if instruction != "" {
			payload.PendingInput = instruction
		}
		if req.ExecutionOverride != nil {
			override := *req.ExecutionOverride
			override.ApplyDefaults(payload.Agent, false)
			if err := override.Validate(payload.Agent, false); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			payload.PendingExecution = &override
		}
		data, _ := json.Marshal(payload)
		if err := h.manager.UpdatePayload(ctx, task.ID, data); err != nil {
			writeError(c, err)
			return
		}
	}
	if err := h.manager.Wake(ctx, task.ID, time.Time{}); err != nil {
		writeError(c, err)
		return
	}
	updated, err := h.manager.Get(ctx, task.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	view, _ := toView(updated)
	c.JSON(http.StatusOK, TaskResponse{Data: view})
}

func (h *Handler) Stop(c *gin.Context) {
	if err := h.manager.Cancel(c.Request.Context(), c.Param("id"), "stopped by user"); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Agents(c *gin.Context) {
	data := make([]AgentAvailability, 0, 2)
	for _, kind := range []agenttask.AgentKind{agenttask.AgentClaude, agenttask.AgentCodex} {
		agent := h.agents[kind]
		item := AgentAvailability{Agent: kind, Available: agent != nil && agent.IsAvailable()}
		if kind == agenttask.AgentClaude {
			item.LaunchProfiles = []agenttask.LaunchProfile{agenttask.LaunchClaudePlan, agenttask.LaunchClaudeEdits}
			item.DefaultProfile = agenttask.LaunchClaudeEdits
			item.ToolFiltering = true
			item.Unattended = true
		} else {
			item.LaunchProfiles = []agenttask.LaunchProfile{agenttask.LaunchCodexReadOnly, agenttask.LaunchCodexWorkspace}
			item.DefaultProfile = agenttask.LaunchCodexWorkspace
			item.Unattended = true
		}
		data = append(data, item)
	}
	// The shell executor needs no external CLI and always runs unattended.
	data = append(data, AgentAvailability{Agent: agentShell, Available: true, Unattended: true})
	c.JSON(http.StatusOK, AgentListResponse{Data: data})
}

func toView(task *coretask.Task) (TaskView, error) {
	if task.Type == shelltask.TaskType {
		return shellToView(task)
	}
	var payload agenttask.Payload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return TaskView{}, fmt.Errorf("decode task %s payload: %w", task.ID, err)
	}
	payload.ApplyDefaults()
	view := TaskView{
		ID: task.ID, Title: payload.Title, Goal: payload.Goal, Agent: payload.Agent,
		Status: task.Status, Progress: task.Progress, Error: task.Error,
		WorkspacePath: payload.WorkspacePath, SessionID: payload.SessionID,
		FollowUp: payload.FollowUp, WakeCount: payload.WakeCount,
		ScheduledAt: task.ScheduledAt, StartedAt: task.StartedAt, FinishedAt: task.FinishedAt,
		CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
		Steps: payload.Steps, CurrentStep: payload.CurrentStep, StepOutcomes: payload.StepOutcomes,
		Execution: payload.Execution,
	}
	if payload.SessionID != "" {
		workspace := shellQuote(payload.WorkspacePath)
		sessionID := shellQuote(payload.SessionID)
		if payload.Agent == agenttask.AgentClaude {
			view.ResumeCommand = fmt.Sprintf("cd %s && claude --resume %s", workspace, sessionID)
		} else {
			view.ResumeCommand = fmt.Sprintf("cd %s && codex resume %s", workspace, sessionID)
		}
	}
	if len(task.Result) > 0 {
		var result agenttask.Result
		if err := json.Unmarshal(task.Result, &result); err == nil {
			view.LatestResult = &result
		}
	}
	if len(task.Recurrence) > 0 {
		var recurrence coretask.RecurrenceSpec
		if err := json.Unmarshal(task.Recurrence, &recurrence); err != nil {
			return TaskView{}, err
		}
		view.Recurrence = &recurrence
	}
	return view, nil
}

func (h *Handler) attachActiveRun(ctx context.Context, taskID string, view *TaskView) error {
	runs, err := h.manager.ListRuns(ctx, coretask.RunListFilter{TaskID: taskID, Limit: 1})
	if err != nil {
		return err
	}
	if len(runs) == 0 || !runs[0].Status.IsActive() {
		return nil
	}
	attachRunAttention(view, &runs[0])
	return nil
}

func attachRunAttention(view *TaskView, run *coretask.TaskRun) {
	if run == nil {
		return
	}
	view.ActiveRunID = run.ID
}

func toRunView(run *coretask.TaskRun) (RunView, error) {
	var payload agenttask.Payload
	if err := json.Unmarshal(run.Input, &payload); err != nil {
		return RunView{}, fmt.Errorf("decode run %s input: %w", run.ID, err)
	}
	payload.ApplyDefaults()
	execution := payload.Execution
	if payload.PendingExecution != nil {
		execution = *payload.PendingExecution
	}
	execution = execution.Automated(payload.Agent)
	view := RunView{
		ID: run.ID, TaskID: run.TaskID, Attempt: run.Attempt, Status: run.Status,
		Execution: execution, Progress: run.Progress, Error: run.Error,
		PendingControl: run.PendingControl, Events: run.Events,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
		Trigger: "run",
	}
	if payload.PendingInput != "" {
		view.Trigger = "instruction"
		view.Instruction = payload.PendingInput
	} else if payload.HasCurrentStep() {
		step := payload.Steps[payload.CurrentStep]
		index := payload.CurrentStep
		view.Trigger = "step"
		view.StepID = step.ID
		view.StepIndex = &index
		view.Instruction = step.Instruction
	}
	if len(run.Result) > 0 {
		var result agenttask.Result
		if err := json.Unmarshal(run.Result, &result); err == nil {
			view.Result = &result
		}
	}
	return view, nil
}

func normalizeSteps(input []CreateStep) ([]agenttask.Step, error) {
	if len(input) > agenttask.MaxSteps {
		return nil, fmt.Errorf("steps cannot exceed %d", agenttask.MaxSteps)
	}
	steps := make([]agenttask.Step, 0, len(input))
	for i, item := range input {
		instruction := strings.TrimSpace(item.Instruction)
		if instruction == "" {
			return nil, fmt.Errorf("step %d instruction is required", i+1)
		}
		title := strings.TrimSpace(strings.SplitN(instruction, "\n", 2)[0])
		runes := []rune(title)
		if len(runes) > 80 {
			title = string(runes[:80]) + "…"
		}
		steps = append(steps, agenttask.Step{
			ID: fmt.Sprintf("step-%d", i+1), Title: title, Instruction: instruction,
		})
	}
	return steps, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, coretask.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, coretask.ErrNotWakeable), errors.Is(err, coretask.ErrNotCancellable), errors.Is(err, coretask.ErrNotEditable):
		status = http.StatusConflict
	case errors.Is(err, coretask.ErrInvalidRecurrence):
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

// agentShell is the executor value for shell tasks on the shared
// agent/executor axis of the API. It is not an agenttask kind: shell tasks
// use shelltask payloads and have no session, steps, or launch profiles.
const agentShell = agenttask.AgentKind(shelltask.TaskType)

func isBoardTask(taskType string) bool {
	return taskType == agenttask.TaskType || taskType == shelltask.TaskType
}

func (h *Handler) createShellTask(c *gin.Context, req CreateRequest) {
	if len(req.Steps) > 0 || req.Execution != nil || req.FollowUp.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "steps, execution policy, and follow-up do not apply to shell tasks"})
		return
	}
	taskID := uuid.NewString()
	var workspace string
	var err error
	if strings.TrimSpace(req.WorkspacePath) == "" {
		workspace, err = agenttask.CreateWorkspace(h.configDir, taskID)
		if err != nil {
			writeError(c, err)
			return
		}
	} else {
		workspace, err = agenttask.ResolveExistingWorkspace(req.WorkspacePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	payload := shelltask.Payload{
		Title:          strings.TrimSpace(req.Title),
		Command:        req.Goal,
		WorkspacePath:  workspace,
		TimeoutSeconds: req.TimeoutSeconds,
	}
	payload.ApplyDefaults()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		writeError(c, err)
		return
	}
	var recurrence json.RawMessage
	if req.Recurrence != nil {
		recurrence, err = json.Marshal(req.Recurrence)
		if err != nil {
			writeError(c, err)
			return
		}
	}
	created, err := h.manager.Submit(c.Request.Context(), coretask.SubmitRequest{
		ID:               taskID,
		Type:             shelltask.TaskType,
		Source:           "webui",
		SerializationKey: workspace,
		Payload:          payloadJSON,
		MaxAttempts:      1,
		ScheduledAt:      req.ScheduledAt,
		Recurrence:       recurrence,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	view, _ := toView(created)
	c.JSON(http.StatusCreated, TaskResponse{Data: view})
}

func shellToView(task *coretask.Task) (TaskView, error) {
	var payload shelltask.Payload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return TaskView{}, fmt.Errorf("decode task %s payload: %w", task.ID, err)
	}
	view := TaskView{
		ID: task.ID, Title: payload.Title, Goal: payload.Command, Agent: agentShell,
		Status: task.Status, Progress: task.Progress, Error: task.Error,
		WorkspacePath: payload.WorkspacePath,
		ScheduledAt:   task.ScheduledAt, StartedAt: task.StartedAt, FinishedAt: task.FinishedAt,
		CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
	}
	if len(task.Result) > 0 {
		// shelltask.Result shares the agenttask.Result JSON shape by design.
		var result agenttask.Result
		if err := json.Unmarshal(task.Result, &result); err == nil {
			view.LatestResult = &result
		}
	}
	if len(task.Recurrence) > 0 {
		var recurrence coretask.RecurrenceSpec
		if err := json.Unmarshal(task.Recurrence, &recurrence); err != nil {
			return TaskView{}, err
		}
		view.Recurrence = &recurrence
	}
	return view, nil
}

func mergeByCreatedAtDesc(a, b []coretask.Task, limit int) []coretask.Task {
	merged := make([]coretask.Task, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].CreatedAt.After(merged[j].CreatedAt)
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
