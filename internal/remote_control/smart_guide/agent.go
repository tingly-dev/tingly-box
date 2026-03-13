package smart_guide

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/agent"
	"github.com/tingly-dev/tingly-agentscope/pkg/memory"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/model/anthropic"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
)

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

	// Get memory for summary generation
	mem := a.ReActAgent.GetMemory()

	// Check if we need to generate summary by looking at the last message
	// If the last message contains tool_use blocks, it means agent is still requesting tools
	// In that case, we should generate a summary for the user
	var summaryText string
	var hist *memory.History
	responseText := response.GetTextContent()

	if mem != nil {
		var ok bool
		hist, ok = mem.(*memory.History)
		if hist != nil && ok {
			messages := hist.GetMessages()
			if len(messages) > 0 {
				lastMsg := messages[len(messages)-1]
				// Generate summary UNLESS last message is assistant without tool_use
				// (meaning agent returned text directly without requesting more tools)
				if !(lastMsg.Role == "assistant" && !a.hasToolUseBlocks(lastMsg)) {
					summaryText = a.generateSummary(ctx, hist)

					// Generate summary if tools were called

					// Add summary to memory as a special message
					summaryMsg := message.NewMsg(
						"summary",
						summaryText,
						"system",
					)
					if err := mem.Add(ctx, summaryMsg); err != nil {
						logrus.WithError(err).Warn("Failed to add summary to memory")
					}

					// Append summary to response
					responseText += "\n\n---\n\n" + summaryText
					response.Content = responseText
				}
			}
		}
	}

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
	historyBuilder.WriteString("Below is the conversation history. Please generate a concise summary (2-3 bullet points max) of what was accomplished:\n\n")

	for i, msg := range messages {
		// Skip summary messages themselves
		if msg.Role == "summary" {
			continue
		}

		// Format message for summary prompt
		historyBuilder.WriteString(fmt.Sprintf("[%s] ", msg.Role))

		// Handle different content types
		if contentStr, ok := msg.Content.(string); ok {
			// Simple text content
			historyBuilder.WriteString(contentStr)
		} else if contentBlocks, ok := msg.Content.([]interface{}); ok {
			// Anthropic-style content blocks
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
								historyBuilder.WriteString(fmt.Sprintf("[Tool: %s]", name))
							}
						case "tool_result":
							if content, ok := blockMap["content"].(string); ok {
								// Truncate long tool results
								if len(content) > 100 {
									content = content[:100] + "..."
								}
								historyBuilder.WriteString(fmt.Sprintf("[Result: %s]", content))
							}
						}
					}
				}
			}
		}

		historyBuilder.WriteString("\n")

		// Limit history length for summary (last 20 messages)
		if i > len(messages)-20 {
			break
		}
	}

	historyBuilder.WriteString("\nPlease provide a brief summary in the following format:\n")
	historyBuilder.WriteString("**Summary**\n\n• [what was done]\n• [key result]\n• [tools used]\n")

	// Create summary request message
	summaryPrompt := message.NewMsg("user", historyBuilder.String(), "user")

	// Call LLM to generate summary (use a separate call to avoid affecting main conversation)
	// We use the ReActAgent's underlying model client
	summaryResponse, err := a.ReActAgent.Reply(ctx, summaryPrompt)
	if err != nil {
		logrus.WithError(err).Warn("Failed to generate summary with LLM, using fallback")
		return a.generateFallbackSummary(messages)
	}

	// Extract the summary text
	summaryText := summaryResponse.GetTextContent()

	return summaryText
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
