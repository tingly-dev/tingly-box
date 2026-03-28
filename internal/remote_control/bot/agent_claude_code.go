package bot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
)

// ClaudeCodeExecutor executes messages through Claude Code agent
type ClaudeCodeExecutor struct {
	deps *ExecutorDependencies
}

// NewClaudeCodeExecutor creates a new Claude Code executor
func NewClaudeCodeExecutor(deps *ExecutorDependencies) *ClaudeCodeExecutor {
	return &ClaudeCodeExecutor{deps: deps}
}

// GetAgentType returns the agent type identifier
func (e *ClaudeCodeExecutor) GetAgentType() agentboot.AgentType {
	return agentClaudeCode
}

// Execute processes a user message through Claude Code
func (e *ClaudeCodeExecutor) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	// Validate input
	if strings.TrimSpace(req.Text) == "" {
		e.deps.SendText(req.HCtx, "Please provide a message for Claude Code.")
		return nil, fmt.Errorf("empty message text")
	}

	// Determine project path: priority is override > bound project > default cwd
	projectPath := req.ProjectPath
	if projectPath == "" {
		boundPath, hasBound, _ := e.deps.ChatStore.GetProjectPath(req.HCtx.ChatID)
		if hasBound && boundPath != "" {
			projectPath = boundPath
		}
	}
	if projectPath == "" {
		projectPath = e.getDefaultProjectPath()
		logrus.WithFields(logrus.Fields{
			"chatID":     req.HCtx.ChatID,
			"defaultCwd": projectPath,
		}).Info("Using default cwd for Claude Code")
	}

	// Find or create session
	agentType := "claude"
	sess := e.deps.SessionMgr.FindBy(req.HCtx.ChatID, agentType, projectPath)

	isNewSession := false
	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed || sess.Status == session.StatusPending {
		sess = e.deps.SessionMgr.CreateWith(req.HCtx.ChatID, agentType, projectPath)
		// Clear expiration for persistent sessions
		e.deps.SessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{}        // No expiration
			s.Status = session.StatusRunning // Mark as running immediately
		})
		isNewSession = true

		logrus.WithFields(logrus.Fields{
			"chatID":    req.HCtx.ChatID,
			"sessionID": sess.ID,
			"project":   projectPath,
			"agent":     agentType,
		}).Info("Created new session for Claude Code")
	} else {
		// Reset status to running for reused sessions
		e.deps.SessionMgr.Update(sess.ID, func(s *session.Session) {
			s.Status = session.StatusRunning
		})
		logrus.WithFields(logrus.Fields{
			"chatID":    req.HCtx.ChatID,
			"sessionID": sess.ID,
			"project":   projectPath,
			"agent":     agentType,
			"status":    sess.Status,
		}).Info("Resumed existing session for Claude Code")
	}

	sessionID := sess.ID

	// Refresh session activity
	e.deps.SessionMgr.Update(sessionID, func(s *session.Session) {
		s.LastActivity = time.Now()
	})

	// Build meta
	meta := ResponseMeta{
		ProjectPath: projectPath,
		AgentType:   string(agentboot.AgentTypeClaude),
		SessionID:   sessionID,
		ChatID:      req.HCtx.ChatID,
		UserID:      req.HCtx.SenderID,
	}

	// Append user message to session
	e.deps.SessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   req.Text,
		Timestamp: time.Now(),
	})

	e.deps.SessionMgr.SetRunning(sessionID)

	// Send status message
	var statusMsg string
	if isNewSession {
		statusMsg = "⏳ CC: Processing new session..."
	} else {
		statusMsg = "⏳ CC: Resuming session..."
	}
	e.deps.SendTextWithReply(req.HCtx, e.deps.FormatResponseWithFooter(meta, statusMsg), req.HCtx.MessageID)

	// Execute with cancellable context
	execCtx, cancel := context.WithCancel(context.Background())

	// Store cancel function for /stop command
	e.deps.RunningCancelMu.Lock()
	e.deps.RunningCancel[req.HCtx.ChatID] = cancel
	e.deps.RunningCancelMu.Unlock()

	// Clean up cancel function when done
	defer func() {
		e.deps.RunningCancelMu.Lock()
		delete(e.deps.RunningCancel, req.HCtx.ChatID)
		e.deps.RunningCancelMu.Unlock()
		cancel()
	}()

	// Get agent instance
	agent, err := e.deps.AgentBoot.GetDefaultAgent()
	if err != nil {
		e.deps.SessionMgr.SetFailed(sessionID, "agent not available: "+err.Error())
		e.deps.SendTextWithReply(req.HCtx, "Agent not available", req.HCtx.MessageID)
		return &ExecutionResult{
			SessionID: sessionID,
			Success:   false,
			Error:     err,
			Meta:      &meta,
		}, err
	}

	// Determine if we should resume
	shouldResume := false
	if msgs, ok := e.deps.SessionMgr.GetMessages(sessionID); ok && len(msgs) > 1 {
		shouldResume = true
	}

	logrus.WithFields(logrus.Fields{
		"chatID":       req.HCtx.ChatID,
		"sessionID":    sessionID,
		"projectPath":  projectPath,
		"shouldResume": shouldResume,
	}).Info("Starting Claude Code execution")

	// Create streaming handler
	streamHandler := e.deps.NewStreamingMessageHandler(req.HCtx, &meta)

	// Check permission mode for this session
	permissionMode := sess.PermissionMode
	if permissionMode == "" {
		permissionMode = string(claude.PermissionModeDefault)
	}

	// Create composite handler
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(streamHandler).
		SetCompletionCallback(&CompletionCallback{
			hCtx:       req.HCtx,
			sessionID:  sessionID,
			sessionMgr: e.deps.SessionMgr,
			meta:       &meta,
		})

	if permissionMode != string(claude.PermissionModeAuto) {
		// Normal mode: use approval handler
		compositeHandler.SetApprovalHandler(e.deps.IMPrompter).
			SetAskHandler(e.deps.IMPrompter)
	}

	// Execute
	startTime := time.Now()
	result, err := agent.Execute(execCtx, req.Text, agentboot.ExecutionOptions{
		ProjectPath:          projectPath,
		Handler:              compositeHandler,
		SessionID:            sessionID,
		Resume:               shouldResume,
		ChatID:               req.HCtx.ChatID,
		Platform:             string(req.HCtx.Platform),
		BotUUID:              req.HCtx.BotUUID,
		PermissionPromptTool: "stdio",
		PermissionMode:       permissionMode,
	})
	duration := time.Since(startTime)

	logrus.WithFields(logrus.Fields{
		"chatID":    req.HCtx.ChatID,
		"sessionID": sessionID,
		"hasError":  err != nil,
		"hasResult": result != nil,
		"duration":  duration,
	}).Info("Claude Code execution completed")

	// Get response text
	response := streamHandler.GetOutput()
	if response == "" {
		if result != nil {
			response = result.TextOutput()
		}
		if err != nil && response == "" {
			response = fmt.Sprintf("Execution failed: %v", err)
		}
	}

	// Handle errors
	if err != nil {
		e.deps.SessionMgr.SetFailed(sessionID, response)
		logrus.WithError(err).WithFields(logrus.Fields{
			"chatID":    req.HCtx.ChatID,
			"sessionID": sessionID,
			"response":  response,
		}).Warn("Claude Code execution failed")

		if response == "" {
			response = fmt.Sprintf("Execution failed: %v", err)
		}
		e.deps.SendTextWithReply(req.HCtx, response, req.HCtx.MessageID)
		return &ExecutionResult{
			SessionID:    sessionID,
			Success:      false,
			Error:        err,
			Response:     response,
			Meta:         &meta,
			IsNewSession: isNewSession,
			Duration:     duration,
		}, err
	}

	// Success - update session
	e.deps.SessionMgr.SetCompleted(sessionID, response)
	e.deps.SessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	// Send response with action keyboard
	e.deps.SendTextWithActionKeyboard(req.HCtx, response, req.HCtx.MessageID)

	return &ExecutionResult{
		SessionID:    sessionID,
		Success:      true,
		Response:     response,
		Meta:         &meta,
		IsNewSession: isNewSession,
		Duration:     duration,
	}, nil
}

// getDefaultProjectPath returns the default project path
// Priority: 1. DefaultCwd from bot setting, 2. Current working directory, 3. User home directory
func (e *ClaudeCodeExecutor) getDefaultProjectPath() string {
	// 1. Check bot setting's DefaultCwd
	if e.deps.BotSetting.DefaultCwd != "" {
		expanded, err := ExpandPath(e.deps.BotSetting.DefaultCwd)
		if err == nil {
			return expanded
		}
		logrus.WithError(err).Warnf("Failed to expand DefaultCwd: %s", e.deps.BotSetting.DefaultCwd)
	}

	// 2. Use current working directory
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	// 3. Fallback to user home directory
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	// Ultimate fallback
	return "/"
}
