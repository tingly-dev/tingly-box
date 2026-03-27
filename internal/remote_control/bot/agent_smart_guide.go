package bot

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SmartGuideExecutor executes messages through Smart Guide (Tingly Box) agent
type SmartGuideExecutor struct {
	deps *ExecutorDependencies
}

// NewSmartGuideExecutor creates a new Smart Guide executor
func NewSmartGuideExecutor(deps *ExecutorDependencies) *SmartGuideExecutor {
	return &SmartGuideExecutor{deps: deps}
}

// GetAgentType returns the agent type identifier
func (e *SmartGuideExecutor) GetAgentType() agentboot.AgentType {
	return agentTinglyBox
}

// Execute processes a user message through Smart Guide
func (e *SmartGuideExecutor) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	// Get current project path from chat store
	projectPath, hasPath, err := e.deps.ChatStore.GetProjectPath(req.HCtx.ChatID)
	logrus.WithFields(logrus.Fields{
		"chatID":      req.HCtx.ChatID,
		"projectPath": projectPath,
		"hasPath":     hasPath,
	}).Info("Loaded project path from chat store")

	if projectPath == "" {
		projectPath = e.getDefaultProjectPath()
		logrus.WithField("defaultPath", projectPath).Info("Using default project path")
	}

	// 1. Load messages from session store
	var messages []*message.Msg
	if e.deps.TBSessionStore != nil {
		messages, err = e.deps.TBSessionStore.Load(req.HCtx.ChatID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to load session, starting with empty history")
			messages = nil
		}

		logrus.WithFields(logrus.Fields{
			"chatID":       req.HCtx.ChatID,
			"historyCount": len(messages),
		}).Info("Loaded SmartGuide messages")
	}

	// 2. Resolve HTTP endpoint configuration for SmartGuide
	var baseURL, apiKey string
	if e.deps.TBClient != nil {
		endpoint, err := e.deps.TBClient.GetHTTPEndpointForScenario(ctx, typ.ScenarioSmartGuide)
		if err != nil {
			logrus.WithError(err).Warn("Failed to get SmartGuide HTTP endpoint")
		} else {
			baseURL = endpoint.BaseURL
			apiKey = endpoint.APIKey
		}
	}

	// 3. Create agent config
	agentConfig := &smart_guide.AgentConfig{
		SmartGuideConfig: smart_guide.LoadSmartGuideConfig(),
		BaseURL:          baseURL,
		APIKey:           apiKey,
		Provider:         e.deps.BotSetting.SmartGuideProvider,
		Model:            e.deps.BotSetting.SmartGuideModel,
		Handler:          agentboot.NewCompositeHandler().SetApprovalHandler(e.deps.IMPrompter),
		ChatID:           req.HCtx.ChatID,
		Platform:         string(req.HCtx.Platform),
		BotUUID:          e.deps.BotSetting.UUID,
		GetStatusFunc: func(chatID string) (*smart_guide.StatusInfo, error) {
			projectPath, _, _ := e.deps.ChatStore.GetProjectPath(chatID)
			workingDir, hasWD, _ := e.deps.ChatStore.GetBashCwd(chatID)
			if !hasWD {
				workingDir = projectPath
			}

			return &smart_guide.StatusInfo{
				CurrentAgent:   "tingly-box",
				SessionID:      chatID,
				ProjectPath:    projectPath,
				WorkingDir:     workingDir,
				HasRunningTask: false,
				Whitelisted:    e.deps.ChatStore.IsWhitelisted(chatID),
			}, nil
		},
		GetProjectFunc: func(chatID string) (string, bool, error) {
			return e.deps.ChatStore.GetProjectPath(chatID)
		},
		UpdateProjectFunc: func(chatID string, newProjectPath string) error {
			logrus.WithFields(logrus.Fields{
				"chatID":  chatID,
				"oldPath": projectPath,
				"newPath": newProjectPath,
			}).Info("updateProjectFunc called - persisting to chat store")
			return e.deps.ChatStore.UpdateChat(chatID, func(chat *Chat) {
				chat.ProjectPath = newProjectPath
				chat.BashCwd = newProjectPath
			})
		},
	}

	// 4. Create agent with history
	agent, err := smart_guide.NewTinglyBoxAgentWithSession(agentConfig, messages)
	if err != nil {
		return e.handleCreationError(ctx, req, projectPath, err)
	}

	// Set working directory from stored project path
	agent.GetExecutor().SetWorkingDirectory(projectPath)
	logrus.WithField("workingDir", projectPath).Debug("Set executor working directory")

	// 5. Build meta for response header
	meta := ResponseMeta{
		ProjectPath: projectPath,
		ChatID:      req.HCtx.ChatID,
		UserID:      req.HCtx.SenderID,
		SessionID:   req.HCtx.ChatID,
		AgentType:   AgentNameTinglyBox,
	}

	// 6. Send processing message
	e.deps.SendTextWithReply(req.HCtx, e.deps.FormatResponse(meta, IconProcess+" "+MsgProcessing, false), req.HCtx.MessageID)

	// 7. Create streaming handler
	streamHandler := e.deps.NewStreamingMessageHandler(req.HCtx)

	// 8. Create completion callback
	completionCallback := &SmartGuideCompletionCallback{
		hCtx:           req.HCtx,
		sessionID:      req.HCtx.ChatID,
		chatStore:      e.deps.ChatStore,
		tbSessionStore: e.deps.TBSessionStore,
		agent:          agent,
		projectPath:    projectPath,
		meta:           meta,
		behavior:       e.deps.BotSetting.GetOutputBehavior(),
		botHandler:     nil, // Will be set later if needed
		messagesSent:   0,
	}

	// 9. Create message tracker wrapper
	messageTracker := &messageTrackingWrapper{
		delegate:           streamHandler,
		completionCallback: completionCallback,
	}

	// 10. Create composite handler
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(messageTracker).
		SetCompletionCallback(completionCallback)

	// 11. Execute with cancellable context
	execCtx, cancel := context.WithCancel(context.Background())

	// Store cancel function for /stop command
	e.deps.RunningCancelMu.Lock()
	e.deps.RunningCancel[req.HCtx.ChatID] = cancel
	e.deps.RunningCancelMu.Unlock()

	defer func() {
		e.deps.RunningCancelMu.Lock()
		delete(e.deps.RunningCancel, req.HCtx.ChatID)
		e.deps.RunningCancelMu.Unlock()
		cancel()
	}()

	// Save user message to session before execution
	if e.deps.TBSessionStore != nil {
		userMsg := message.NewMsg("user", req.Text, types.RoleUser)
		if err := e.deps.TBSessionStore.AddMessage(req.HCtx.ChatID, userMsg); err != nil {
			logrus.WithError(err).Warn("Failed to save user message to session")
		}
	}

	// 12. Execute
	startTime := time.Now()
	toolCtx := &smart_guide.ToolContext{
		ChatID:      req.HCtx.ChatID,
		ProjectPath: projectPath,
		SessionID:   req.HCtx.ChatID,
	}

	result, err := agent.ExecuteWithHandler(execCtx, req.Text, toolCtx, compositeHandler)
	duration := time.Since(startTime)

	if err != nil {
		logrus.WithError(err).Error("Smart guide agent failed")
		e.deps.SendText(req.HCtx, fmt.Sprintf("%s Error: %v", IconError, err))
		return &ExecutionResult{
			SessionID: req.HCtx.ChatID,
			Success:   false,
			Error:     err,
			Meta:      meta,
			Duration:  duration,
		}, err
	}

	logrus.WithFields(logrus.Fields{
		"chatID":   req.HCtx.ChatID,
		"success":  result != nil,
		"duration": duration,
	}).Info("SmartGuide execution completed")

	return &ExecutionResult{
		SessionID: req.HCtx.ChatID,
		Success:   true,
		Meta:      meta,
		Duration:  duration,
	}, nil
}

// handleCreationError handles agent creation errors with fallback to Claude Code
func (e *SmartGuideExecutor) handleCreationError(ctx context.Context, req ExecutionRequest, projectPath string, err error) (*ExecutionResult, error) {
	logrus.WithError(err).Warn("Failed to create Smart Guide agent, fallback handled by caller")

	// Send warning notification to user
	e.deps.SendText(req.HCtx, "⚠️ Smart Guide (@tb) is currently unavailable due to configuration issues.\n"+
		"Reason: "+err.Error()+"\n"+
		"Type '/help' for available commands.")

	return &ExecutionResult{
		SessionID: req.HCtx.ChatID,
		Success:   false,
		Error:     err,
		Meta: ResponseMeta{
			ProjectPath: projectPath,
			ChatID:      req.HCtx.ChatID,
			UserID:      req.HCtx.SenderID,
			SessionID:   req.HCtx.ChatID,
			AgentType:   AgentNameTinglyBox,
		},
	}, err
}

// getDefaultProjectPath returns the default project path
func (e *SmartGuideExecutor) getDefaultProjectPath() string {
	if e.deps.BotSetting.DefaultCwd != "" {
		if expanded, err := ExpandPath(e.deps.BotSetting.DefaultCwd); err == nil {
			return expanded
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}
