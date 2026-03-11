package smart_guide

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/agent"
	"github.com/tingly-dev/tingly-agentscope/pkg/memory"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/model/anthropic"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
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
	TBClient         tbclient.TBClient // TB Client for getting model configuration
	ToolExecutor     *ToolExecutor
	// SmartGuide model configuration (required from bot setting)
	SmartGuideProvider string // Provider UUID
	SmartGuideModel    string // Model identifier
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
	if config.SmartGuideProvider == "" || config.SmartGuideModel == "" {
		return nil, fmt.Errorf("smartguide_provider and smartguide_model are required in bot setting")
	}

	if config.TBClient != nil {
		// Get provider configuration via SelectModel
		ctx := context.Background()
		modelCfg, err := config.TBClient.SelectModel(ctx, tbclient.ModelSelectionRequest{
			ProviderUUID: config.SmartGuideProvider,
			ModelID:      config.SmartGuideModel,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get model config for provider %s, model %s: %w", config.SmartGuideProvider, config.SmartGuideModel, err)
		}

		// Use bot setting configuration
		modelConfig = &anthropic.Config{
			Model:   modelCfg.ModelID,
			APIKey:  modelCfg.APIKey,
			BaseURL: modelCfg.BaseURL,
		}
		logrus.WithFields(logrus.Fields{
			"provider": config.SmartGuideProvider,
			"model":    config.SmartGuideModel,
		}).Info("Using bot setting configuration for smartguide agent")
	}

	if modelConfig == nil {
		return nil, fmt.Errorf("failed to create model config: TBClient not available")
	}

	// Validate model configuration
	if modelConfig.APIKey == "" {
		return nil, fmt.Errorf("model configuration failed: no API key available from TB Client or config")
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
func NewTinglyBoxAgentWithSession(config *AgentConfig, sess *SmartGuideSession) (*TinglyBoxAgent, error) {
	// Create agent normally
	tbAgent, err := NewTinglyBoxAgent(config)
	if err != nil {
		return nil, err
	}

	// Load conversation history into agent's memory
	if sess != nil && len(sess.Messages) > 0 {
		mem := tbAgent.ReActAgent.GetMemory()
		if mem != nil {
			ctx := context.Background()
			for i, msg := range sess.Messages {
				contentPreview := msg.Content
				if len(contentPreview) > 50 {
					contentPreview = contentPreview[:50] + "..."
				}

				logrus.WithFields(logrus.Fields{
					"index":   i,
					"role":    msg.Role,
					"content": contentPreview,
				}).Debug("Loading message from session into agent memory")

				// Convert string role to types.Role
				var role types.Role
				if msg.Role == "user" {
					role = types.RoleUser
				} else if msg.Role == "assistant" {
					role = types.RoleAssistant
				} else {
					// Skip unknown roles
					logrus.WithField("role", msg.Role).Warn("Unknown role, skipping message")
					continue
				}

				// message.NewMsg takes (name, content, role)
				// name is typically the role identifier, role is types.Role
				agentMsg := message.NewMsg(string(role), msg.Content, role)
				if err := mem.Add(ctx, agentMsg); err != nil {
					logrus.WithError(err).WithFields(logrus.Fields{
						"index": i,
						"role":  msg.Role,
					}).Warn("Failed to add message to memory")
				} else {
					logrus.WithFields(logrus.Fields{
						"index": i,
						"role":  msg.Role,
					}).Debug("Successfully loaded message into memory")
				}
			}
			logrus.WithFields(logrus.Fields{
				"chatID":   sess.ChatID,
				"msgCount": len(sess.Messages),
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

	return response, nil
}

// GetGreeting returns the default greeting for new users
func (a *TinglyBoxAgent) GetGreeting() string {
	return DefaultGreeting
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
	tbClient           tbclient.TBClient
	smartGuideProvider string // Provider UUID
	smartGuideModel    string // Model identifier
}

// NewAgentFactory creates a new agent factory
func NewAgentFactory(config *SmartGuideConfig, tbClient tbclient.TBClient, smartGuideProvider, smartGuideModel string) *AgentFactory {
	return &AgentFactory{
		config:             config,
		tbClient:           tbClient,
		smartGuideProvider: smartGuideProvider,
		smartGuideModel:    smartGuideModel,
	}
}

// CreateAgent creates a new TinglyBoxAgent with the given callbacks
func (f *AgentFactory) CreateAgent(getStatusFunc func(chatID string) (*StatusInfo, error),
	getProjectFunc func(chatID string) (string, bool, error),
	updateProjectFunc func(chatID string, projectPath string) error) (*TinglyBoxAgent, error) {

	return NewTinglyBoxAgent(&AgentConfig{
		SmartGuideConfig:   f.config,
		TBClient:           f.tbClient,
		SmartGuideProvider: f.smartGuideProvider,
		SmartGuideModel:    f.smartGuideModel,
		GetStatusFunc:      getStatusFunc,
		GetProjectFunc:     getProjectFunc,
		UpdateProjectFunc:  updateProjectFunc,
	})
}
