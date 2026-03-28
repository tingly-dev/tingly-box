package bot

import (
	"context"
	"fmt"
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
func (e *ClaudeCodeExecutor) Execute(ctx context.Context, req PreparedRequest) (*ExecutionResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		e.deps.SendText(req.HCtx, "Please provide a message for Claude Code.")
		return nil, fmt.Errorf("empty message text")
	}

	sessionID := req.SessionID
	projectPath := req.ProjectPath
	meta := req.Meta

	// Refresh session activity
	e.deps.SessionMgr.Update(sessionID, func(s *session.Session) {
		s.LastActivity = time.Now()
	})

	// Append user message to session
	e.deps.SessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   req.Text,
		Timestamp: time.Now(),
	})
	e.deps.SessionMgr.SetRunning(sessionID)

	// Send status message
	var statusMsg string
	if req.IsNewSession {
		statusMsg = "⏳ CC: Processing new session..."
	} else {
		statusMsg = "⏳ CC: Resuming session..."
	}
	e.deps.SendTextWithReply(req.HCtx, e.deps.FormatResponseWithFooter(*meta, statusMsg), req.ReplyTo)

	// Get agent instance
	agent, err := e.deps.AgentBoot.GetDefaultAgent()
	if err != nil {
		e.deps.SessionMgr.SetFailed(sessionID, "agent not available: "+err.Error())
		e.deps.SendTextWithReply(req.HCtx, "Agent not available", req.ReplyTo)
		return &ExecutionResult{
			SessionID: sessionID,
			Success:   false,
			Error:     err,
			Meta:      meta,
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

	// Create streaming handler (shared meta pointer)
	streamHandler := e.deps.NewStreamingMessageHandler(req.HCtx, meta)

	// Use resolved permission mode (default to manual if not set)
	permissionMode := req.PermissionMode
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
			meta:       meta,
		})

	if permissionMode != string(claude.PermissionModeAuto) {
		compositeHandler.SetApprovalHandler(e.deps.IMPrompter).
			SetAskHandler(e.deps.IMPrompter)
	}

	// Execute
	startTime := time.Now()
	result, err := agent.Execute(ctx, req.Text, agentboot.ExecutionOptions{
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
		e.deps.SendTextWithReply(req.HCtx, response, req.ReplyTo)
		return &ExecutionResult{
			SessionID:    sessionID,
			Success:      false,
			Error:        err,
			Response:     response,
			Meta:         meta,
			IsNewSession: req.IsNewSession,
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

	e.deps.SendTextWithActionKeyboard(req.HCtx, response, req.ReplyTo)

	return &ExecutionResult{
		SessionID:    sessionID,
		Success:      true,
		Response:     response,
		Meta:         meta,
		IsNewSession: req.IsNewSession,
		Duration:     duration,
	}, nil
}
