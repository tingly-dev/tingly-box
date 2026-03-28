package bot

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/memory"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
)

type CompletionCallback struct {
	hCtx       HandlerContext
	sessionID  string
	sessionMgr *session.Manager
	meta       *ResponseMeta
}

func (c *CompletionCallback) OnComplete(result *agentboot.CompletionResult) {
	// Update session status based on completion result
	if c.sessionMgr != nil && c.sessionID != "" {
		if result.Success {
			c.sessionMgr.SetCompleted(c.sessionID, "")
		} else {
			c.sessionMgr.SetFailed(c.sessionID, result.Error)
		}
	}

	// Build action keyboard
	kb := BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	doneText := IconDone + " " + MsgTaskDone + ". \n" + MsgContinueOrHelp + BuildFooter(c.meta.AgentType, c.meta.ProjectPath)
	_, err := c.hCtx.Bot.SendMessage(context.Background(), c.hCtx.ChatID, &imbot.SendMessageOptions{
		Text: doneText,
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to send action keyboard")
	}

	// Log completion event
	logrus.WithFields(logrus.Fields{
		"chatID":    c.hCtx.ChatID,
		"sessionID": c.sessionID,
		"success":   result.Success,
		"duration":  result.DurationMS,
		"error":     result.Error,
	}).Info("Agent execution completed via callback")
}

// SmartGuideCompletionCallback handles completion events for SmartGuide agent
// It saves messages to session, updates project path if changed, and sends response + action keyboard
type SmartGuideCompletionCallback struct {
	hCtx           HandlerContext
	sessionID      string
	chatStore      ChatStoreInterface
	tbSessionStore *smart_guide.SessionStore
	agent          *smart_guide.TinglyBoxAgent
	projectPath    string
	meta           *ResponseMeta
	behavior       OutputBehavior
	botHandler     *BotHandler // Add reference to bot handler for formatting
	messagesSent   int         // Track number of messages sent via hooks (for fallback)
}

// messageTrackingWrapper wraps a message handler and tracks assistant messages
// This is used to detect silent completions (Issue #3)
type messageTrackingWrapper struct {
	delegate           *streamingMessageHandler // Concrete type, not interface
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

// GetOutput forwards to delegate
func (w *messageTrackingWrapper) GetOutput() string {
	return w.delegate.GetOutput()
}

// OnComplete implements agentboot.CompletionCallback
func (c *SmartGuideCompletionCallback) OnComplete(result *agentboot.CompletionResult) {
	// Get response text from the agent's memory
	responseText := ""
	if result.Success {
		// Try to get the last assistant message from agent memory
		if mem := c.agent.GetMemory(); mem != nil {
			if hist, ok := mem.(*memory.History); ok {
				messages := hist.GetMessages()
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "assistant" {
						responseText = messages[i].GetTextContent()
						break
					}
				}
			}
		}
	}

	// Save assistant message to session (for conversation history)
	if c.tbSessionStore != nil && responseText != "" {
		assistantMsg := message.NewMsg("assistant", responseText, types.RoleAssistant)
		if err := c.tbSessionStore.AddMessage(c.hCtx.ChatID, assistantMsg); err != nil {
			logrus.WithError(err).Warn("Failed to save assistant message to session")
		}
	}

	// Check if working directory was changed by change_workdir tool
	newProjectPath := c.agent.GetExecutor().GetWorkingDirectory()
	if newProjectPath != c.projectPath {
		logrus.WithFields(logrus.Fields{
			"chatID":         c.hCtx.ChatID,
			"oldProjectPath": c.projectPath,
			"newProjectPath": newProjectPath,
		}).Info("Project path changed, updating chat store")

		// Update chat store with new project path
		chat, err := c.chatStore.GetChat(c.hCtx.ChatID)
		if err != nil {
			logrus.WithError(err).WithField("chatID", c.hCtx.ChatID).Warn("Failed to get chat for update")
		}

		if chat == nil {
			now := time.Now().UTC()
			chat = &Chat{
				ChatID:      c.hCtx.ChatID,
				Platform:    string(c.hCtx.Platform),
				ProjectPath: newProjectPath,
				BashCwd:     newProjectPath,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := c.chatStore.UpsertChat(chat); err != nil {
				logrus.WithError(err).WithField("chatID", c.hCtx.ChatID).Warn("Failed to create chat")
			}
		} else {
			if err := c.chatStore.UpdateChat(c.hCtx.ChatID, func(ch *Chat) {
				ch.ProjectPath = newProjectPath
				ch.BashCwd = newProjectPath
			}); err != nil {
				logrus.WithError(err).WithField("chatID", c.hCtx.ChatID).Warn("Failed to update chat")
			}
		}
	}

	// Update shared meta so footers reflect the new project path
	c.meta.ProjectPath = newProjectPath

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
		formattedResponse := c.botHandler.formatResponseWithHeader(*c.meta, responseText, false)
		c.botHandler.SendText(c.hCtx, formattedResponse)
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

	// Send action keyboard on completion
	kb := BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	// Build metadata with context_token (required by Weixin)
	metadata := map[string]interface{}{
		"replyMarkup":        tgKeyboard,
		"_trackActionMenuID": true,
	}
	// Forward context_token from incoming message metadata (required by Weixin)
	if c.hCtx.Message.Metadata != nil {
		if ct, ok := c.hCtx.Message.Metadata["context_token"].(string); ok {
			metadata["context_token"] = ct
		}
	}

	sgDoneText := IconDone + " " + MsgTaskDone + ". \n" + MsgContinueOrHelp + BuildFooter(c.meta.AgentType, c.meta.ProjectPath)
	_, err := c.hCtx.Bot.SendMessage(context.Background(), c.hCtx.ChatID, &imbot.SendMessageOptions{
		Text:     sgDoneText,
		Metadata: metadata,
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to send action keyboard for SmartGuide")
	}

	// Log completion event
	logrus.WithFields(logrus.Fields{
		"chatID":    c.hCtx.ChatID,
		"sessionID": c.sessionID,
		"success":   result.Success,
		"duration":  result.DurationMS,
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

	_, err := h.agentRouter.Execute(h.ctx, agent, req)
	if err != nil {
		logrus.WithError(err).Error("Agent execution failed via router")
		h.SendText(hCtx, fmt.Sprintf("Agent execution failed: %v", err))
	}
}

// getCurrentAgent retrieves the current agent for a chat
// Returns "tingly-box" as default (Smart Guide is now the entry point)
func (h *BotHandler) getCurrentAgent(chatID string) (agentboot.AgentType, error) {
	currentAgent, err := h.chatStore.GetCurrentAgent(chatID)
	if err != nil {
		return agentTinglyBox, err
	}

	// Return the stored agent type
	if currentAgent == string(agentTinglyBox) {
		return agentTinglyBox, nil
	}
	if currentAgent == string(agentClaudeCode) {
		return agentClaudeCode, nil
	}
	if currentAgent == string(agentMock) {
		return agentMock, nil
	}

	return agentTinglyBox, nil
}

// setCurrentAgent sets the current agent for a chat
func (h *BotHandler) setCurrentAgent(chatID string, agentType agentboot.AgentType) error {
	return h.chatStore.SetCurrentAgent(chatID, string(agentType))
}

// handleHandoff performs a handoff from one agent to another
func (h *BotHandler) handleHandoff(hCtx HandlerContext, toAgent agentboot.AgentType) error {
	// Get current agent
	fromAgent, err := h.getCurrentAgent(hCtx.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get current agent: %w", err)
	}

	// Get project path
	projectPath, _, _ := h.getProjectPathForChat(hCtx)
	if projectPath == "" {
		projectPath, _, _ = h.chatStore.GetProjectPath(hCtx.ChatID)
	}

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

	// Update current agent in chat store
	if err := h.setCurrentAgent(hCtx.ChatID, toAgent); err != nil {
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

	// Send handoff confirmation
	h.SendText(hCtx, result.Message)

	return nil
}

// routeToAgent routes a message to the appropriate agent based on current_agent
func (h *BotHandler) routeToAgent(hCtx HandlerContext, text string) error {
	// Check for handoff commands first (supports "@cc help me" format)
	if toAgent, isHandoff, remainingText := smart_guide.DetectHandoffCommand(text); isHandoff {
		// Determine target agent by comparing string values
		var targetAgent agentboot.AgentType
		switch string(toAgent) {
		case smart_guide.AgentTypeTinglyBox:
			targetAgent = agentTinglyBox
		case smart_guide.AgentTypeClaudeCode:
			targetAgent = agentClaudeCode
		case smart_guide.AgentTypeMock:
			targetAgent = agentMock
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

			req := ExecutionRequest{
				HCtx:             hCtx,
				Text:             remainingText,
				ReplyToMessageID: hCtx.MessageID,
			}

			_, execErr := h.agentRouter.Execute(h.ctx, targetAgent, req)
			return execErr
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
	req := ExecutionRequest{
		HCtx:             hCtx,
		Text:             text,
		ReplyToMessageID: hCtx.MessageID,
	}

	_, execErr := h.agentRouter.Execute(h.ctx, currentAgent, req)
	if execErr != nil {
		logrus.WithError(execErr).Error("Agent execution failed via router")
		return execErr
	}
	return nil
}

// getProjectPathForChat gets the project path for a chat
func (h *BotHandler) getProjectPathForChat(hCtx HandlerContext) (string, bool, error) {
	// Try direct chat first
	if hCtx.IsDirect() {
		projectPath, ok, err := h.chatStore.GetProjectPath(hCtx.ChatID)
		return projectPath, ok, err
	}

	// Try group chat
	projectPath, ok, err := h.chatStore.GetProjectPath(hCtx.ChatID)
	return projectPath, ok, err
}

// getProjectPath returns the project path for the current chat
func (h *BotHandler) getProjectPath(hCtx HandlerContext) (string, bool) {
	if hCtx.IsDirect() {
		// Direct chat: get project path from Chat store
		projectPath, hasBound, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		if hasBound && projectPath != "" {
			return projectPath, true
		}
	} else {
		// Group chat: get bound project path
		return getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform))
	}
	return "", false
}

// getDefaultProjectPath returns the default project path for the bot
// Priority: 1. DefaultCwd from bot setting, 2. Current working directory, 3. User home directory
func (h *BotHandler) getDefaultProjectPath() string {
	// 1. Check bot setting's DefaultCwd
	if h.botSetting.DefaultCwd != "" {
		expanded, err := ExpandPath(h.botSetting.DefaultCwd)
		if err == nil {
			return expanded
		}
		logrus.WithError(err).Warnf("Failed to expand DefaultCwd: %s", h.botSetting.DefaultCwd)
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
