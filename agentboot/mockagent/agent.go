package mock

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// mockToolNames are the tool names used by the linear default script.
var mockToolNames = []string{
	"mock_tool_read",
	"mock_tool_write",
	"mock_tool_execute",
	"mock_tool_search",
	"mock_tool_analyze",
}

// Agent is a scriptable mock agentboot.Agent for tests.
//
// Two ways to drive it:
//
//   - Pass Config{Script: ...} to declare an explicit event sequence.
//   - Pass legacy Config{MaxIterations, AskUserQuestionFrequency, ...} to
//     fall back to the linear default script.
type Agent struct {
	config        Config
	abConfig      agentboot.Config
	defaultFormat agentboot.OutputFormat
	mu            sync.RWMutex
}

// NewAgent creates a new mock agent with the given configuration.
func NewAgent(config Config) *Agent {
	config = config.Merge(DefaultConfig())
	return &Agent{
		config:        config,
		defaultFormat: agentboot.OutputFormatStreamJSON,
	}
}

// NewAgentWithConfig creates a new mock agent with both mock and agentboot configs.
func NewAgentWithConfig(mockConfig Config, abConfig agentboot.Config) *Agent {
	a := NewAgent(mockConfig)
	a.abConfig = abConfig
	return a
}

// Execute plays the agent's script (explicit or linear-default) and returns
// the captured event stream.
func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.Result, error) {
	startTime := time.Now()
	logrus.Infof("[MockAgent] Starting execution with prompt: %s", truncatePrompt(prompt))

	a.mu.RLock()
	cfg := a.config
	a.mu.RUnlock()

	sessionID := opts.SessionID
	if sessionID == "" {
		sessionID = uuid.NewString()[:8]
	}

	script := cfg.Script
	if len(script) == 0 {
		script = defaultLinearScript(cfg)
	}

	var events []agentboot.Event
	state := &runState{
		agentType:  agentboot.AgentTypeMockAgent,
		sessionID:  sessionID,
		prompt:     prompt,
		handler:    opts.Handler,
		opts:       opts,
		cfg:        cfg,
		events:     &events,
		totalSteps: cfg.MaxIterations,
	}

	if err := state.emit(agentboot.NewInitMessage(state.agentType, sessionID, cfg.MaxIterations)); err != nil {
		logrus.WithError(err).Warn("[MockAgent] init handler error")
	}

	for i, step := range script {
		select {
		case <-ctx.Done():
			return a.finishCancelled(state, startTime, ctx.Err()), ctx.Err()
		default:
		}
		state.stepIdx = i + 1
		if err := step.play(ctx, state); err != nil {
			return a.finishWithError(state, startTime, err), err
		}
		if state.halt {
			break
		}
		if cfg.StepDelay > 0 && i < len(script)-1 {
			select {
			case <-time.After(cfg.StepDelay):
			case <-ctx.Done():
				return a.finishCancelled(state, startTime, ctx.Err()), ctx.Err()
			}
		}
	}

	if !hasFinalResult(events) {
		_ = state.emit(agentboot.NewResultMessage(
			state.agentType, sessionID, "success", "Mock agent completed all iterations",
		))
	}

	if state.handler != nil {
		successTerm := isSuccessTerminal(events)
		completion := &agentboot.CompletionResult{
			Success:    successTerm && len(state.mismatches) == 0,
			DurationMS: time.Since(startTime).Milliseconds(),
			SessionID:  sessionID,
		}
		if !successTerm {
			completion.Error = lastResultMessage(events)
		} else if len(state.mismatches) > 0 {
			completion.Error = state.mismatches[0]
		}
		state.handler.OnComplete(completion)
	}

	return a.buildResult(events, startTime, sessionID, state.mismatches), nil
}

func (a *Agent) finishCancelled(state *runState, startTime time.Time, err error) *agentboot.Result {
	if !hasFinalResult(*state.events) {
		_ = state.emit(agentboot.NewResultMessage(
			state.agentType, state.sessionID, "cancelled", "Context cancelled by user",
		))
	}
	if state.handler != nil {
		state.handler.OnComplete(&agentboot.CompletionResult{
			Success:    false,
			DurationMS: time.Since(startTime).Milliseconds(),
			SessionID:  state.sessionID,
			Error:      err.Error(),
		})
	}
	return a.buildResult(*state.events, startTime, state.sessionID, state.mismatches)
}

func (a *Agent) finishWithError(state *runState, startTime time.Time, err error) *agentboot.Result {
	if !hasFinalResult(*state.events) {
		_ = state.emit(agentboot.NewResultMessage(
			state.agentType, state.sessionID, "error", err.Error(),
		))
	}
	if state.handler != nil {
		state.handler.OnComplete(&agentboot.CompletionResult{
			Success:    false,
			DurationMS: time.Since(startTime).Milliseconds(),
			SessionID:  state.sessionID,
			Error:      err.Error(),
		})
	}
	return a.buildResult(*state.events, startTime, state.sessionID, state.mismatches)
}

func (a *Agent) buildResult(events []agentboot.Event, startTime time.Time, sessionID string, mismatches []string) *agentboot.Result {
	res := &agentboot.Result{
		Output:   "",
		ExitCode: 0,
		Duration: time.Since(startTime),
		Format:   a.defaultFormat,
		Events:   events,
		Metadata: map[string]interface{}{
			"session_id": sessionID,
			"agent_type": "mock",
		},
	}
	if len(mismatches) > 0 {
		res.Error = mismatches[0]
		res.Metadata["expectation_failures"] = mismatches
	}
	return res
}

func hasFinalResult(events []agentboot.Event) bool {
	for _, e := range events {
		if e.Type == agentboot.EventTypeResult {
			return true
		}
	}
	return false
}

func isSuccessTerminal(events []agentboot.Event) bool {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == agentboot.EventTypeResult {
			if status, _ := events[i].Data["status"].(string); status == "success" {
				return true
			}
			return false
		}
	}
	return false
}

func lastResultMessage(events []agentboot.Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == agentboot.EventTypeResult {
			if msg, _ := events[i].Data["message"].(string); msg != "" {
				return msg
			}
			if st, _ := events[i].Data["status"].(string); st != "" {
				return st
			}
		}
	}
	return ""
}

// IsAvailable always returns true for the mock agent.
func (a *Agent) IsAvailable() bool { return true }

// Type returns the mock agent type.
func (a *Agent) Type() agentboot.AgentType { return agentboot.AgentTypeMockAgent }

// SetDefaultFormat sets the default output format.
func (a *Agent) SetDefaultFormat(format agentboot.OutputFormat) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.defaultFormat = format
}

// GetDefaultFormat returns the current default format.
func (a *Agent) GetDefaultFormat() agentboot.OutputFormat {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.defaultFormat
}

// SetMaxIterations updates the linear-default iteration count.
func (a *Agent) SetMaxIterations(max int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if max > 0 {
		a.config.MaxIterations = max
	}
}

// SetStepDelay updates the inter-step delay.
func (a *Agent) SetStepDelay(delay time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if delay > 0 {
		a.config.StepDelay = delay
	}
}

// SetAutoApprove toggles auto-approval mode.
func (a *Agent) SetAutoApprove(autoApprove bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.AutoApprove = autoApprove
}

// SetAskUserQuestionFrequency configures how often the linear-default script
// emits AskUserQuestion (every N steps).
func (a *Agent) SetAskUserQuestionFrequency(freq int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if freq > 0 {
		a.config.AskUserQuestionFrequency = freq
	}
}

// SetScript replaces the agent's script with the provided steps. Pass nil or
// an empty slice to revert to the linear default.
func (a *Agent) SetScript(steps []Step) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.Script = steps
}

// truncatePrompt truncates a prompt for display purposes.
func truncatePrompt(prompt string) string {
	const maxLen = 50
	if len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen-3] + "..."
}
