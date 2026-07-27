package remoteagent

import (
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
)

// SmartGuideCompletionCallback handles completion events for SmartGuide agent
// It saves messages to session, updates project path if changed, and sends response + action keyboard
type SmartGuideCompletionCallback struct {
	hCtx           HandlerContext
	chatStore      bot.ChatStoreInterface
	tbSessionStore *smart_guide.SessionStore
	agent          *smart_guide.TinglyBoxAgent
	meta           *ResponseMeta
	sendText       func(hCtx HandlerContext, text string)
	messagesSent   int // Track number of messages sent via hooks (for fallback)
}

// messageTrackingWrapper wraps a message handler and tracks assistant messages
// This is used to detect silent completions (Issue #3)
type messageTrackingWrapper struct {
	delegate           *streamingMessageHandler
	completionCallback *SmartGuideCompletionCallback
}

// OnMessage forwards to delegate and tracks assistant messages
func (w *messageTrackingWrapper) OnMessage(msg interface{}) error {
	// Track assistant messages
	if m, ok := msg.(map[string]interface{}); ok {
		if msgType, ok := m["type"].(string); ok && msgType == "assistant" {
			w.completionCallback.messagesSent++
		}
	}
	// Forward to delegate
	return w.delegate.OnMessage(msg)
}

// OnError forwards to delegate
func (w *messageTrackingWrapper) OnError(err error) {
	w.delegate.OnError(err)
}

// OnComplete forwards to the completion callback.
func (w *messageTrackingWrapper) OnComplete(result *smart_guide.CompletionResult) {
	// Drain any trailing tool renders the stream buffered. The smart-guide loop
	// never emits a `result` frame to trigger handleMapMessage's flush, so
	// without this the last tool activity would be dropped before the banner.
	w.delegate.Flush()
	w.completionCallback.OnComplete(result)
}

// OnComplete handles the smart-guide completion signal.
func (c *SmartGuideCompletionCallback) OnComplete(result *smart_guide.CompletionResult) {
	// Capture the final assistant text from the agent's history.
	responseText := ""
	if result.Success {
		responseText = c.agent.LastAssistantText()
	}

	// Persist the agent's full updated history (user + assistant + tool turns)
	// as native Anthropic params. The engine owns history; we snapshot it here.
	if c.tbSessionStore != nil {
		if err := c.tbSessionStore.Save(c.hCtx.ChatID, c.agent.History()); err != nil {
			logrus.WithError(err).Warn("Failed to save SmartGuide session history")
		}
	}

	// Sync working directory state to ChatStore
	// This handles both change_workdir tool (which already persisted via updateProjectFunc)
	// and bash cd (which only updates ToolExecutor state, requiring sync here)
	//
	// Note: change_workdir tool immediately persists via updateProjectFunc (updates both ProjectPath and BashCwd)
	// The completion callback sync here is a safety net that ensures:
	// 1. bash cd changes are persisted (ToolExecutor -> ChatStore.BashCwd)
	// 2. change_workdir changes are correctly reflected (ChatStore already updated, condition prevents duplicate write)
	currentWorkingDir := c.agent.GetExecutor().GetWorkingDirectory()
	if currentWorkingDir != "" {
		// Get current stored values
		storedBashCwd, hasBashCwd, _ := c.chatStore.GetBashCwd(c.hCtx.ChatID)
		storedProjectPath, _, _ := c.chatStore.GetProjectPath(c.hCtx.ChatID)

		// Determine what needs to be updated
		updateProjectPath := (currentWorkingDir != storedProjectPath)
		updateBashCwd := (currentWorkingDir != storedBashCwd || !hasBashCwd)

		if updateProjectPath || updateBashCwd {
			logrus.WithFields(logrus.Fields{
				"chatID":            c.hCtx.ChatID,
				"currentWorkingDir": currentWorkingDir,
				"storedProjectPath": storedProjectPath,
				"storedBashCwd":     storedBashCwd,
				"updateProjectPath": updateProjectPath,
				"updateBashCwd":     updateBashCwd,
			}).Info("SmartGuide: Syncing working directory to chat store")

			if err := c.chatStore.UpdateChat(c.hCtx.ChatID, func(ch *bot.Chat) {
				// Only update ProjectPath if it actually changed (change_workdir case)
				if updateProjectPath {
					ch.ProjectPath = currentWorkingDir
				}
				// Always update BashCwd to track actual working directory (bash cd case)
				ch.BashCwd = currentWorkingDir
			}); err != nil {
				logrus.WithError(err).WithField("chatID", c.hCtx.ChatID).Warn("Failed to sync working directory to chat store")
			}
		}
	}

	// Update shared meta so footers reflect the new project path
	if currentWorkingDir != "" {
		c.meta.ProjectPath = currentWorkingDir
	}

	// NOTE: Response should be sent via OnMessage callbacks from hooks
	// However, if hooks failed to fire or agent completed without generating output,
	// we need a fallback to prevent silent completion (Issue #3)

	// Check if any assistant messages were sent via hooks
	if c.messagesSent == 0 && responseText != "" {
		logrus.WithFields(logrus.Fields{
			"chatID":       c.hCtx.ChatID,
			"responseLen":  len(responseText),
			"messagesSent": c.messagesSent,
		}).Warn("SmartGuide: No messages sent via hooks - using fallback to send response")

		// Send the response as a fallback (no meta for regular messages)
		if c.sendText != nil {
			c.sendText(c.hCtx, responseText)
		}
	} else if c.messagesSent == 0 && responseText == "" {
		logrus.WithFields(logrus.Fields{
			"chatID":  c.hCtx.ChatID,
			"success": result.Success,
		}).Warn("SmartGuide: Agent completed with NO output (possible crash or empty response)")
	} else {
		logrus.WithFields(logrus.Fields{
			"chatID":       c.hCtx.ChatID,
			"messagesSent": c.messagesSent,
		}).Debug("SmartGuide: Messages were sent via hooks - no fallback needed")
	}

	// Send the "Task done" action card on completion
	sendTaskDoneCard(c.hCtx, c.meta)

	// Log completion event
	logrus.WithFields(logrus.Fields{
		"chatID":   c.hCtx.ChatID,
		"success":  result.Success,
		"duration": result.DurationMS,
	}).Info("SmartGuide execution completed via callback")
}

// handleAgentMessage routes message to the appropriate agent handler
// Uses AgentRouter for clean delegation to agent executors
func (h *BotHandler) handleAgentMessage(hCtx HandlerContext, agent agentboot.AgentType, text string, projectPathOverride string) {
	logrus.WithFields(logrus.Fields{
		"agent":    agent,
		"chatID":   hCtx.ChatID,
		"senderID": hCtx.SenderID,
	}).Infof("Agent call: %s", text)

	req := ExecutionRequest{
		HCtx:             hCtx,
		Text:             text,
		ProjectPath:      projectPathOverride,
		ReplyToMessageID: hCtx.MessageID,
	}

	if err := h.agentRouter.Execute(h.ctx, agent, req); err != nil {
		logrus.WithError(err).Error("Agent execution failed via router")
		h.SendText(hCtx, executionErrorMessage(err))
		return
	}
	h.reactDone(hCtx)
}

// executionErrorMessage renders an agent execution error for the chat,
// substituting a guided message for the two session-conflict cases: our own
// duplicate-run guard (errExecutionBusy) and the Claude CLI's session-file
// lock (isSessionInUseText).
func executionErrorMessage(err error) string {
	if errors.Is(err, errExecutionBusy) || isSessionInUseText(err.Error()) {
		return "⚠️ **Session Busy**\n\nAnother execution is already in progress for this chat.\n\nPlease:\n• Wait for the current task to complete\n• Use `/stop` to cancel the current execution"
	}
	return fmt.Sprintf("⚠️ **Error**: %v", err)
}

// getCurrentAgent retrieves the current agent for a chat
// Returns "tingly-box" as default (Smart Guide is now the entry point)
func (h *BotHandler) getCurrentAgent(chatID string) (agentboot.AgentType, error) {
	currentAgent, err := h.chatStore.GetCurrentAgent(chatID)
	if err != nil {
		return agentTinglyBox, err
	}
	switch agentboot.AgentType(currentAgent) {
	case agentTinglyBox, agentClaudeCode, agentMock:
		return agentboot.AgentType(currentAgent), nil
	default:
		return agentTinglyBox, nil
	}
}

// setCurrentAgent sets the current agent for a chat. The platform is
// forwarded so fresh chats — those not yet created by /cd or /bind — get
// a row with the correct platform on the first handoff.
func (h *BotHandler) setCurrentAgent(chatID, platform string, agentType agentboot.AgentType) error {
	return h.chatStore.SetCurrentAgent(chatID, platform, string(agentType))
}

// handleHandoff performs a handoff from one agent to another
func (h *BotHandler) handleHandoff(hCtx HandlerContext, toAgent agentboot.AgentType) error {
	// Get current agent
	fromAgent, err := h.getCurrentAgent(hCtx.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get current agent: %w", err)
	}

	// Get project path
	projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)

	// Create handoff state (no sessionID needed - sessions are managed per-agent)
	handoffState := &smart_guide.HandoffState{
		FromAgent:   string(fromAgent),
		ToAgent:     string(toAgent),
		Timestamp:   time.Now(),
		ProjectPath: projectPath,
		ChatID:      hCtx.ChatID,
	}

	// Execute handoff
	result := h.handoffManager.ExecuteHandoff(h.ctx, handoffState)
	if !result.Success {
		return fmt.Errorf("handoff failed: %s", result.Error)
	}

	// Update current agent in chat store. Pass platform so a brand-new
	// chat gets created with the right platform — without this, UpdateChat
	// silently no-ops on a missing row and the handoff fails to stick.
	if err := h.setCurrentAgent(hCtx.ChatID, string(hCtx.Platform), toAgent); err != nil {
		logrus.WithError(err).Error("Failed to update current agent after handoff")
	}

	// Note: Session context update removed - sessions are now managed per-(chat, agent, project)
	// The target agent will find/create its own session when it processes the next message

	logrus.WithFields(logrus.Fields{
		"chatID":    hCtx.ChatID,
		"fromAgent": fromAgent,
		"toAgent":   toAgent,
		"project":   projectPath,
	}).Info("Agent handoff completed")

	// Send handoff confirmation (skip empty messages from same-agent handoff)
	if result.Message != "" {
		h.SendText(hCtx, result.Message)
	}

	return nil
}

// routeToAgent routes a message to the appropriate agent based on current_agent
func (h *BotHandler) routeToAgent(hCtx HandlerContext, text string) error {
	// Check for handoff commands first (supports "@cc help me" format)
	if toAgent, isHandoff, remainingText := smart_guide.DetectHandoffCommand(text); isHandoff {
		// smart_guide names the handoff targets with the same identity strings
		// this package routes on, so a typed conversion plus a validity check
		// replaces the old value-by-value re-mapping.
		targetAgent := agentboot.AgentType(toAgent)
		switch targetAgent {
		case agentTinglyBox, agentClaudeCode:
		default:
			return fmt.Errorf("unknown target agent: %s", toAgent)
		}

		// Perform handoff
		if err := h.handleHandoff(hCtx, targetAgent); err != nil {
			return err
		}

		// If there's remaining text, process it immediately with the new agent
		if remainingText != "" {
			logrus.WithFields(logrus.Fields{
				"chatID":        hCtx.ChatID,
				"targetAgent":   targetAgent,
				"remainingText": remainingText,
			}).Info("Processing remaining text after handoff")

			return h.agentRouter.Execute(h.ctx, targetAgent, ExecutionRequest{
				HCtx:             hCtx,
				Text:             remainingText,
				ReplyToMessageID: hCtx.MessageID,
			})
		}

		return nil
	}

	// Get current agent
	currentAgent, err := h.getCurrentAgent(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to get current agent, defaulting to smart guide")
		currentAgent = agentTinglyBox
	}

	// Route to current agent via AgentRouter
	execErr := h.agentRouter.Execute(h.ctx, currentAgent, ExecutionRequest{
		HCtx:             hCtx,
		Text:             text,
		ReplyToMessageID: hCtx.MessageID,
	})
	if execErr != nil {
		logrus.WithError(execErr).Error("Agent execution failed via router")
		return execErr
	}
	return nil
}

// getProjectPath returns the project path bound to the current chat (direct
// and group chats share the same chat-id key in the store).
func (h *BotHandler) getProjectPath(hCtx HandlerContext) (string, bool) {
	projectPath, hasBound, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	if hasBound && projectPath != "" {
		return projectPath, true
	}
	return "", false
}

// defaultProjectPath returns the bot's default project path. The resolution
// (DefaultCwd → cwd → home → "/") lives on ExecutorDependencies so it reads
// the CURRENT settings — an operator's DefaultCwd change applies without a
// bot restart, here as on the execution path.
func (h *BotHandler) defaultProjectPath() string {
	return h.agentRouter.deps.ResolveDefaultProjectPath()
}
