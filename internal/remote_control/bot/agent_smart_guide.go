package bot

import (
	"context"
	"fmt"
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
func (e *SmartGuideExecutor) Execute(ctx context.Context, req PreparedRequest) (*ExecutionResult, error) {
	projectPath := req.ProjectPath
	meta := req.Meta

	// 1. Load messages from session store
	var messages []*message.Msg
	if e.deps.TBSessionStore != nil {
		msgs, err := e.deps.TBSessionStore.Load(req.HCtx.ChatID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to load session, starting with empty history")
		} else {
			messages = msgs
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
		e.deps.SendText(req.HCtx, "⚠️ Smart Guide (@tb) is currently unavailable due to configuration issues.\n"+
			"Reason: "+err.Error()+"\n"+
			"Type '/help' for available commands.")
		return &ExecutionResult{
			SessionID: req.SessionID,
			Success:   false,
			Error:     err,
			Meta:      meta,
		}, err
	}

	// Set working directory from resolved project path
	agent.GetExecutor().SetWorkingDirectory(projectPath)

	// 5. Send processing message
	e.deps.SendTextWithReply(req.HCtx, e.deps.FormatResponseWithFooter(*meta, IconProcess+" "+MsgProcessing), req.ReplyTo)

	// 6. Create streaming handler (shared meta pointer)
	streamHandler := e.deps.NewStreamingMessageHandler(req.HCtx, meta)

	// 7. Create completion callback
	completionCallback := &SmartGuideCompletionCallback{
		hCtx:           req.HCtx,
		sessionID:      req.SessionID,
		chatStore:      e.deps.ChatStore,
		tbSessionStore: e.deps.TBSessionStore,
		agent:          agent,
		projectPath:    projectPath,
		meta:           meta,
		behavior:       e.deps.BotSetting.GetOutputBehavior(),
		botHandler:     nil,
		messagesSent:   0,
	}

	// 8. Create message tracker wrapper
	messageTracker := &messageTrackingWrapper{
		delegate:           streamHandler,
		completionCallback: completionCallback,
	}

	// 9. Create composite handler
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(messageTracker).
		SetCompletionCallback(completionCallback)

	// 10. Save user message to session before execution
	if e.deps.TBSessionStore != nil {
		userMsg := message.NewMsg("user", req.Text, types.RoleUser)
		if err := e.deps.TBSessionStore.AddMessage(req.HCtx.ChatID, userMsg); err != nil {
			logrus.WithError(err).Warn("Failed to save user message to session")
		}
	}

	// 11. Execute
	startTime := time.Now()
	toolCtx := &smart_guide.ToolContext{
		ChatID:      req.HCtx.ChatID,
		ProjectPath: projectPath,
		SessionID:   req.SessionID,
	}

	result, err := agent.ExecuteWithHandler(ctx, req.Text, toolCtx, compositeHandler)
	duration := time.Since(startTime)

	if err != nil {
		logrus.WithError(err).Error("Smart guide agent failed")
		e.deps.SendText(req.HCtx, fmt.Sprintf("%s Error: %v", IconError, err))
		return &ExecutionResult{
			SessionID: req.SessionID,
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
		SessionID: req.SessionID,
		Success:   true,
		Meta:      meta,
		Duration:  duration,
	}, nil
}
