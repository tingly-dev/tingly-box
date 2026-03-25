package smart_guide

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/agent"
	"github.com/tingly-dev/tingly-agentscope/pkg/memory"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/model/anthropic"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// AgentType constants
// AgentTypeTinglyBox is the agent type identifier for SmartGuide (TB) agent
// Defined here to allow external extension without modifying agentboot core types
const (
	AgentTypeTinglyBox  = "tingly-box" // @tb
	AgentTypeClaudeCode = "claude"     // @cc
	AgentTypeMock       = "mock"
)

// Summary prompt template
// Provides a concise, user-friendly summary of what was accomplished
const summaryPrompt = `You are providing a brief task summary to the user.

Based on the conversation history, provide a concise summary (2-3 sentences max) that includes:
1. What was accomplished
2. Key actions taken (commands executed, tools used, files modified)
3. Current state or next step (if applicable)

Keep it brief and informative. Use plain text or simple markdown. Focus on outcomes.

Examples:
- "I've listed the files in the current directory. Found 3 files: main.go, go.mod, and README.md."
- "Cloned the repository successfully. You're now in the project directory. Type @cc to start coding."
- "Updated the port from 3000 to 8080 in config.json. The change has been saved."
`

// TinglyBoxAgent is the smart guide agent (@tb)
type TinglyBoxAgent struct {
	*agent.ReActAgent
	config   *SmartGuideConfig
	executor *ToolExecutor
	toolkit  *tool.Toolkit
}

// AgentConfig holds the configuration for creating a TinglyBoxAgent
type AgentConfig struct {
	SmartGuideConfig *SmartGuideConfig
	// HTTP endpoint configuration (resolved from TBClient by caller)
	BaseURL      string // e.g., "http://localhost:12580/tingly/_smart_guide"
	APIKey       string // Tingly-box authentication token
	ToolExecutor *ToolExecutor
	// SmartGuide model configuration (required from bot setting)
	Provider string // Provider UUID
	Model    string // Model identifier
	// Callback functions for internal tools
	GetStatusFunc     func(chatID string) (*StatusInfo, error)
	GetProjectFunc    func(chatID string) (string, bool, error)
	UpdateProjectFunc func(chatID string, projectPath string) error // Updates project path in chat store

	// Approval context for non-allowlisted commands
	Handler  agentboot.MessageHandler // Message handler for approval requests
	ChatID   string                   // Chat ID for approval routing
	Platform string                   // Platform identifier
	BotUUID  string                   // Bot UUID for routing
}

// NewTinglyBoxAgent creates a new smart guide agent
func NewTinglyBoxAgent(config *AgentConfig) (*TinglyBoxAgent, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.SmartGuideConfig == nil {
		config.SmartGuideConfig = DefaultSmartGuideConfig()
	}

	// Create tool executor if not provided
	executor := config.ToolExecutor
	if executor == nil {
		executor = NewToolExecutor([]string{"cd", "ls", "pwd"})
	}

	// Set approval callback if handler is provided
	if config.Handler != nil {
		executor.SetApprovalCallback(tb.createApprovalCallback(&config))
		logrus.WithField("chatID", config.ChatID).Info("Approval callback configured for ToolExecutor")
	}

	// Get model configuration from bot setting (required)
	var modelConfig *anthropic.Config

	// Validate that SmartGuide config is provided
	if config.Provider == "" || config.Model == "" {
		return nil, fmt.Errorf("smartguide_provider and smartguide_model are required in bot setting")
	}

	// Validate HTTP endpoint configuration
	if config.BaseURL == "" || config.APIKey == "" {
		return nil, fmt.Errorf("BaseURL and APIKey are required in config")
	}

	// Create model config using provided endpoint configuration
	modelConfig = &anthropic.Config{
		Model:   config.Model,
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	}
	logrus.WithFields(logrus.Fields{
		"provider": config.Provider,
		"model":    config.Model,
		"endpoint": config.BaseURL,
	}).Info("Using HTTP endpoint for smartguide agent")

	// Validate model configuration
	if modelConfig.APIKey == "" {
		return nil, fmt.Errorf("model configuration failed: no API key available")
	}

	modelClient, err := anthropic.NewClient(modelConfig)
	if err != nil {
		return nil, err
	}

	// Create toolkit
	toolkit := tool.NewToolkit()

	// Register tools
	if err := RegisterTools(toolkit, executor, config.GetStatusFunc, config.UpdateProjectFunc); err != nil {
		return nil, fmt.Errorf("failed to register tools: %w", err)
	}

	// Create memory
	mem := memory.NewHistory(100)

	// Create ReActAgent
	systemPrompt := config.SmartGuideConfig.GetSystemPrompt()
	reactConfig := &agent.ReActAgentConfig{
		Name:          "tingly-box",
		SystemPrompt:  systemPrompt,
		Model:         modelClient,
		Toolkit:       toolkit,
		Memory:        mem,
		MaxIterations: config.SmartGuideConfig.MaxIterations,
		Temperature:   &config.SmartGuideConfig.Temperature,
	}

	reactAgent := agent.NewReActAgent(reactConfig)

	// Register hook to capture intermediate messages
	// This will be set later via SetMessageHandler when ExecuteWithHandler is called
	// For now, create the agent without hooks

	return &TinglyBoxAgent{
		ReActAgent: reactAgent,
		config:     config.SmartGuideConfig,
		executor:   executor,
		toolkit:    toolkit,
	}, nil
}

// NewTinglyBoxAgentWithSession creates a new smart guide agent with conversation history from session
func NewTinglyBoxAgentWithSession(config *AgentConfig, messages []*message.Msg) (*TinglyBoxAgent, error) {
	// Create agent normally
	tbAgent, err := NewTinglyBoxAgent(config)
	if err != nil {
		return nil, err
	}

	// Load conversation history into agent's memory
	if len(messages) > 0 {
		mem := tbAgent.ReActAgent.GetMemory()
		if mem != nil {
			ctx := context.Background()
			for i, msg := range messages {
				contentStr := ""
				if s, ok := msg.Content.(string); ok {
					contentStr = s
					if len(contentStr) > 50 {
						contentStr = contentStr[:50] + "..."
					}
				}

				logrus.WithFields(logrus.Fields{
					"index":   i,
					"role":    msg.Role,
					"content": contentStr,
				}).Debug("Loading message from session into agent memory")

				if err := mem.Add(ctx, msg); err != nil {
					logrus.WithError(err).WithFields(logrus.Fields{
						"index": i,
						"role":  msg.Role,
					}).Warn("Failed to add message to memory")
				}
			}
			logrus.WithFields(logrus.Fields{
				"msgCount": len(messages),
			}).Info("Loaded conversation history into agent memory")
		}
	}

	return tbAgent, nil
}

// createApprovalCallback creates an approval callback function for tool execution
func (a *TinglyBoxAgent) createApprovalCallback(config *AgentConfig) func(context.Context, ApprovalRequest) (bool, error) {
	return func(ctx context.Context, req ApprovalRequest) (bool, error) {
		// Build permission request
		permReq := agentboot.PermissionRequest{
			RequestID: uuid.New().String(),
			AgentType: AgentTypeTinglyBox,
			ToolName:  "bash",
			Input: map[string]interface{}{
				"command": req.Command,
				"args":    req.Args,
			},
			Reason:    req.Reason,
			SessionID: config.ChatID, // Use chatID as session identifier
			BotUUID:   config.BotUUID,
			ChatID:    config.ChatID,
			Platform:  config.Platform,
		}

		// Request approval from handler
		result, err := config.Handler.OnApproval(ctx, permReq)
		if err != nil {
			logrus.WithError(err).WithField("command", req.Command).Error("Approval request failed")
			return false, err
		}

		logrus.WithFields(logrus.Fields{
			"command":  req.Command,
			"approved": result.Approved,
		}).Info("Approval result")

		return result.Approved, nil
	}
}

// ReplyWithContext handles a user message with additional context
func (a *TinglyBoxAgent) ReplyWithContext(ctx context.Context, text string, toolCtx *ToolContext) (*message.Msg, error) {
	// Update executor working directory if project path is provided
	if toolCtx != nil && toolCtx.ProjectPath != "" {
		a.executor.SetWorkingDirectory(toolCtx.ProjectPath)
	}

	// Create user message
	userMsg := message.NewMsg(
		"user",
		text,
		"user",
	)

	// Get response
	response, err := a.Reply(ctx, userMsg)
	if err != nil {
		logrus.WithError(err).Error("Failed to get agent response")
		return nil, fmt.Errorf("failed to get response: %w", err)
	}

	// Return the original response directly
	// Summary generation removed - user should see the actual agent response
	// not a meta-summary of what happened
	return response, nil
}

// GetGreeting returns the default greeting for new users
func (a *TinglyBoxAgent) GetGreeting() string {
	return DefaultGreeting()
}

// GetExecutor returns the tool executor
func (a *TinglyBoxAgent) GetExecutor() *ToolExecutor {
	return a.executor
}

// GetToolkit returns the agent's toolkit
func (a *TinglyBoxAgent) GetToolkit() *tool.Toolkit {
	return a.toolkit
}

// IsEnabled returns whether the smart guide is enabled
func (a *TinglyBoxAgent) IsEnabled() bool {
	return a.config != nil && a.config.Enabled
}

// GetConfig returns the agent's configuration
func (a *TinglyBoxAgent) GetConfig() *SmartGuideConfig {
	return a.config
}

// AgentFactory creates TinglyBoxAgent instances
type AgentFactory struct {
	config             *SmartGuideConfig
	baseURL            string // HTTP endpoint URL
	apiKey             string // Authentication token
	smartGuideProvider string // Provider UUID
	smartGuideModel    string // Model identifier
}

// NewAgentFactory creates a new agent factory
func NewAgentFactory(config *SmartGuideConfig, baseURL, apiKey string, smartGuideProvider, smartGuideModel string) *AgentFactory {
	return &AgentFactory{
		config:             config,
		baseURL:            baseURL,
		apiKey:             apiKey,
		smartGuideProvider: smartGuideProvider,
		smartGuideModel:    smartGuideModel,
	}
}

// CreateAgent creates a new TinglyBoxAgent with the given callbacks
func (f *AgentFactory) CreateAgent(getStatusFunc func(chatID string) (*StatusInfo, error),
	getProjectFunc func(chatID string) (string, bool, error),
	updateProjectFunc func(chatID string, projectPath string) error) (*TinglyBoxAgent, error) {

	return NewTinglyBoxAgent(&AgentConfig{
		SmartGuideConfig:  f.config,
		BaseURL:           f.baseURL,
		APIKey:            f.apiKey,
		Provider:          f.smartGuideProvider,
		Model:             f.smartGuideModel,
		GetStatusFunc:     getStatusFunc,
		GetProjectFunc:    getProjectFunc,
		UpdateProjectFunc: updateProjectFunc,
	})
}

// CanCreateAgent checks if a SmartGuide agent can be created with the given configuration
// Returns true if all required dependencies are available, false otherwise
// Note: Model validation should be done by the caller (e.g., BotHandler using TBClient)
func CanCreateAgent(baseURL, apiKey, smartGuideProvider, smartGuideModel string) bool {
	// Check if provider and model are configured
	if smartGuideProvider == "" || smartGuideModel == "" {
		return false
	}

	// Check if endpoint configuration is provided
	if baseURL == "" || apiKey == "" {
		return false
	}

	return true
}

// hasToolUseBlocks checks if a message contains tool_use blocks
func (a *TinglyBoxAgent) hasToolUseBlocks(msg *message.Msg) bool {
	if msg.Role != "assistant" {
		return false
	}

	// Check if content is a slice of blocks (Anthropic message format)
	if content, ok := msg.Content.([]interface{}); ok {
		for _, block := range content {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if blockType, ok := blockMap["type"].(string); ok && blockType == "tool_use" {
					return true
				}
			}
		}
	}

	return false
}

// generateSummary generates a summary using the LLM with full conversation history
func (a *TinglyBoxAgent) generateSummary(ctx context.Context, mem *memory.History) string {
	// Get all messages from memory
	messages := mem.GetMessages()
	if len(messages) == 0 {
		return ""
	}

	// Build conversation history for summary generation
	var historyBuilder strings.Builder
	historyBuilder.WriteString("Conversation history:\n\n")

	for i, msg := range messages {
		// Skip summary messages themselves
		if msg.Role == "summary" {
			continue
		}

		// Format message for summary prompt
		historyBuilder.WriteString(fmt.Sprintf("[%s] ", msg.Role))

		// Handle different content types
		if contentStr, ok := msg.Content.(string); ok {
			historyBuilder.WriteString(contentStr)
		} else if contentBlocks, ok := msg.Content.([]interface{}); ok {
			for _, block := range contentBlocks {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if blockType, ok := blockMap["type"].(string); ok {
						switch blockType {
						case "text":
							if text, ok := blockMap["text"].(string); ok {
								historyBuilder.WriteString(text)
							}
						case "tool_use":
							if name, ok := blockMap["name"].(string); ok {
								historyBuilder.WriteString(fmt.Sprintf("[used tool: %s]", name))
							}
						case "tool_result":
							historyBuilder.WriteString("[tool result]")
						}
					}
				}
			}
		}

		historyBuilder.WriteString("\n")

		// Limit history length (last 20 messages)
		if i > len(messages)-20 {
			break
		}
	}

	// Build the full prompt
	fullPrompt := fmt.Sprintf("%s\n\n%s", summaryPrompt, historyBuilder.String())

	// Create a user message with the summary prompt
	summaryMsg := message.NewMsg("user", fullPrompt, "user")

	// Call agent internally to generate summary
	// This is an internal call that doesn't affect the user-visible conversation
	summaryResponse, err := a.ReActAgent.Reply(ctx, summaryMsg)
	if err != nil {
		logrus.WithError(err).Warn("Failed to generate summary, using fallback")
		return a.generateFallbackSummary(messages)
	}

	return summaryResponse.GetTextContent()
}

// generateFallbackSummary generates a simple summary when LLM call fails
func (a *TinglyBoxAgent) generateFallbackSummary(messages []*message.Msg) string {
	var summary strings.Builder
	summary.WriteString("**Summary**\n\n")

	// Count tool calls
	toolCalls := make(map[string]int)
	for _, msg := range messages {
		if msg.Role == "assistant" {
			if contentBlocks, ok := msg.Content.([]interface{}); ok {
				for _, block := range contentBlocks {
					if blockMap, ok := block.(map[string]interface{}); ok {
						if blockType, ok := blockMap["type"].(string); ok && blockType == "tool_use" {
							if name, ok := blockMap["name"].(string); ok {
								toolCalls[name]++
							}
						}
					}
				}
			}
		}
	}

	// Build summary from tool calls
	if len(toolCalls) > 0 {
		var actions []string
		for tool := range toolCalls {
			actions = append(actions, formatToolAction(tool))
		}
		summary.WriteString("• ")
		summary.WriteString(strings.Join(actions, ", "))
		summary.WriteString("\n\n")

		summary.WriteString("**Tools used:** ")
		var tools []string
		for tool := range toolCalls {
			tools = append(tools, tool)
		}
		summary.WriteString(strings.Join(tools, ", "))
		summary.WriteString("\n")
	} else {
		summary.WriteString("• Task completed\n")
	}

	return summary.String()
}

// formatToolAction formats a tool name as a human-readable action
func formatToolAction(toolName string) string {
	switch toolName {
	case "bash_cd":
		return "changed directory"
	case "bash_ls":
		return "listed directory contents"
	case "bash_pwd":
		return "showed current directory"
	case "git_clone":
		return "cloned repository"
	case "git_status":
		return "checked git status"
	case "get_status":
		return "retrieved status"
	case "get_project":
		return "retrieved project info"
	default:
		// Convert tool_name to "tool name"
		parts := strings.Split(toolName, "_")
		return strings.Join(parts, " ")
	}
}

// uniqueStrings returns unique strings from a slice
func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// Execute implements agentboot.Agent interface for TinglyBoxAgent
// This allows SmartGuide to be used with the same callback mechanism as Claude Code and Mock Agent
func (a *TinglyBoxAgent) Execute(
	ctx context.Context,
	prompt string,
	opts agentboot.ExecutionOptions,
) (*agentboot.Result, error) {
	// Build ToolContext from execution options
	toolCtx := &ToolContext{
		ProjectPath: opts.ProjectPath,
		ChatID:      opts.ChatID,
		SessionID:   opts.SessionID,
	}

	// Call ExecuteWithHandler with the handler from options
	return a.ExecuteWithHandler(ctx, prompt, toolCtx, opts.Handler)
}

// ExecuteWithHandler executes the agent with callback support
// This enables streaming messages, completion callbacks, and error handling
func (a *TinglyBoxAgent) ExecuteWithHandler(
	ctx context.Context,
	prompt string,
	toolCtx *ToolContext,
	handler agentboot.MessageHandler,
) (*agentboot.Result, error) {
	startTime := time.Now()
	result := &agentboot.Result{
		Format:   agentboot.OutputFormatText,
		Metadata: make(map[string]interface{}),
	}

	// Update executor working directory if project path is provided
	if toolCtx != nil && toolCtx.ProjectPath != "" {
		a.executor.SetWorkingDirectory(toolCtx.ProjectPath)
	}

	// Send initial message callback if handler is provided
	if handler != nil {
		// Send a processing message
		handler.OnMessage(map[string]interface{}{
			"type":    "status",
			"status":  "processing",
			"message": "Smart Guide is thinking...",
		})
	}

	// Register hook to capture intermediate messages from ReAct loop
	// This hook is called after each model response (including text and tool blocks)
	hookName := fmt.Sprintf("stream_hook_%s", uuid.New().String())
	if handler != nil {
		streamHook := agent.LoopModelResponseHookFunc(func(ctx context.Context, ag agent.Agent, msg *message.Msg, hookCtx *agent.LoopModelResponseContext) error {
			// Extract text content from the message
			textContent := msg.GetTextContent()
			if strings.TrimSpace(textContent) != "" {
				// Send the intermediate message to the handler
				handler.OnMessage(map[string]interface{}{
					"type":      "assistant",
					"message":   textContent,
					"iteration": hookCtx.Iteration,
				})
				logrus.WithFields(logrus.Fields{
					"iteration":  hookCtx.Iteration,
					"textLength": len(textContent),
					"toolBlocks": hookCtx.ToolBlocksCount,
				}).Debug("SmartGuide: Sent intermediate message via hook")
			}
			return nil
		})

		// Register the hook
		if err := a.ReActAgent.RegisterHook(types.HookTypeLoopModelResponse, hookName, streamHook); err != nil {
			logrus.WithError(err).Warn("Failed to register loop hook for streaming")
		} else {
			// Ensure hook is cleaned up after execution
			defer func() {
				if err := a.ReActAgent.RemoveHook(types.HookTypeLoopModelResponse, hookName); err != nil {
					logrus.WithError(err).Warn("Failed to remove loop hook")
				}
			}()
		}
	}

	// Execute the agent
	response, err := a.ReplyWithContext(ctx, prompt, toolCtx)
	duration := time.Since(startTime)

	// Handle errors
	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
		result.Duration = duration

		// Send error callback
		if handler != nil {
			handler.OnError(err)
			handler.OnComplete(&agentboot.CompletionResult{
				Success:    false,
				DurationMS: duration.Milliseconds(),
				Error:      err.Error(),
				SessionID:  toolCtx.SessionID,
			})
		}

		logrus.WithError(err).WithFields(logrus.Fields{
			"duration_ms": duration.Milliseconds(),
			"session_id":  toolCtx.SessionID,
		}).Error("SmartGuide execution failed")

		return result, err
	}

	// Extract response content
	var responseText string
	if response != nil {
		responseText = response.GetTextContent()
		result.Output = responseText

		// Send message callback with the response
		if handler != nil {
			handler.OnComplete(&agentboot.CompletionResult{
				Success:    true,
				DurationMS: duration.Milliseconds(),
				SessionID:  toolCtx.SessionID,
			})
		}
	}

	result.ExitCode = 0
	result.Duration = duration

	logrus.WithFields(logrus.Fields{
		"duration_ms": duration.Milliseconds(),
		"session_id":  toolCtx.SessionID,
		"success":     true,
	}).Info("SmartGuide execution completed")

	return result, nil
}

// IsAvailable returns true if the agent is available for execution
func (a *TinglyBoxAgent) IsAvailable() bool {
	return a.IsEnabled()
}

// Type returns the agent type for agentboot.Agent interface
func (a *TinglyBoxAgent) Type() agentboot.AgentType {
	return AgentTypeTinglyBox
}

// SetDefaultFormat sets the default output format (no-op for SmartGuide)
func (a *TinglyBoxAgent) SetDefaultFormat(format agentboot.OutputFormat) {
	// SmartGuide always uses text format
}

// GetDefaultFormat returns the current default format
func (a *TinglyBoxAgent) GetDefaultFormat() agentboot.OutputFormat {
	return agentboot.OutputFormatText
}
