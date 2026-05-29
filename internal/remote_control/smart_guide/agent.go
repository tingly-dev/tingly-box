package smart_guide

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/afk"
)

// AgentType constants. Defined here (not in agentboot core) so Smart Guide can
// extend the agent-type space without modifying agentboot.
const (
	AgentTypeTinglyBox  = "tingly-box" // @tb
	AgentTypeClaudeCode = "claude"     // @cc
	AgentTypeMock       = "mock"
)

// TinglyBoxAgent is the Smart Guide agent (@tb). It runs an in-house ReAct loop
// (internal/afk.Engine) on the official Anthropic SDK, replacing the former
// tingly-agentscope runtime.
type TinglyBoxAgent struct {
	engine   *afk.Engine
	config   *SmartGuideConfig
	executor *ToolExecutor

	// history is the conversation as native Anthropic beta message params,
	// loaded from the session store and updated after each run.
	history []anthropic.BetaMessageParam
}

// AgentConfig holds the configuration for creating a TinglyBoxAgent.
type AgentConfig struct {
	SmartGuideConfig *SmartGuideConfig
	ToolExecutor     *ToolExecutor

	// HTTP endpoint configuration (resolved from TBClient by caller).
	BaseURL string
	APIKey  string
	Model   string

	// Callback functions for internal tools.
	GetStatusFunc     func(chatID string) (*StatusInfo, error)
	GetProjectFunc    func(chatID string) (string, bool, error)
	UpdateProjectFunc func(chatID string, projectPath string) error

	// Approval context for non-allowlisted commands.
	Approver Approver
	ChatID   string
	Platform string
	BotUUID  string

	// ToolContext for file send capability and cross-path approval. If nil,
	// the send_file tool is not registered.
	ToolCtx *ToolContext
}

// NewTinglyBoxAgent creates a new Smart Guide agent.
func NewTinglyBoxAgent(config *AgentConfig) (*TinglyBoxAgent, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.SmartGuideConfig == nil {
		config.SmartGuideConfig = DefaultSmartGuideConfig()
	}
	if config.Model == "" {
		return nil, fmt.Errorf("smartguide_provider and smartguide_model are required in bot setting")
	}
	if config.BaseURL == "" || config.APIKey == "" {
		return nil, fmt.Errorf("BaseURL and APIKey are required in config")
	}

	executor := config.ToolExecutor
	if executor == nil {
		executor = NewToolExecutor([]string{"cd", "ls", "pwd"})
	}

	tb := &TinglyBoxAgent{
		config:   config.SmartGuideConfig,
		executor: executor,
	}

	// Wire the bash approval callback before building tools so the executor
	// gates non-allowlisted commands through the approver.
	if config.Approver != nil {
		executor.SetApprovalCallback(tb.createApprovalCallback(config))
		logrus.WithField("chatID", config.ChatID).Info("Approval callback configured for ToolExecutor")
	} else {
		logrus.WithField("chatID", config.ChatID).Warn("No approver provided - non-whitelisted commands will be denied")
	}

	tools := BuildTools(executor, config.ChatID, config.GetStatusFunc, config.UpdateProjectFunc, config.ToolCtx)

	engine, err := afk.NewEngine(afk.Config{
		BaseURL:       config.BaseURL,
		APIKey:        config.APIKey,
		Model:         config.Model,
		System:        config.SmartGuideConfig.GetSystemPrompt(),
		Temperature:   &config.SmartGuideConfig.Temperature,
		MaxIterations: config.SmartGuideConfig.MaxIterations,
		Tools:         tools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build anthropic engine: %w", err)
	}
	tb.engine = engine

	logrus.WithFields(logrus.Fields{
		"model":    config.Model,
		"endpoint": config.BaseURL,
	}).Info("Created SmartGuide agent (anthropic engine)")

	return tb, nil
}

// NewTinglyBoxAgentWithSession creates a Smart Guide agent seeded with prior
// conversation history (native Anthropic beta message params from the session store).
func NewTinglyBoxAgentWithSession(config *AgentConfig, history []anthropic.BetaMessageParam) (*TinglyBoxAgent, error) {
	tb, err := NewTinglyBoxAgent(config)
	if err != nil {
		return nil, err
	}
	tb.history = history
	logrus.WithField("msgCount", len(history)).Info("Loaded conversation history into SmartGuide agent")
	return tb, nil
}

// createApprovalCallback builds the bash approval callback that routes
// non-allowlisted commands to the configured Approver via IMPrompter.
func (a *TinglyBoxAgent) createApprovalCallback(config *AgentConfig) func(context.Context, ApprovalRequest) (bool, error) {
	return func(ctx context.Context, req ApprovalRequest) (bool, error) {
		permReq := agentboot.ApprovalRequestEvent{
			ID:        uuid.New().String(),
			AgentType: AgentTypeTinglyBox,
			ToolName:  "bash",
			Input: map[string]interface{}{
				"command": req.Command,
				"args":    req.Args,
			},
			Reason:    req.Reason,
			SessionID: config.ChatID,
			BotUUID:   config.BotUUID,
			ChatID:    config.ChatID,
			Platform:  config.Platform,
		}
		result, err := config.Approver.OnApproval(ctx, permReq)
		if err != nil {
			logrus.WithError(err).WithField("command", req.Command).Error("Approval request failed")
			return false, err
		}
		return result.Approved, nil
	}
}

// engineSink adapts the engine's StreamSink to the smart-guide StreamHandler,
// forwarding assistant text and tool activity as the map-shaped messages the
// bot streaming layer already understands. It also counts what it forwarded so
// ExecuteWithHandler can log (and diagnose) tool-only runs.
type engineSink struct {
	handler      StreamHandler
	iteration    int
	textMessages int
	toolCalls    int
}

func (s *engineSink) OnText(delta string) {
	if delta == "" {
		return
	}
	s.textMessages++
	logrus.WithFields(logrus.Fields{
		"iteration": s.iteration,
		"text_len":  len(delta),
	}).Debug("SmartGuide sink: assistant text")
	if s.handler == nil {
		return
	}
	s.handler.OnMessage(map[string]interface{}{
		"type":      "assistant",
		"message":   delta,
		"iteration": s.iteration,
	})
}

func (s *engineSink) OnToolCall(name string, input json.RawMessage) {
	s.iteration++
	s.toolCalls++
	logrus.WithFields(logrus.Fields{
		"iteration": s.iteration,
		"tool":      name,
	}).Debug("SmartGuide sink: tool call")
	if s.handler == nil {
		return
	}
	s.handler.OnMessage(map[string]interface{}{
		"type":  "tool_use",
		"name":  name,
		"input": string(input),
	})
}

func (s *engineSink) OnToolResult(name string, result string, isErr bool) {
	logrus.WithFields(logrus.Fields{
		"tool":       name,
		"is_error":   isErr,
		"result_len": len(result),
	}).Debug("SmartGuide sink: tool result")
	if s.handler == nil {
		return
	}
	s.handler.OnMessage(map[string]interface{}{
		"type":     "tool_result",
		"name":     name,
		"result":   result,
		"is_error": isErr,
	})
}

// ExecuteWithHandler runs one user turn through the ReAct engine, streaming
// intermediate output to the handler and reporting completion. It returns an
// agentboot.Result for compatibility with the executor layer.
func (a *TinglyBoxAgent) ExecuteWithHandler(
	ctx context.Context,
	prompt string,
	toolCtx *ToolContext,
	handler StreamHandler,
) (*agentboot.Result, error) {
	startTime := time.Now()
	result := &agentboot.Result{
		Format:   agentboot.OutputFormatText,
		Metadata: make(map[string]interface{}),
	}

	if handler != nil {
		handler.OnMessage(map[string]interface{}{
			"type":    "status",
			"status":  "processing",
			"message": "Smart Guide is thinking...",
		})
	}

	sink := &engineSink{handler: handler}
	messages, finalText, err := a.engine.Run(ctx, a.history, prompt, sink)
	duration := time.Since(startTime)

	// Persist whatever was produced so a mid-run cancel still advances history.
	a.history = messages

	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
		result.Duration = duration
		if handler != nil {
			handler.OnError(err)
			handler.OnComplete(&CompletionResult{
				Success:    false,
				DurationMS: duration.Milliseconds(),
				Error:      err.Error(),
				SessionID:  toolCtx.SessionID,
			})
		}
		logrus.WithError(err).WithField("session_id", toolCtx.SessionID).Error("SmartGuide execution failed")
		return result, err
	}

	result.Output = finalText
	result.ExitCode = 0
	result.Duration = duration

	if handler != nil {
		handler.OnComplete(&CompletionResult{
			Success:    true,
			DurationMS: duration.Milliseconds(),
			SessionID:  toolCtx.SessionID,
		})
	}

	logEntry := logrus.WithFields(logrus.Fields{
		"duration_ms":   duration.Milliseconds(),
		"session_id":    toolCtx.SessionID,
		"final_len":     len(finalText),
		"history_msgs":  len(messages),
		"messages_sent": sink.textMessages,
		"tool_calls":    sink.toolCalls,
	})
	// A run that produced tool calls but no final text leaves the user with no
	// visible reply — surface it at WARN so it is greppable, not buried.
	if finalText == "" {
		logEntry.Warn("SmartGuide execution completed with NO assistant text (tool calls only)")
	} else {
		logEntry.Info("SmartGuide execution completed")
	}
	return result, nil
}

// History returns the agent's current conversation history (native beta params).
func (a *TinglyBoxAgent) History() []anthropic.BetaMessageParam {
	return a.history
}

// LastAssistantText returns the text of the most recent assistant message in
// history, used by the completion callback to capture the final response.
func (a *TinglyBoxAgent) LastAssistantText() string {
	for i := len(a.history) - 1; i >= 0; i-- {
		m := a.history[i]
		if m.Role != anthropic.BetaMessageParamRoleAssistant {
			continue
		}
		var text string
		for _, block := range m.Content {
			if block.OfText != nil {
				text += block.OfText.Text
			}
		}
		if text != "" {
			return text
		}
	}
	return ""
}

// GetGreeting returns the default greeting for new users.
func (a *TinglyBoxAgent) GetGreeting() string {
	return DefaultGreeting()
}

// GetExecutor returns the tool executor.
func (a *TinglyBoxAgent) GetExecutor() *ToolExecutor {
	return a.executor
}

// IsEnabled returns whether the smart guide is enabled.
func (a *TinglyBoxAgent) IsEnabled() bool {
	return a.config != nil && a.config.Enabled
}

// GetConfig returns the agent's configuration.
func (a *TinglyBoxAgent) GetConfig() *SmartGuideConfig {
	return a.config
}

// CanCreateAgent reports whether a SmartGuide agent can be created with the
// given configuration.
func CanCreateAgent(baseURL, apiKey, smartGuideProvider, smartGuideModel string) bool {
	if smartGuideProvider == "" || smartGuideModel == "" {
		return false
	}
	if baseURL == "" || apiKey == "" {
		return false
	}
	return true
}

// IsAvailable returns true if the agent is available for execution.
func (a *TinglyBoxAgent) IsAvailable() bool {
	return a.IsEnabled()
}

// Type returns the agent type for the agentboot.Agent interface.
func (a *TinglyBoxAgent) Type() agentboot.AgentType {
	return AgentTypeTinglyBox
}

// SetDefaultFormat is a no-op (SmartGuide always uses text format).
func (a *TinglyBoxAgent) SetDefaultFormat(format agentboot.OutputFormat) {}

// GetDefaultFormat returns the current default format.
func (a *TinglyBoxAgent) GetDefaultFormat() agentboot.OutputFormat {
	return agentboot.OutputFormatText
}
