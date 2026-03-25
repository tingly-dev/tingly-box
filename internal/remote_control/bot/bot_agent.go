package bot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/memory"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	mock "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type CompletionCallback struct {
	hCtx       HandlerContext
	sessionID  string
	sessionMgr *session.Manager
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

	_, err := c.hCtx.Bot.SendMessage(context.Background(), c.hCtx.ChatID, &imbot.SendMessageOptions{
		Text: "✅ Task done. \nContinue to chat with this session or /help.",
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
	meta           ResponseMeta
	behavior       OutputBehavior
	botHandler     *BotHandler // Add reference to bot handler for formatting
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

	// NOTE: Don't send response text here - it's already sent via OnMessage callbacks from the hook
	// Only send the completion action keyboard

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

	_, err := c.hCtx.Bot.SendMessage(context.Background(), c.hCtx.ChatID, &imbot.SendMessageOptions{
		Text:     "✅ Task done. \nContinue to chat with this session or /help.",
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
func (h *BotHandler) handleAgentMessage(hCtx HandlerContext, agent agentboot.AgentType, text string, projectPathOverride string) {
	logrus.WithFields(logrus.Fields{
		"agent":    agent,
		"chatID":   hCtx.ChatID,
		"senderID": hCtx.SenderID,
	}).Infof("Agent call: %s", text)

	switch agent {
	case agentClaudeCode:
		h.handleClaudeCodeMessage(hCtx, text, projectPathOverride)
	case agentMock:
		h.handleMockAgentMessage(hCtx, text, projectPathOverride)
	default:
		h.SendText(hCtx, fmt.Sprintf("Unknown agent: %s", agent))
	}
}

// handleClaudeCodeMessage executes a message through Claude Code
func (h *BotHandler) handleClaudeCodeMessage(hCtx HandlerContext, text string, projectPathOverride string) {
	if strings.TrimSpace(text) == "" {
		h.SendText(hCtx, "Please provide a message for Claude Code.")
		return
	}

	// Determine project path FIRST: priority is override > bound project > default cwd
	projectPath := projectPathOverride
	if projectPath == "" {
		boundPath, hasBound, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		if hasBound && boundPath != "" {
			projectPath = boundPath
		}
	}
	// Use default cwd if no project bound
	if projectPath == "" {
		projectPath = h.getDefaultProjectPath()
		logrus.WithFields(logrus.Fields{
			"chatID":     hCtx.ChatID,
			"defaultCwd": projectPath,
		}).Info("Using default cwd for Claude Code")
	}

	// NEW: Find session by (chatID, agent, project) tuple
	// This ensures we resume the correct session for the current project
	agentType := "claude" // Claude Code agent type
	sess := h.sessionMgr.FindBy(hCtx.ChatID, agentType, projectPath)

	// Track if this is a new session or resuming an existing one
	isNewSession := false

	// Auto-create session if none exists or if session is in pending state (stale)
	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed || sess.Status == session.StatusPending {
		sess = h.sessionMgr.CreateWith(hCtx.ChatID, agentType, projectPath)
		// Clear expiration for persistent sessions
		h.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{}        // Zero value means no expiration
			s.Status = session.StatusRunning // Mark as running immediately
		})
		isNewSession = true

		logrus.WithFields(logrus.Fields{
			"chatID":    hCtx.ChatID,
			"sessionID": sess.ID,
			"project":   projectPath,
			"agent":     agentType,
		}).Info("Created new session for Claude Code")
	} else {
		// Reset status to running for reused sessions (e.g., completed/failed sessions)
		h.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.Status = session.StatusRunning
		})
		logrus.WithFields(logrus.Fields{
			"chatID":    hCtx.ChatID,
			"sessionID": sess.ID,
			"project":   projectPath,
			"agent":     agentType,
			"status":    sess.Status,
		}).Info("Resumed existing session for Claude Code")
	}

	sessionID := sess.ID

	// Refresh session activity
	if sess != nil {
		h.sessionMgr.Update(sessionID, func(s *session.Session) {
			s.LastActivity = time.Now()
		})
	}

	// Build meta
	meta := ResponseMeta{
		ProjectPath: projectPath,
		AgentType:   string(agentboot.AgentTypeClaude),
		SessionID:   sessionID,
		ChatID:      hCtx.ChatID,
		UserID:      hCtx.SenderID,
	}

	h.sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	})

	//// Check if session is already running (prevent concurrent execution)
	//if sess.Status == session.StatusRunning {
	//	h.SendText(hCtx, "⚠️ A task is currently running.\n\nUse `stop` or `/stop` to cancel it first.")
	//	return
	//}

	h.sessionMgr.SetRunning(sessionID)

	// Send status message - differentiate between new and resumed sessions
	behavior := h.getOutputBehavior()
	var statusMsg string
	if isNewSession {
		statusMsg = "⏳ CC: Processing new session..."
	} else {
		statusMsg = "⏳ CC: Resuming session..."
	}
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, statusMsg, behavior), hCtx.MessageID)

	// Execute with context.Background() to avoid cancellation on reconnect
	execCtx, cancel := context.WithCancel(context.Background())

	// Store cancel function for /stop command
	h.runningCancelMu.Lock()
	h.runningCancel[hCtx.ChatID] = cancel
	h.runningCancelMu.Unlock()

	// Clean up cancel function when done
	defer func() {
		h.runningCancelMu.Lock()
		delete(h.runningCancel, hCtx.ChatID)
		h.runningCancelMu.Unlock()
		cancel()
	}()

	agent, err := h.agentBoot.GetDefaultAgent()
	if err != nil {
		h.sessionMgr.SetFailed(sessionID, "agent not available: "+err.Error())
		h.sendTextWithReply(hCtx, "Agent not available", hCtx.MessageID)
		return

	}

	// Determine if we should resume
	shouldResume := false
	if msgs, ok := h.sessionMgr.GetMessages(sessionID); ok && len(msgs) > 1 {
		shouldResume = true
	}

	logrus.WithFields(logrus.Fields{
		"chatID":       hCtx.ChatID,
		"sessionID":    sessionID,
		"projectPath":  projectPath,
		"shouldResume": shouldResume,
	}).Info("Starting agent execution")

	// Create streaming handler for message output
	streamHandler := h.newStreamingMessageHandler(hCtx)

	// Check permission mode for this session
	permissionMode := sess.PermissionMode
	if permissionMode == "" {
		permissionMode = string(claude.PermissionModeDefault)
	}

	// Create composite handler that combines streaming + approval + ask handling
	// In auto mode (yolo), skip approval handler to auto-approve all permissions
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(streamHandler).
		SetCompletionCallback(&CompletionCallback{hCtx: hCtx, sessionID: sessionID, sessionMgr: h.sessionMgr})

	if permissionMode != string(claude.PermissionModeAuto) {
		// Normal mode: use approval handler
		compositeHandler.SetApprovalHandler(h.imPrompter).
			SetAskHandler(h.imPrompter)
	}

	result, err := agent.Execute(execCtx, text, agentboot.ExecutionOptions{
		ProjectPath:          projectPath,
		Handler:              compositeHandler,
		SessionID:            sessionID,
		Resume:               shouldResume,
		ChatID:               hCtx.ChatID,
		Platform:             string(hCtx.Platform),
		BotUUID:              hCtx.BotUUID,
		PermissionPromptTool: "stdio",
		PermissionMode:       permissionMode,
	})

	logrus.WithFields(logrus.Fields{
		"chatID":    hCtx.ChatID,
		"sessionID": sessionID,
		"hasError":  err != nil,
		"hasResult": result != nil,
	}).Info("Agent execution completed")

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
		h.sessionMgr.SetFailed(sessionID, response)
		logrus.WithError(err).WithFields(logrus.Fields{
			"chatID":    hCtx.ChatID,
			"sessionID": sessionID,
			"response":  response,
		}).Warn("Remote-coder execution failed")

		if response == "" {
			response = fmt.Sprintf("Execution failed: %v", err)
		}
		h.sendTextWithReply(hCtx, response, hCtx.MessageID)
		return
	}

	h.sessionMgr.SetCompleted(sessionID, response)

	h.sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	h.sendTextWithActionKeyboard(hCtx, response, hCtx.MessageID)
}

// handleSmartGuideMessage handles a message for the smart guide agent
// Loads conversation history from session file, processes message, and saves back
func (h *BotHandler) handleSmartGuideMessage(hCtx HandlerContext, text string) error {
	// Get current project path from chat store
	projectPath, hasPath, err := h.chatStore.GetProjectPath(hCtx.ChatID)
	logrus.WithFields(logrus.Fields{
		"chatID":      hCtx.ChatID,
		"projectPath": projectPath,
		"hasPath":     hasPath,
	}).Info("Loaded project path from chat store")

	if projectPath == "" {
		projectPath = h.getDefaultProjectPath()
		logrus.WithField("defaultPath", projectPath).Info("Using default project path")
	}

	// 1. Load messages from session store
	var messages []*message.Msg
	if h.tbSessionStore != nil {
		messages, err = h.tbSessionStore.Load(hCtx.ChatID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to load session, starting with empty history")
			messages = nil
		}

		logrus.WithFields(logrus.Fields{
			"chatID":       hCtx.ChatID,
			"historyCount": len(messages),
		}).Info("Loaded SmartGuide messages")
	}
	// else: messages remains nil, which is fine

	// 2. Resolve HTTP endpoint configuration for SmartGuide
	var baseURL, apiKey string
	if h.tbClient != nil {
		endpoint, err := h.tbClient.GetHTTPEndpointForScenario(h.ctx, typ.ScenarioSmartGuide)
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
		Provider:         h.botSetting.SmartGuideProvider,
		Model:            h.botSetting.SmartGuideModel,
		// Approval context
		Handler:  agentboot.NewCompositeHandler().SetApprovalHandler(h.imPrompter),
		ChatID:   hCtx.ChatID,
		Platform: string(hCtx.Platform),
		BotUUID:  h.botSetting.UUID,
		GetStatusFunc: func(chatID string) (*smart_guide.StatusInfo, error) {
			projectPath, _, _ := h.chatStore.GetProjectPath(chatID)
			workingDir, hasWD, _ := h.chatStore.GetBashCwd(chatID)
			if !hasWD {
				workingDir = projectPath
			}

			return &smart_guide.StatusInfo{
				CurrentAgent:   "tingly-box",
				SessionID:      hCtx.ChatID, // Use chatID as session identifier
				ProjectPath:    projectPath,
				WorkingDir:     workingDir,
				HasRunningTask: false,
				Whitelisted:    h.chatStore.IsWhitelisted(chatID),
			}, nil
		},
		GetProjectFunc: func(chatID string) (string, bool, error) {
			return h.chatStore.GetProjectPath(chatID)
		},
		UpdateProjectFunc: func(chatID string, newProjectPath string) error {
			logrus.WithFields(logrus.Fields{
				"chatID":  chatID,
				"oldPath": projectPath,
				"newPath": newProjectPath,
			}).Info("updateProjectFunc called - persisting to chat store")
			return h.chatStore.UpdateChat(chatID, func(chat *Chat) {
				chat.ProjectPath = newProjectPath
				chat.BashCwd = newProjectPath
			})
		},
	}

	// 4. Create agent with history
	agent, err := smart_guide.NewTinglyBoxAgentWithSession(agentConfig, messages)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create Smart Guide agent, falling back to Claude Code")

		// Send warning notification to user before switching
		h.SendText(hCtx, "⚠️ Smart Guide (@tb) is currently unavailable due to configuration issues.\n"+
			"Reason: "+err.Error()+"\n"+
			"Automatically switching to Claude Code (@cc) to continue your work.\n"+
			"Type '@tb' to return to Smart Guide once the issue is resolved.\n"+
			"Type '/help' for available commands.")

		// Automatically switch to Claude Code agent
		if err := h.handleHandoff(hCtx, agentClaudeCode); err != nil {
			logrus.WithError(err).Error("Failed to fallback to Claude Code")
			h.SendText(hCtx, "⚠️ Smart Guide unavailable and failed to switch to Claude Code. Please check your configuration.")
			return fmt.Errorf("smart guide failed and fallback to claude code failed: %w", err)
		}

		// Route the message to Claude Code with the original text
		projectPath, _, _ := h.getProjectPathForChat(hCtx)
		logrus.WithFields(logrus.Fields{
			"chatID":      hCtx.ChatID,
			"projectPath": projectPath,
			"textLength":  len(text),
		}).Info("Routing message to Claude Code after Smart Guide fallback")

		h.handleAgentMessage(hCtx, agentClaudeCode, text, projectPath)
		return nil
	}

	// Set working directory from stored project path
	agent.GetExecutor().SetWorkingDirectory(projectPath)
	logrus.WithField("workingDir", projectPath).Debug("Set executor working directory")

	// 5. Create tool context
	toolCtx := &smart_guide.ToolContext{
		ChatID:      hCtx.ChatID,
		ProjectPath: projectPath,
		SessionID:   hCtx.ChatID, // Use chatID as session identifier
	}

	// 6. Build meta for response header
	behavior := h.getOutputBehavior()
	meta := ResponseMeta{
		ProjectPath: projectPath,
		ChatID:      hCtx.ChatID,
		UserID:      hCtx.SenderID,
		SessionID:   hCtx.ChatID, // Use chatID as session identifier
		AgentType:   AgentNameTinglyBox,
	}

	// 7. Send processing message (always send, regardless of verbose mode)
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, IconProcess+" "+MsgProcessing, behavior), hCtx.MessageID)

	// 8. Create streaming handler for message output
	streamHandler := h.newStreamingMessageHandler(hCtx)

	// 9. Create composite handler that combines streaming + completion callback
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(streamHandler).
		SetCompletionCallback(&SmartGuideCompletionCallback{
			hCtx:           hCtx,
			sessionID:      hCtx.ChatID,
			chatStore:      h.chatStore,
			tbSessionStore: h.tbSessionStore,
			agent:          agent,
			projectPath:    projectPath,
			meta:           meta,
			behavior:       behavior,
			botHandler:     h,
		})

	// 10. Execute with callback support
	execCtx, cancel := context.WithCancel(context.Background())

	// Store cancel function for /stop command
	h.runningCancelMu.Lock()
	h.runningCancel[hCtx.ChatID] = cancel
	h.runningCancelMu.Unlock()

	// Clean up cancel function when done
	defer func() {
		h.runningCancelMu.Lock()
		delete(h.runningCancel, hCtx.ChatID)
		h.runningCancelMu.Unlock()
		cancel()
	}()

	// Save user message to session before execution
	if h.tbSessionStore != nil {
		userMsg := message.NewMsg("user", text, types.RoleUser)
		if err := h.tbSessionStore.AddMessage(hCtx.ChatID, userMsg); err != nil {
			logrus.WithError(err).Warn("Failed to save user message to session")
		}
	}

	// Execute with handler
	result, err := agent.ExecuteWithHandler(execCtx, text, toolCtx, compositeHandler)
	if err != nil {
		logrus.WithError(err).Error("Smart guide agent failed")
		h.SendText(hCtx, fmt.Sprintf("%s Error: %v", IconError, err))
		return nil
	}

	// Note: Response is sent by the CompletionCallback to ensure proper order
	// The callback sends both the response and the action keyboard together
	logrus.WithFields(logrus.Fields{
		"chatID":  hCtx.ChatID,
		"success": result != nil,
	}).Info("SmartGuide execution completed")

	return nil
}

// handleMockAgentMessage executes a message through the mock agent for testing
func (h *BotHandler) handleMockAgentMessage(hCtx HandlerContext, text string, projectPathOverride string) {
	if strings.TrimSpace(text) == "" {
		h.SendText(hCtx, "Please provide a message for the mock agent.")
		return

	}

	// Get project path
	projectPath := projectPathOverride
	if projectPath == "" {
		boundPath, hasBound, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		if hasBound && boundPath != "" {
			projectPath = boundPath
		}
	}
	if projectPath == "" {
		projectPath = h.getDefaultProjectPath()
	}

	// Find or create session for mock agent
	agentType := "mock"
	sess := h.sessionMgr.FindBy(hCtx.ChatID, agentType, projectPath)

	// Track if this is a new session or resuming an existing one
	isNewSession := false

	// Create new session if needed (including pending state sessions)
	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed || sess.Status == session.StatusPending {
		sess = h.sessionMgr.CreateWith(hCtx.ChatID, agentType, projectPath)
		// Clear expiration for persistent sessions
		h.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{}
			s.Status = session.StatusRunning
		})
		isNewSession = true
	} else {
		// Reset status to running for reused sessions
		h.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.Status = session.StatusRunning
		})
	}
	sessionID := sess.ID

	// Build meta
	meta := ResponseMeta{
		ProjectPath: projectPath,
		AgentType:   string(agentboot.AgentTypeMockAgent),
		SessionID:   sessionID,
		ChatID:      hCtx.ChatID,
		UserID:      hCtx.SenderID,
	}

	h.sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	})

	// Check if session is already running (prevent concurrent execution)
	if sess.Status == session.StatusRunning {
		h.SendText(hCtx, "⚠️ A task is currently running.\n\nUse `stop` or `/stop` to cancel it first.")
		return
	}

	h.sessionMgr.SetRunning(sessionID)

	// Send status message - differentiate between new and resumed sessions
	behavior := h.getOutputBehavior()
	var statusMsg string
	if isNewSession {
		statusMsg = "🧪 Mock: Processing new session..."
	} else {
		statusMsg = "🧪 Mock: Resuming session..."
	}
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, statusMsg, behavior), hCtx.MessageID)

	// Execute with context
	execCtx, cancel := context.WithCancel(context.Background())

	// Store cancel function for /stop command
	h.runningCancelMu.Lock()
	h.runningCancel[hCtx.ChatID] = cancel
	h.runningCancelMu.Unlock()

	// Clean up cancel function when done
	defer func() {
		h.runningCancelMu.Lock()
		delete(h.runningCancel, hCtx.ChatID)
		h.runningCancelMu.Unlock()
		cancel()
	}()

	// Get mock agent
	mockAgent, err := h.agentBoot.GetAgent(agentboot.AgentTypeMockAgent)
	if err != nil {
		// Register mock agent if not exists
		newMockAgent := mock.NewAgent(mock.Config{
			MaxIterations: 3,
			StepDelay:     2 * time.Second,
		})
		h.agentBoot.RegisterAgent(agentboot.AgentTypeMockAgent, newMockAgent)
		mockAgent = newMockAgent
	}

	logrus.WithFields(logrus.Fields{
		"chatID":    hCtx.ChatID,
		"sessionID": sessionID,
		"agent":     "mock",
	}).Info("Starting mock agent execution")

	// Create streaming handler for message output
	streamHandler := h.newStreamingMessageHandler(hCtx)

	// Create composite handler that combines streaming + approval + ask handling
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(streamHandler).
		SetApprovalHandler(h.imPrompter).
		SetAskHandler(h.imPrompter)

	result, err := mockAgent.Execute(execCtx, text, agentboot.ExecutionOptions{
		ProjectPath: projectPath,
		Handler:     compositeHandler,
		SessionID:   sessionID,
		ChatID:      hCtx.ChatID,
		Platform:    string(hCtx.Platform),
		BotUUID:     hCtx.BotUUID,
	})

	logrus.WithFields(logrus.Fields{
		"chatID":    hCtx.ChatID,
		"sessionID": sessionID,
		"hasError":  err != nil,
		"hasResult": result != nil,
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
		h.sessionMgr.SetFailed(sessionID, response)
		logrus.WithError(err).WithFields(logrus.Fields{
			"chatID":    hCtx.ChatID,
			"sessionID": sessionID,
			"response":  response,
		}).Warn("Mock agent execution failed")

		if response == "" {
			response = fmt.Sprintf("Execution failed: %v", err)
		}
		h.sendTextWithReply(hCtx, response, hCtx.MessageID)
		return

	}

	h.sessionMgr.SetCompleted(sessionID, response)

	h.sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	h.sendTextWithActionKeyboard(hCtx, response, hCtx.MessageID)
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

			// Route the remaining text to the new agent
			switch targetAgent {
			case agentTinglyBox:
				err := h.handleSmartGuideMessage(hCtx, remainingText)
				return err
			case agentClaudeCode:
				projectPath, _, _ := h.getProjectPathForChat(hCtx)
				h.handleAgentMessage(hCtx, agentClaudeCode, remainingText, projectPath)
				return nil
			case agentMock:
				projectPath, _, _ := h.getProjectPathForChat(hCtx)
				h.handleAgentMessage(hCtx, agentMock, remainingText, projectPath)
				return nil
			}
		}

		return nil
	}

	// Get current agent
	currentAgent, err := h.getCurrentAgent(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to get current agent, defaulting to smart guide")
		currentAgent = agentTinglyBox
	}

	// Route to appropriate agent
	switch currentAgent {
	case agentTinglyBox:
		return h.handleSmartGuideMessage(hCtx, text)
	case agentClaudeCode:
		// Get project path
		projectPath, _, _ := h.getProjectPathForChat(hCtx)
		h.handleAgentMessage(hCtx, agentClaudeCode, text, projectPath)
		return nil
	case agentMock:
		// Get project path
		projectPath, _, _ := h.getProjectPathForChat(hCtx)
		h.handleAgentMessage(hCtx, agentMock, text, projectPath)
		return nil
	default:
		return fmt.Errorf("unknown agent type: %s", currentAgent)
	}
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
