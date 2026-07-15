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
	return &Handler{agents: registered, envResolver: envResolver, now: time.Now}
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

	resume := payload.SessionID != ""
	sessionID := payload.SessionID
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
		ProjectPath:          payload.WorkspacePath,
		OutputFormat:         agentboot.OutputFormatStreamJSON,
		Timeout:              time.Duration(payload.TimeoutSeconds) * time.Second,
		Env:                  env,
		SessionID:            sessionID,
		Resume:               resume,
		AppendSystemPrompt:   outcomeSystemAppendix,
		PermissionPromptTool: "stdio",
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
	if checkpointAfterStart {
		if err := checkpointPayload(ctx, ctl, &payload); err != nil {
			cancelAndWait(ctx, handle, ctl)
			return nil, err
		}
	}

	pending, eventErr := consumeEvents(ctx, handle, ctl, func(nativeSessionID string) error {
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
	if pending != nil {
		return h.needsInputResult(ctx, ctl, &payload, *pending)
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
	return h.taskResult(payload, normalized)
}

func nextPrompt(payload Payload, resume bool) string {
	if strings.TrimSpace(payload.PendingInput) != "" {
		return payload.PendingInput
	}
	if resume {
		return "Continue working toward the existing task goal. Review the session context and current workspace before acting."
	}
	return payload.Goal
}

type pendingInput struct {
	Question string
}

type nativeSessionMessage interface {
	GetSessionID() string
}

func consumeEvents(
	ctx context.Context,
	handle agentboot.ExecutionHandle,
	ctl task.Controller,
	onSession func(string) error,
) (*pendingInput, error) {
	messageCount := 0
	var pending *pendingInput
	var eventErr error
	for event := range handle.Events() {
		switch e := event.(type) {
		case agentboot.MessageEvent:
			if pending != nil {
				continue
			}
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
			question := strings.TrimSpace(e.Message)
			if question == "" {
				question = marshalQuestion(e.Input)
			}
			_ = handle.Respond(e.ID, agentboot.AskResponse{Approved: false, Reason: "Task paused for user input"})
			handle.Cancel()
			if pending == nil {
				pending = &pendingInput{Question: question}
			}
		case agentboot.ApprovalRequestEvent:
			question := fmt.Sprintf("Approval required for tool %s: %s", e.ToolName, marshalQuestion(e.Input))
			_ = handle.Respond(e.ID, agentboot.ApprovalResponse{Approved: false, Reason: "Task paused for user approval"})
			handle.Cancel()
			if pending == nil {
				pending = &pendingInput{Question: question}
			}
		case agentboot.ErrorEvent:
			if pending == nil && e.Err != nil {
				_ = ctl.UpdateProgress(ctx, e.Err.Error())
			}
		}
	}
	return pending, eventErr
}

func cancelAndWait(ctx context.Context, handle agentboot.ExecutionHandle, ctl task.Controller) {
	handle.Cancel()
	_, _ = consumeEvents(ctx, handle, ctl, nil)
	_, _ = handle.Wait()
}

func marshalQuestion(input map[string]any) string {
	if len(input) == 0 {
		return "Agent requires user input"
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "Agent requires user input"
	}
	return string(data)
}

func (h *Handler) needsInputResult(ctx context.Context, ctl task.Controller, payload *Payload, pending pendingInput) (*task.TaskResult, error) {
	payload.WakeCount++
	if err := checkpointPayload(ctx, ctl, payload); err != nil {
		return nil, err
	}
	result := Result{
		State:           "needs_input",
		Summary:         "Task paused for user input",
		Question:        pending.Question,
		NativeSessionID: payload.SessionID,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &task.TaskResult{Outcome: task.OutcomeNeedsInput, Result: data}, nil
}

func (h *Handler) taskResult(payload Payload, result Result) (*task.TaskResult, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	switch result.State {
	case "needs_input":
		return &task.TaskResult{Outcome: task.OutcomeNeedsInput, Result: data}, nil
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
		return &task.TaskResult{Outcome: task.OutcomeComplete, Result: data}, nil
	case "done":
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
