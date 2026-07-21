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
	"github.com/tingly-dev/tingly-box/internal/task/shelltask"
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

	if len(payload.Steps) > 0 && payload.CurrentStep == len(payload.Steps) && strings.TrimSpace(payload.PendingInput) == "" {
		payload.CurrentStep = 0
		payload.StepOutcomes = nil
		payload.SessionID = ""
		if payload.Repeat != nil {
			payload.Repeat.Iteration = 0
		}
		if err := checkpointPayload(ctx, ctl, &payload); err != nil {
			return nil, err
		}
	}

	// Skip leading steps whose `when` is unmet (A1), recording a skipped
	// outcome for each so downstream conditions can see it. If nothing
	// runnable remains, finish the pipeline (repeat/until decision).
	if len(payload.Steps) > 0 {
		skipped := false
		for payload.HasCurrentStep() {
			step := payload.Steps[payload.CurrentStep]
			run, err := evalCondition(step.When, outcomesByID(payload.StepOutcomes))
			if err != nil {
				return nil, fmt.Errorf("agent task: step %d when: %w", payload.CurrentStep+1, err)
			}
			if run {
				break
			}
			payload.StepOutcomes = append(payload.StepOutcomes, StepOutcome{
				StepID: step.ID, Result: Result{State: "skipped", Summary: "Skipped: condition not met", ExitReason: "when_false"}, CompletedAt: h.now(),
			})
			payload.CurrentStep++
			payload.SessionID = ""
			skipped = true
		}
		if skipped {
			if err := checkpointPayload(ctx, ctl, &payload); err != nil {
				return nil, err
			}
		}
		if !payload.HasCurrentStep() {
			return h.finishPipeline(ctx, ctl, &payload)
		}
	}

	// A shell step runs deterministically without an agent — dispatch it
	// before requiring an agent CLI to be available (A0 heterogeneous steps).
	if payload.HasCurrentStep() && payload.Steps[payload.CurrentStep].IsShell() {
		return h.runShellStep(ctx, ctl, &payload)
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
	execution := payload.Execution
	if payload.PendingExecution != nil {
		execution = *payload.PendingExecution
	}
	execution = execution.Automated(payload.Agent)
	if payload.Agent == AgentClaude && !resume {
		sessionID = uuid.NewString()
	}

	prompt := nextPrompt(payload, resume)
	runCtl, _ := ctl.(task.RunController)
	var env []string
	if h.envResolver != nil {
		resolved, err := h.envResolver(ctx, payload.Agent)
		if err != nil {
			return nil, fmt.Errorf("agent task: resolve environment: %w", err)
		}
		env = resolved
	}
	if payload.Agent == AgentClaude {
		// Attribute this run's gateway traffic to the task/run so usage
		// tracking can aggregate per-run token cost (design §6.6). The CLI
		// forwards these on every request via ANTHROPIC_CUSTOM_HEADERS.
		attribution := "X-Tingly-Task-Id: " + t.ID
		if runCtl != nil {
			attribution += "\nX-Tingly-Run-Id: " + runCtl.RunID()
		}
		env = append(env, "ANTHROPIC_CUSTOM_HEADERS="+attribution)
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
	appendRuntimeEvent(ctx, runCtl, "run_started", fmt.Sprintf("Started unattended %s run", payload.Agent), eventData(map[string]any{
		"launch_profile": execution.LaunchProfile, "tools": execution.Tools,
	}))

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
			cancelAndWait(handle)
			return nil, err
		}
	}

	prompter := &pausingPrompter{
		cancel: handle.Cancel,
		record: func(kind, summary string, data json.RawMessage) {
			appendRuntimeEvent(ctx, runCtl, kind, summary, data)
		},
	}
	messageCount := 0
	var sinkErr error
	sink := func(raw any) {
		if ev, ok := raw.(agentboot.ErrorEvent); ok {
			if ev.Err != nil {
				_ = ctl.UpdateProgress(ctx, ev.Err.Error())
				appendRuntimeEvent(ctx, runCtl, "runtime_error", ev.Err.Error(), nil)
			}
			return
		}
		if sinkErr != nil {
			return
		}
		if sessionMessage, ok := raw.(nativeSessionMessage); ok {
			if id := strings.TrimSpace(sessionMessage.GetSessionID()); id != "" && id != payload.SessionID {
				payload.SessionID = id
				appendRuntimeEvent(ctx, runCtl, "session_discovered", "Native session checkpointed", eventData(map[string]any{"session_id": id}))
				if err := checkpointPayload(ctx, ctl, &payload); err != nil {
					sinkErr = err
					handle.Cancel()
					return
				}
			}
		}
		messageCount++
		recordNativeMessage(ctx, runCtl, raw)
		_ = ctl.UpdateProgress(ctx, fmt.Sprintf("Agent working · %d events", messageCount))
	}
	agentResult, waitErr := agentboot.RunWithPrompter(ctx, handle, prompter, sink)
	if sinkErr != nil {
		return nil, sinkErr
	}
	runtimePause := prompter.pause
	if runtimePause == nil {
		runtimePause = pauseFromPermissionDenials(agentResult)
		if runtimePause != nil {
			kind := "handoff_required"
			if runtimePause.State == "needs_input" {
				kind = "input_required"
			}
			appendRuntimeEvent(ctx, runCtl, kind, runtimePause.Summary, eventData(runtimePause))
		}
	}
	if runtimePause != nil {
		runtimePause.NativeSessionID = payload.SessionID
		if agentResult != nil {
			runtimePause.DurationMS = agentResult.Duration.Milliseconds()
		}
		payload.WakeCount++
		if err := checkpointPayload(ctx, ctl, &payload); err != nil {
			return nil, err
		}
		appendRuntimeEvent(ctx, runCtl, "outcome", runtimePause.Summary, eventData(runtimePause))
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
	normalized.ExitCode = &agentResult.ExitCode
	normalized.DurationMS = agentResult.Duration.Milliseconds()
	if normalized.ExitReason == "" {
		normalized.ExitReason = normalized.State
	}
	normalized.Artifacts = safeArtifacts(normalized.Artifacts)
	appendRuntimeEvent(ctx, runCtl, "outcome", normalized.Summary, eventData(normalized))
	return h.taskResult(ctx, ctl, &payload, normalized)
}

func nextPrompt(payload Payload, resume bool) string {
	if strings.TrimSpace(payload.PendingInput) != "" {
		if payload.HasCurrentStep() {
			step := payload.Steps[payload.CurrentStep]
			return appendRunContext(payload.Goal, fmt.Sprintf("Current step %d of %d — %s\n%s\n\nUser instruction for this step:\n%s\n\nContinue only this step. Do not start later steps.%s", payload.CurrentStep+1, len(payload.Steps), step.Title, step.Instruction, payload.PendingInput, completedStepContext(payload)))
		}
		return appendRunContext(payload.Goal, fmt.Sprintf("Additional instruction for this run:\n%s\n\nContinue working toward the task goal using this instruction. Report the bounded run outcome when finished.", payload.PendingInput))
	}
	if payload.HasCurrentStep() {
		step := payload.Steps[payload.CurrentStep]
		return appendRunContext(payload.Goal, fmt.Sprintf("Current step %d of %d — %s\n%s\n\nComplete only this step during this bounded execution. Do not start later steps. Report done when this step is complete.%s", payload.CurrentStep+1, len(payload.Steps), step.Title, step.Instruction, completedStepContext(payload)))
	}
	if resume {
		return appendRunContext(payload.Goal, "Continue working toward the task goal. Review the session context and current workspace before acting.")
	}
	return payload.Goal
}

func appendRunContext(goal, context string) string {
	return goal + "\n\n---\n\nTingly Box run context:\n" + context
}

// completedStepContext summarizes finished steps for the current step's
// fresh native session. State crosses step boundaries only through the
// workspace and these structured summaries, never through a shared session.
func completedStepContext(p Payload) string {
	if p.CurrentStep == 0 || len(p.StepOutcomes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nCompleted steps so far (each ran in its own session; their file changes are already in this workspace):\n")
	for i, outcome := range p.StepOutcomes {
		if i >= p.CurrentStep || i >= len(p.Steps) {
			break
		}
		b.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, p.Steps[i].Title, clipRunes(outcome.Result.Summary, 300)))
	}
	return b.String()
}

func clipRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

type nativeSessionMessage interface {
	GetSessionID() string
}

func appendRuntimeEvent(ctx context.Context, ctl task.RunController, kind, summary string, data json.RawMessage) {
	if ctl == nil {
		return
	}
	_ = ctl.AppendRunEvent(ctx, task.RunEvent{
		ID: uuid.NewString(), Kind: kind, Summary: truncate(summary), Data: data, CreatedAt: time.Now(),
	})
}

func cancelAndWait(handle agentboot.ExecutionHandle) {
	handle.Cancel()
	for range handle.Events() {
	}
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
		// The agent reports more work remains but no automatic wake-up is
		// available (follow-up disabled or wake budget spent): pause for a
		// human decision instead of silently completing.
		result.State = "needs_input"
		if strings.TrimSpace(result.Question) == "" {
			result.Question = "The agent reports more work remains, but automatic follow-up is unavailable. Run now, send an instruction, or stop the task."
		}
		if payload.FollowUp.Enabled {
			result.ExitReason = "follow_up_exhausted"
		} else {
			result.ExitReason = "follow_up_disabled"
		}
		data, err = json.Marshal(result)
		if err != nil {
			return nil, err
		}
		return &task.TaskResult{Outcome: task.OutcomeNeedsInput, Result: data}, nil
	case "done", "failed":
		// In a stepped pipeline both done and failed record the step outcome
		// and advance the cursor; a downstream `when` step or the repeat
		// `until` reacts to a failure (A1). Only a free-goal (no-steps) task
		// completes directly on "done".
		if payload.HasCurrentStep() {
			step := payload.Steps[payload.CurrentStep]
			payload.StepOutcomes = append(payload.StepOutcomes, StepOutcome{
				StepID: step.ID, Result: result, CompletedAt: h.now(),
			})
			payload.CurrentStep++
			// Session continuity ends at the step boundary: the next step
			// starts a fresh native session and receives prior-step state
			// via the workspace plus completedStepContext summaries.
			payload.SessionID = ""
			if err := checkpointPayload(ctx, ctl, payload); err != nil {
				return nil, err
			}
			if payload.HasCurrentStep() {
				next := h.now()
				return &task.TaskResult{Outcome: task.OutcomeReschedule, Result: data, NextRunAt: &next}, nil
			}
			return h.finishPipeline(ctx, ctl, payload)
		}
		if result.State == "failed" {
			return nil, fmt.Errorf("agent task: run failed: %s", clipRunes(result.Summary, 300))
		}
		return &task.TaskResult{Outcome: task.OutcomeComplete, Result: data}, nil
	default:
		return nil, fmt.Errorf("agent task: invalid normalized state %q", result.State)
	}
}

// finishPipeline is called when the step cursor reaches the end. It applies
// the repeat policy: satisfied `until` (or no repeat) completes; an unmet
// `until` with iterations left restarts the pipeline; an unmet `until` at the
// iteration cap pauses for a human. Without a repeat, the pipeline succeeds
// unless its last step failed.
func (h *Handler) finishPipeline(ctx context.Context, ctl task.Controller, payload *Payload) (*task.TaskResult, error) {
	outcomes := payload.StepOutcomes
	final := Result{State: "done", Summary: "Pipeline complete"}
	if n := len(outcomes); n > 0 {
		final = outcomes[n-1].Result
	}

	if payload.Repeat != nil && strings.TrimSpace(payload.Repeat.Until) != "" {
		met, err := evalCondition(payload.Repeat.Until, outcomesByID(outcomes))
		if err != nil {
			return nil, fmt.Errorf("agent task: repeat until: %w", err)
		}
		maxIter := payload.Repeat.Max
		if maxIter <= 0 {
			maxIter = 1
		}
		if met {
			final.State = "done"
			data, _ := json.Marshal(final)
			return &task.TaskResult{Outcome: task.OutcomeComplete, Result: data}, nil
		}
		if payload.Repeat.Iteration+1 < maxIter {
			payload.Repeat.Iteration++
			payload.CurrentStep = 0
			payload.StepOutcomes = nil
			payload.SessionID = ""
			if err := checkpointPayload(ctx, ctl, payload); err != nil {
				return nil, err
			}
			next := h.now()
			data, _ := json.Marshal(final)
			return &task.TaskResult{Outcome: task.OutcomeReschedule, Result: data, NextRunAt: &next}, nil
		}
		// Hit the iteration cap without meeting the condition: pause for a
		// human rather than silently completing an unmet loop.
		final.State = "needs_input"
		final.Question = fmt.Sprintf("Repeated %d× without meeting the exit condition (%s). Review and decide.", maxIter, payload.Repeat.Until)
		final.ExitReason = "repeat_exhausted"
		data, _ := json.Marshal(final)
		return &task.TaskResult{Outcome: task.OutcomeNeedsInput, Result: data}, nil
	}

	// No repeat: the last step's outcome is the pipeline's verdict.
	if n := len(outcomes); n > 0 && outcomeFailed(outcomes[n-1]) {
		return nil, fmt.Errorf("agent task: pipeline ended with a failed step: %s", clipRunes(final.Summary, 300))
	}
	final.State = "done"
	data, _ := json.Marshal(final)
	return &task.TaskResult{Outcome: task.OutcomeComplete, Result: data}, nil
}

// runShellStep executes the current shell step deterministically (no agent,
// no native session) and advances the cursor via taskResult.
func (h *Handler) runShellStep(ctx context.Context, ctl task.Controller, payload *Payload) (*task.TaskResult, error) {
	step := payload.Steps[payload.CurrentStep]
	runCtl, _ := ctl.(task.RunController)
	appendRuntimeEvent(ctx, runCtl, "run_started", fmt.Sprintf("Shell step %d of %d — %s", payload.CurrentStep+1, len(payload.Steps), step.Title), eventData(map[string]any{"executor": "shell"}))
	_ = ctl.UpdateProgress(ctx, "Shell step running")

	shellResult, err := shelltask.RunOnce(ctx, shelltask.RunSpec{
		Command:        step.Command,
		WorkspacePath:  payload.WorkspacePath,
		TimeoutSeconds: payload.TimeoutSeconds,
	}, func(kind, summary string, data json.RawMessage) {
		appendRuntimeEvent(ctx, runCtl, kind, summary, data)
	})
	if err != nil {
		return nil, fmt.Errorf("agent task: shell step %q: %w", step.Title, err)
	}
	// A shell step owns no native session; keep the step boundary fresh.
	payload.SessionID = ""
	if strings.TrimSpace(payload.PendingInput) != "" {
		payload.PendingInput = ""
	}
	result := Result{
		State: shellResult.State, Summary: shellResult.Summary, Artifacts: shellResult.Artifacts,
		ExitCode: shellResult.ExitCode, DurationMS: shellResult.DurationMS, ExitReason: shellResult.ExitReason,
	}
	appendRuntimeEvent(ctx, runCtl, "outcome", result.Summary, eventData(result))
	return h.taskResult(ctx, ctl, payload, result)
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
	// An unreported outcome never means done: pause for a human decision
	// instead of silently marking the task complete.
	return Result{
		State:      "needs_input",
		Summary:    truncate(trimmed),
		Question:   "The run ended without a machine-readable outcome. Review the summary, then run again or send an instruction.",
		ExitReason: "outcome_unreported",
	}
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
