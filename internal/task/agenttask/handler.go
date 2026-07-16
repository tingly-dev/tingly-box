package agenttask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/task"
)

const (
	outcomeOpenTag  = "<task_outcome>"
	outcomeCloseTag = "</task_outcome>"
	maxSummaryRunes = 4000
)

const outcomeSystemAppendix = `At the end of this bounded execution, include exactly one machine-readable outcome using this format:
<task_outcome>{"state":"done|continue|needs_input","summary":"short result","question":"optional question","artifacts":["relative/path"],"suggested_delay_seconds":300}</task_outcome>
Use "continue" only when more work or observation is genuinely required. Use "needs_input" when a human decision is required. Artifact paths must be relative to the current workspace.`

type EnvResolver func(ctx context.Context, agent AgentKind) ([]string, error)

// Handler executes one bounded native-agent process for an agent Task.
type Handler struct {
	agents      map[AgentKind]agentboot.Agent
	envResolver EnvResolver
	now         func() time.Time
}

func NewHandler(agents map[AgentKind]agentboot.Agent, envResolver EnvResolver) *Handler {
	registered := make(map[AgentKind]agentboot.Agent, len(agents))
	for kind, agent := range agents {
		registered[kind] = agent
	}
	return &Handler{
		agents: registered, envResolver: envResolver,
		now: time.Now,
	}
}

func (h *Handler) Type() string { return TaskType }

func (h *Handler) Run(ctx context.Context, t *task.Task, ctl task.Controller) (*task.TaskResult, error) {
	var payload Payload
	if err := json.Unmarshal(t.Payload, &payload); err != nil {
		return nil, fmt.Errorf("agent task: decode payload: %w", err)
	}
	payload.ApplyDefaults()
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("agent task: validate payload: %w", err)
	}
	if err := validateWorkspace(payload.WorkspacePath); err != nil {
		return nil, fmt.Errorf("agent task: %w", err)
	}

	agent, ok := h.agents[payload.Agent]
	if !ok || agent == nil {
		return nil, fmt.Errorf("agent task: %s worker is not registered", payload.Agent)
	}
	if !agent.IsAvailable() {
		return nil, fmt.Errorf("agent task: %s CLI is not available", payload.Agent)
	}
	if len(payload.Steps) > 0 && payload.CurrentStep == len(payload.Steps) && strings.TrimSpace(payload.PendingInput) == "" {
		payload.CurrentStep = 0
		payload.StepOutcomes = nil
		if err := checkpointPayload(ctx, ctl, &payload); err != nil {
			return nil, err
		}
	}

	resume := payload.SessionID != ""
	sessionID := payload.SessionID
	execution := payload.Execution
	if payload.PendingExecution != nil {
		execution = *payload.PendingExecution
	}
	execution = execution.Automated(payload.Agent)
	if payload.Agent == AgentClaude && !resume {
		sessionID = uuid.NewString()
	}

	prompt := nextPrompt(payload, resume)
	var env []string
	if h.envResolver != nil {
		resolved, err := h.envResolver(ctx, payload.Agent)
		if err != nil {
			return nil, fmt.Errorf("agent task: resolve environment: %w", err)
		}
		env = resolved
	}

	handle, err := agent.Execute(ctx, prompt, agentboot.ExecutionOptions{
		ProjectPath:        payload.WorkspacePath,
		OutputFormat:       agentboot.OutputFormatStreamJSON,
		Timeout:            time.Duration(payload.TimeoutSeconds) * time.Second,
		Env:                env,
		SessionID:          sessionID,
		Resume:             resume,
		AppendSystemPrompt: outcomeSystemAppendix,
		AvailableTools:     execution.ClaudeTools(),
		AllowedTools:       execution.ClaudeTools(),
		PermissionMode:     execution.ClaudePermissionMode(),
		SandboxMode:        execution.CodexSandboxMode(),
	})
	if err != nil {
		return nil, fmt.Errorf("agent task: start %s: %w", payload.Agent, err)
	}

	checkpointAfterStart := false
	if payload.SessionID != sessionID {
		payload.SessionID = sessionID
		checkpointAfterStart = true
	}
	if payload.PendingInput != "" {
		payload.PendingInput = ""
		checkpointAfterStart = true
	}
	if payload.PendingExecution != nil {
		payload.PendingExecution = nil
		checkpointAfterStart = true
	}
	if checkpointAfterStart {
		if err := checkpointPayload(ctx, ctl, &payload); err != nil {
			cancelAndWait(ctx, handle, ctl)
			return nil, err
		}
	}

	runCtl, _ := ctl.(task.RunController)
	runtimePause, eventErr := consumeEvents(ctx, handle, ctl, runCtl, func(nativeSessionID string) error {
		if nativeSessionID == "" || nativeSessionID == payload.SessionID {
			return nil
		}
		payload.SessionID = nativeSessionID
		return checkpointPayload(ctx, ctl, &payload)
	})
	agentResult, waitErr := handle.Wait()
	if eventErr != nil {
		return nil, eventErr
	}
	if runtimePause != nil {
		runtimePause.NativeSessionID = payload.SessionID
		payload.WakeCount++
		if err := checkpointPayload(ctx, ctl, &payload); err != nil {
			return nil, err
		}
		return h.taskResult(ctx, ctl, &payload, *runtimePause)
	}
	if waitErr != nil {
		return nil, fmt.Errorf("agent task: %s execution: %w", payload.Agent, waitErr)
	}
	if agentResult == nil {
		return nil, errors.New("agent task: worker returned no result")
	}

	if nativeID := strings.TrimSpace(agentResult.GetSessionID()); nativeID != "" && nativeID != payload.SessionID {
		payload.SessionID = nativeID
	}
	payload.WakeCount++
	if err := checkpointPayload(ctx, ctl, &payload); err != nil {
		return nil, err
	}

	normalized := parseOutcome(agentResult.TextOutput())
	normalized.NativeSessionID = payload.SessionID
	normalized.Artifacts = safeArtifacts(normalized.Artifacts)
	return h.taskResult(ctx, ctl, &payload, normalized)
}

func nextPrompt(payload Payload, resume bool) string {
	if strings.TrimSpace(payload.PendingInput) != "" {
		if payload.HasCurrentStep() {
			step := payload.Steps[payload.CurrentStep]
			return fmt.Sprintf("Overall task goal:\n%s\n\nCurrent step %d of %d — %s\n%s\n\nUser instruction for this step:\n%s\n\nContinue only this step. Do not start later steps.", payload.Goal, payload.CurrentStep+1, len(payload.Steps), step.Title, step.Instruction, payload.PendingInput)
		}
		return payload.PendingInput
	}
	if payload.HasCurrentStep() {
		step := payload.Steps[payload.CurrentStep]
		return fmt.Sprintf("Overall task goal:\n%s\n\nCurrent step %d of %d — %s\n%s\n\nComplete only this step during this bounded execution. Do not start later steps. Report done when this step is complete.", payload.Goal, payload.CurrentStep+1, len(payload.Steps), step.Title, step.Instruction)
	}
	if resume {
		return "Continue working toward the existing task goal. Review the session context and current workspace before acting."
	}
	return payload.Goal
}

type nativeSessionMessage interface {
	GetSessionID() string
}

func consumeEvents(
	ctx context.Context,
	handle agentboot.ExecutionHandle,
	ctl task.Controller,
	runCtl task.RunController,
	onSession func(string) error,
) (*Result, error) {
	messageCount := 0
	var eventErr error
	var pause *Result
	for event := range handle.Events() {
		switch e := event.(type) {
		case agentboot.MessageEvent:
			if sessionMessage, ok := e.Raw.(nativeSessionMessage); ok && onSession != nil {
				if err := onSession(strings.TrimSpace(sessionMessage.GetSessionID())); err != nil {
					eventErr = err
					handle.Cancel()
					continue
				}
			}
			messageCount++
			_ = ctl.UpdateProgress(ctx, fmt.Sprintf("Agent working · %d events", messageCount))
		case agentboot.AskRequestEvent:
			if pause == nil {
				question := strings.TrimSpace(e.Message)
				if question == "" {
					question = strings.TrimSpace(e.Reason)
				}
				if question == "" {
					question = "The agent needs information before it can continue."
				}
				pause = &Result{State: "needs_input", Summary: "The automated run stopped for a business question.", Question: truncate(question)}
				appendRuntimeEvent(ctx, runCtl, "input_required", question, nil)
			}
			if err := handle.Respond(e.ID, agentboot.AskResponse{Approved: false, Reason: "Unattended Task runs cannot answer interactive questions"}); err != nil {
				eventErr = fmt.Errorf("agent task: reject interactive question: %w", err)
			}
			handle.Cancel()
		case agentboot.ApprovalRequestEvent:
			if pause == nil {
				tool := strings.TrimSpace(e.ToolName)
				if tool == "" {
					tool = "a protected tool"
				}
				summary := fmt.Sprintf("Native handoff required: %s requested permission outside this Task's automation boundary.", tool)
				pause = &Result{State: "handoff_required", Summary: summary, Question: "Open the native session to review the request, then continue automation when ready."}
				data, _ := json.Marshal(map[string]any{"tool": e.ToolName, "reason": e.Reason})
				appendRuntimeEvent(ctx, runCtl, "handoff_required", summary, data)
			}
			if err := handle.Respond(e.ID, agentboot.ApprovalResponse{Approved: false, Reason: "Permission is outside the Task's pre-authorized automation boundary"}); err != nil {
				eventErr = fmt.Errorf("agent task: reject approval request: %w", err)
			}
			handle.Cancel()
		case agentboot.ErrorEvent:
			if e.Err != nil {
				_ = ctl.UpdateProgress(ctx, e.Err.Error())
			}
		}
	}
	return pause, eventErr
}

func appendRuntimeEvent(ctx context.Context, ctl task.RunController, kind, summary string, data json.RawMessage) {
	if ctl == nil {
		return
	}
	_ = ctl.AppendRunEvent(ctx, task.RunEvent{
		ID: uuid.NewString(), Kind: kind, Summary: truncate(summary), Data: data, CreatedAt: time.Now(),
	})
}

func cancelAndWait(ctx context.Context, handle agentboot.ExecutionHandle, ctl task.Controller) {
	handle.Cancel()
	_, _ = consumeEvents(ctx, handle, ctl, nil, nil)
	_, _ = handle.Wait()
}

func (h *Handler) taskResult(ctx context.Context, ctl task.Controller, payload *Payload, result Result) (*task.TaskResult, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	switch result.State {
	case "needs_input":
		return &task.TaskResult{Outcome: task.OutcomeNeedsInput, Result: data}, nil
	case "handoff_required":
		return &task.TaskResult{Outcome: task.OutcomeHandoff, Result: data}, nil
	case "continue":
		if payload.FollowUp.Enabled && payload.WakeCount < payload.FollowUp.MaxWakeUps {
			delay := payload.FollowUp.DelaySeconds
			if result.SuggestedDelaySec > 0 {
				delay = result.SuggestedDelaySec
			}
			if delay < 60 {
				delay = 60
			}
			if delay > 24*60*60 {
				delay = 24 * 60 * 60
			}
			next := h.now().Add(time.Duration(delay) * time.Second)
			return &task.TaskResult{Outcome: task.OutcomeReschedule, Result: data, NextRunAt: &next}, nil
		}
		if payload.HasCurrentStep() {
			result.State = "needs_input"
			result.Question = "This step needs another run. Run now or send an instruction to continue."
			data, err = json.Marshal(result)
			if err != nil {
				return nil, err
			}
			return &task.TaskResult{Outcome: task.OutcomeNeedsInput, Result: data}, nil
		}
		return &task.TaskResult{Outcome: task.OutcomeComplete, Result: data}, nil
	case "done":
		if payload.HasCurrentStep() {
			step := payload.Steps[payload.CurrentStep]
			payload.StepOutcomes = append(payload.StepOutcomes, StepOutcome{
				StepID: step.ID, Result: result, CompletedAt: h.now(),
			})
			payload.CurrentStep++
			if err := checkpointPayload(ctx, ctl, payload); err != nil {
				return nil, err
			}
			if payload.HasCurrentStep() {
				next := h.now()
				return &task.TaskResult{Outcome: task.OutcomeReschedule, Result: data, NextRunAt: &next}, nil
			}
		}
		return &task.TaskResult{Outcome: task.OutcomeComplete, Result: data}, nil
	default:
		return nil, fmt.Errorf("agent task: invalid normalized state %q", result.State)
	}
}

func checkpointPayload(ctx context.Context, ctl task.Controller, payload *Payload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("agent task: encode payload: %w", err)
	}
	if err := ctl.UpdatePayload(ctx, data); err != nil {
		return fmt.Errorf("agent task: checkpoint payload: %w", err)
	}
	return nil
}

func parseOutcome(output string) Result {
	trimmed := strings.TrimSpace(output)
	var result Result
	if raw, ok := taggedOutcome(trimmed); ok && json.Unmarshal([]byte(raw), &result) == nil && validState(result.State) {
		result.Summary = truncate(result.Summary)
		return result
	}
	if json.Unmarshal([]byte(trimmed), &result) == nil && validState(result.State) {
		result.Summary = truncate(result.Summary)
		return result
	}
	return Result{State: "done", Summary: truncate(trimmed)}
}

func taggedOutcome(output string) (string, bool) {
	end := strings.LastIndex(output, outcomeCloseTag)
	if end < 0 {
		return "", false
	}
	start := strings.LastIndex(output[:end], outcomeOpenTag)
	if start < 0 {
		return "", false
	}
	start += len(outcomeOpenTag)
	return strings.TrimSpace(output[start:end]), true
}

func validState(state string) bool {
	return state == "done" || state == "continue" || state == "needs_input"
}

func safeArtifacts(artifacts []string) []string {
	safe := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifact = strings.TrimSpace(artifact)
		if artifact == "" || filepath.IsAbs(artifact) {
			continue
		}
		clean := filepath.Clean(artifact)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			continue
		}
		safe = append(safe, clean)
	}
	return safe
}

func truncate(value string) string {
	if utf8.RuneCountInString(value) <= maxSummaryRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxSummaryRunes]) + "…"
}
