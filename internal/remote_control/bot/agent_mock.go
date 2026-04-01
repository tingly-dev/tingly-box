package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	mock "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
)

// MockAgentExecutor executes messages through Mock Agent (for testing)
type MockAgentExecutor struct {
	deps *ExecutorDependencies
}

// NewMockAgentExecutor creates a new Mock Agent executor
func NewMockAgentExecutor(deps *ExecutorDependencies) *MockAgentExecutor {
	return &MockAgentExecutor{deps: deps}
}

// GetAgentType returns the agent type identifier
func (e *MockAgentExecutor) GetAgentType() agentboot.AgentType {
	return agentMock
}

// Execute processes a user message through Mock Agent
func (e *MockAgentExecutor) Execute(ctx context.Context, req PreparedRequest) (*ExecutionResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		e.deps.SendText(req.HCtx, "Please provide a message for the mock agent.")
		return nil, fmt.Errorf("empty message text")
	}

	sessionID := req.SessionID
	projectPath := req.ProjectPath
	meta := req.Meta

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
		statusMsg = "🧪 Mock: Processing new session..."
	} else {
		statusMsg = "🧪 Mock: Resuming session..."
	}
	e.deps.SendTextWithReply(req.HCtx, e.deps.FormatResponseWithFooter(*meta, statusMsg), req.ReplyTo)

	// Get mock agent
	mockAgent, err := e.deps.AgentBoot.GetAgent(agentboot.AgentTypeMockAgent)
	if err != nil {
		newMockAgent := mock.NewAgent(mock.Config{
			MaxIterations: 3,
			StepDelay:     2 * time.Second,
		})
		e.deps.AgentBoot.RegisterAgent(agentboot.AgentTypeMockAgent, newMockAgent)
		mockAgent = newMockAgent
	}

	logrus.WithFields(logrus.Fields{
		"chatID":    req.HCtx.ChatID,
		"sessionID": sessionID,
		"agent":     "mock",
	}).Info("Starting mock agent execution")

	// Create streaming handler (shared meta pointer)
	streamHandler := e.deps.NewStreamingMessageHandler(req.HCtx, meta)

	// Create composite handler
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(streamHandler).
		SetApprovalHandler(e.deps.IMPrompter).
		SetAskHandler(e.deps.IMPrompter)

	startTime := time.Now()
	result, err := mockAgent.Execute(ctx, req.Text, agentboot.ExecutionOptions{
		ProjectPath: projectPath,
		Handler:     compositeHandler,
		SessionID:   sessionID,
		ChatID:      req.HCtx.ChatID,
		Platform:    string(req.HCtx.Platform),
		BotUUID:     req.HCtx.BotUUID,
	})
	duration := time.Since(startTime)

	logrus.WithFields(logrus.Fields{
		"chatID":    req.HCtx.ChatID,
		"sessionID": sessionID,
		"hasError":  err != nil,
		"hasResult": result != nil,
		"duration":  duration,
	}).Info("Mock agent execution completed")

	response := streamHandler.GetOutput()
	if response == "" {
		if result != nil {
			response = result.TextOutput()
		}
		if err != nil && response == "" {
			response = fmt.Sprintf("Execution failed: %v", err)
		}
	}

	if err != nil {
		e.deps.SessionMgr.SetFailed(sessionID, response)
		logrus.WithError(err).WithFields(logrus.Fields{
			"chatID":    req.HCtx.ChatID,
			"sessionID": sessionID,
			"response":  response,
		}).Warn("Mock agent execution failed")

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
