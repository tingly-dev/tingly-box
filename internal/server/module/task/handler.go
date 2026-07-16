package taskapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
)

type Handler struct {
	manager   coretask.Manager
	configDir string
	agents    map[agenttask.AgentKind]agentboot.Agent
	controls  *agenttask.ControlBroker
}

func NewHandler(manager coretask.Manager, configDir string, agents map[agenttask.AgentKind]agentboot.Agent, controls ...*agenttask.ControlBroker) *Handler {
	broker := agenttask.NewControlBroker(0)
	if len(controls) > 0 && controls[0] != nil {
		broker = controls[0]
	}
	return &Handler{manager: manager, configDir: configDir, agents: agents, controls: broker}
}

func (h *Handler) List(c *gin.Context) {
	tasks, err := h.manager.List(c.Request.Context(), coretask.ListFilter{Type: agenttask.TaskType, Limit: 200})
	if err != nil {
		writeError(c, err)
		return
	}
	runs, err := h.manager.ListRuns(c.Request.Context(), coretask.RunListFilter{
		Status: []coretask.RunStatus{
			coretask.RunStatusRunning, coretask.RunStatusWaitingApproval, coretask.RunStatusWaitingInput,
		},
		Limit: 500,
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
	if task.Type != agenttask.TaskType {
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

func (h *Handler) ListRuns(c *gin.Context) {
	task, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if task.Type != agenttask.TaskType {
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
	if task.Type != agenttask.TaskType {
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

func (h *Handler) RespondControl(c *gin.Context) {
	taskRecord, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if taskRecord.Type != agenttask.TaskType {
		writeError(c, coretask.ErrNotFound)
		return
	}
	run, err := h.manager.GetRun(c.Request.Context(), taskRecord.ID, c.Param("runID"))
	if err != nil {
		writeError(c, err)
		return
	}
	controlID := c.Param("controlID")
	if run.PendingControl == nil || run.PendingControl.ID != controlID {
		c.JSON(http.StatusConflict, gin.H{"error": agenttask.ErrControlNotActive.Error()})
		return
	}
	var req ControlResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	decision := agenttask.ControlDecision{
		Action: strings.ToLower(strings.TrimSpace(req.Action)),
		Answer: strings.TrimSpace(req.Answer),
		Reason: strings.TrimSpace(req.Reason),
	}
	if err := h.controls.Respond(c.Request.Context(), run.ID, controlID, decision); err != nil {
		if errors.Is(err, agenttask.ErrControlNotActive) || errors.Is(err, agenttask.ErrControlAnswered) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, agenttask.ErrInvalidControlDecision) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		writeError(c, err)
		return
	}
	updated, err := h.manager.GetRun(c.Request.Context(), taskRecord.ID, run.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := toRunView(updated)
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
	if req.Goal == "" || !req.Agent.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goal and a supported agent are required"})
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
	workspace, err := agenttask.CreateWorkspace(h.configDir, taskID)
	if err != nil {
		writeError(c, err)
		return
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
	if task.Type != agenttask.TaskType {
		writeError(c, coretask.ErrNotFound)
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction != "" || req.ExecutionOverride != nil {
		if task.Status != coretask.StatusNeedsInput && !task.Status.IsTerminal() {
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
			item.LaunchProfiles = []agenttask.LaunchProfile{agenttask.LaunchClaudePlan, agenttask.LaunchClaudeManual, agenttask.LaunchClaudeEdits}
			item.DefaultProfile = agenttask.LaunchClaudeEdits
			item.ToolFiltering = true
			item.InteractiveControl = true
		} else {
			item.LaunchProfiles = []agenttask.LaunchProfile{agenttask.LaunchCodexReadOnly, agenttask.LaunchCodexWorkspace}
			item.DefaultProfile = agenttask.LaunchCodexWorkspace
		}
		data = append(data, item)
	}
	c.JSON(http.StatusOK, AgentListResponse{Data: data})
}

func toView(task *coretask.Task) (TaskView, error) {
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
			if mode := payload.Execution.ClaudePermissionMode(); mode != "" {
				view.ResumeCommand += " --permission-mode " + shellQuote(mode)
			}
			if tools := payload.Execution.ClaudeTools(); len(tools) > 0 {
				view.ResumeCommand += " --tools " + shellQuote(strings.Join(tools, ","))
			}
		} else {
			view.ResumeCommand = fmt.Sprintf("cd %s && codex exec", workspace)
			if sandbox := payload.Execution.CodexSandboxMode(); sandbox != "" {
				view.ResumeCommand += " -s " + shellQuote(sandbox)
			}
			view.ResumeCommand += " resume " + sessionID
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
	if run.PendingControl != nil {
		control := *run.PendingControl
		view.Attention = &control
	}
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
	case errors.Is(err, coretask.ErrNotWakeable), errors.Is(err, coretask.ErrNotCancellable):
		status = http.StatusConflict
	case errors.Is(err, coretask.ErrInvalidRecurrence):
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
