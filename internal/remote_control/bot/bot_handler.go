package bot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	mock "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/remote_control/summarizer"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// BotHandler encapsulates all bot message handling logic and dependencies
type BotHandler struct {
	ctx              context.Context
	botSetting       BotSetting
	chatStore        ChatStoreInterface // Use interface for flexibility
	sessionMgr       *session.Manager
	agentBoot        *agentboot.AgentBoot
	summaryEngine    *summarizer.Engine
	directoryBrowser *DirectoryBrowser
	manager          *imbot.Manager
	imPrompter       *IMPrompter
	fileStore        *FileStore
	interaction      *imbot.InteractionHandler // New interaction handler
	tbClient         tbclient.TBClient         // TB Client for model configuration

	// Handoff manager for agent switching
	handoffManager *smart_guide.HandoffManager

	// SmartGuide session store for conversation history
	tbSessionStore *smart_guide.SessionStore

	// runningCancel tracks cancel functions for active executions per chatID
	runningCancel   map[string]context.CancelFunc
	runningCancelMu sync.RWMutex

	// pendingBinds tracks bind confirmation requests for unbound chats
	pendingBinds   map[string]*PendingBind
	pendingBindsMu sync.RWMutex

	// actionMenuMessageID tracks the message ID of the action keyboard menu per chatID
	actionMenuMessageID   map[string]string
	actionMenuMessageIDMu sync.RWMutex
}

// PendingBind represents a pending bind confirmation request
type PendingBind struct {
	OriginalMessage string
	ProposedPath    string
	ExpiresAt       time.Time
}

// HandlerContext contains per-message context data
type HandlerContext struct {
	Bot       imbot.Bot
	BotUUID   string
	ChatID    string
	SenderID  string
	MessageID string
	Platform  imbot.Platform
	IsDirect  bool
	IsGroup   bool
	Text      string
	Media     []imbot.MediaAttachment
	Metadata  map[string]interface{}
}

// NewBotHandler creates a new bot handler with all dependencies
func NewBotHandler(
	ctx context.Context,
	botSetting BotSetting,
	chatStore ChatStoreInterface,
	sessionMgr *session.Manager,
	agentBoot *agentboot.AgentBoot,
	summaryEngine *summarizer.Engine,
	directoryBrowser *DirectoryBrowser,
	manager *imbot.Manager,
	tbClient tbclient.TBClient,
) *BotHandler {
	// Create IM prompter for permission requests
	imPrompter := NewIMPrompter(manager)

	// Create interaction handler for platform-agnostic interactions
	interactionHandler := imbot.NewInteractionHandler(manager)

	// Create file store with proxy support
	fileStore, err := NewFileStoreWithProxy(botSetting.ProxyURL)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create file store with proxy, using default")
		fileStore = NewFileStore()
	}

	// Set telegram token for file URL resolution
	if token, ok := botSetting.Auth["token"]; ok {
		fileStore.SetTelegramToken(token)
	}

	// Initialize handoff manager
	handoffMgr := smart_guide.NewHandoffManager()

	// Initialize SmartGuide rule if configured
	if tbClient != nil && botSetting.SmartGuideProvider != "" && botSetting.SmartGuideModel != "" {
		// Use bot-specific rule creation with bot UUID and name
		if err := tbClient.EnsureSmartGuideRuleForBot(ctx, botSetting.UUID, botSetting.Name, botSetting.SmartGuideProvider, botSetting.SmartGuideModel); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"bot_uuid": botSetting.UUID,
				"bot_name": botSetting.Name,
				"provider": botSetting.SmartGuideProvider,
				"model":    botSetting.SmartGuideModel,
			}).Error("Failed to initialize SmartGuide rule, @tb will be unavailable")
			// Don't block startup, SmartGuide will return errors when used
		} else {
			logrus.WithFields(logrus.Fields{
				"bot_uuid": botSetting.UUID,
				"bot_name": botSetting.Name,
				"provider": botSetting.SmartGuideProvider,
				"model":    botSetting.SmartGuideModel,
			}).Info("SmartGuide rule initialized successfully")
		}
	}

	// Create SmartGuide session store using data directory from tbClient
	var tbSessionStore *smart_guide.SessionStore
	if tbClient != nil {
		dataDir := tbClient.GetDataDir()
		if dataDir != "" {
			sessionsDir := filepath.Join(dataDir, "sessions")
			tbSessionStore, err = smart_guide.NewSessionStore(sessionsDir)
			if err != nil {
				logrus.WithError(err).WithField("sessionsDir", sessionsDir).Warn("Failed to create SmartGuide session store")
			} else {
				logrus.WithField("sessionsDir", sessionsDir).Info("Created SmartGuide session store")
			}
		}
	}

	return &BotHandler{
		ctx:                 ctx,
		botSetting:          botSetting,
		chatStore:           chatStore,
		sessionMgr:          sessionMgr,
		agentBoot:           agentBoot,
		summaryEngine:       summaryEngine,
		directoryBrowser:    directoryBrowser,
		manager:             manager,
		imPrompter:          imPrompter,
		fileStore:           fileStore,
		interaction:         interactionHandler,
		tbClient:            tbClient,
		handoffManager:      handoffMgr,
		tbSessionStore:      tbSessionStore,
		runningCancel:       make(map[string]context.CancelFunc),
		pendingBinds:        make(map[string]*PendingBind),
		actionMenuMessageID: make(map[string]string),
	}
}

// HandleMessage is the main entry point for handling bot messages
func (h *BotHandler) HandleMessage(msg imbot.Message, platform imbot.Platform, botUUID string) {
	bot := h.manager.GetBot(botUUID, platform)
	if bot == nil {
		return
	}

	chatID := getReplyTarget(msg)
	if chatID == "" {
		return
	}

	// NEW: Check if this is an interaction response first
	// This handles both callback queries (interactive mode) and text replies (text mode)
	resp, err := h.interaction.HandleMessage(msg)
	if err == nil && resp != nil {
		// Message was handled as an interaction response
		logrus.WithFields(logrus.Fields{
			"request_id": resp.RequestID,
			"action":     resp.Action.Type,
			"chat_id":    chatID,
		}).Debug("Interaction response handled")
		return
	}

	// OLD: Check if this is a legacy callback query (for backward compatibility)
	if isCallback, _ := msg.Metadata["is_callback"].(bool); isCallback {
		h.handleCallbackQuery(bot, chatID, msg)
		return
	}

	// Create handler context
	mediaAttachments := msg.GetMedia()
	hCtx := HandlerContext{
		Bot:       bot,
		BotUUID:   botUUID,
		ChatID:    chatID,
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Platform:  platform,
		IsDirect:  msg.IsDirectMessage(),
		IsGroup:   msg.IsGroupMessage(),
		Text:      strings.TrimSpace(msg.GetText()),
		Media:     mediaAttachments,
		Metadata:  msg.Metadata,
	}

	// Handle media content (with or without text)
	if msg.IsMediaContent() && len(hCtx.Media) > 0 {
		h.handleMediaMessage(hCtx)
		return
	}

	// Handle text-only messages
	if !msg.IsTextContent() {
		h.SendText(hCtx, "Only text and media messages are supported.")
		return
	}

	if hCtx.Text == "" {
		return
	}

	// Check for stop commands FIRST (highest priority)
	// Supports: /stop, stop, /clear (stop+clear)
	if isStopCommand(hCtx.Text) {
		h.handleStopCommand(hCtx, hCtx.Text == "/clear")
		return
	}

	// Handle direct chat
	if hCtx.IsDirect {
		h.handleDirectMessage(hCtx)
		return
	}

	// Handle group chat
	h.handleGroupMessage(hCtx)
}

// isStopCommand checks if the text is a stop command
// Supports: /stop, stop, /clear
func isStopCommand(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return trimmed == "/stop" || trimmed == "stop" || trimmed == "/clear"
}

// handleStopCommand handles stop commands (/stop, stop, /clear)
func (h *BotHandler) handleStopCommand(hCtx HandlerContext, clearSession bool) {
	h.runningCancelMu.Lock()
	cancel, exists := h.runningCancel[hCtx.ChatID]
	h.runningCancelMu.Unlock()

	if !exists {
		// No running task
		if clearSession {
			// /clear always works, even if nothing running
			h.handleClearCommand(hCtx)
			return
		}
		h.SendText(hCtx, "No running task to stop.")
		return
	}

	// Cancel the execution
	cancel()
	delete(h.runningCancel, hCtx.ChatID)

	if clearSession {
		// /clear also clears the session
		h.handleClearCommand(hCtx)
		return
	}

	h.SendText(hCtx, "🛑 Task stopped.")
}

// ============== Agent Routing Methods ==============

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
	if currentAgent == string(agentClaudeCode) || currentAgent == "claude" {
		return agentClaudeCode, nil
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
	default:
		return fmt.Errorf("unknown agent type: %s", currentAgent)
	}
}

// getProjectPathForChat gets the project path for a chat
func (h *BotHandler) getProjectPathForChat(hCtx HandlerContext) (string, bool, error) {
	// Try direct chat first
	if hCtx.IsDirect {
		projectPath, ok, err := h.chatStore.GetProjectPath(hCtx.ChatID)
		return projectPath, ok, err
	}

	// Try group chat
	projectPath, ok, err := h.chatStore.GetProjectPath(hCtx.ChatID)
	return projectPath, ok, err
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

	// 5. Build meta for response header
	behavior := h.getOutputBehavior()
	meta := ResponseMeta{
		ProjectPath: projectPath,
		ChatID:      hCtx.ChatID,
		UserID:      hCtx.SenderID,
		SessionID:   hCtx.ChatID, // Use chatID as session identifier
		AgentType:   AgentNameTinglyBox,
	}

	// 6. Send processing message (respects verbose mode)
	if behavior.Verbose {
		h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, IconProcess+" "+MsgProcessing, behavior), hCtx.MessageID)
	}

	// 7. Get response from agent
	response, err := agent.ReplyWithContext(h.ctx, text, toolCtx)
	if err != nil {
		logrus.WithError(err).Error("Smart guide agent failed")
		h.SendText(hCtx, fmt.Sprintf("%s Error: %v", IconError, err))
		return nil
	}

	// 8. Get text content from response
	responseText := response.GetTextContent()

	// 9. Save messages to session file
	if h.tbSessionStore != nil {
		// Save user message
		userMsg := message.NewMsg("user", text, types.RoleUser)
		contentPreview := text
		if len(contentPreview) > 50 {
			contentPreview = contentPreview[:50] + "..."
		}
		logrus.WithFields(logrus.Fields{
			"chatID":  hCtx.ChatID,
			"role":    userMsg.Role,
			"content": contentPreview,
		}).Debug("Saving user message to session")

		if err := h.tbSessionStore.AddMessage(hCtx.ChatID, userMsg); err != nil {
			logrus.WithError(err).Warn("Failed to save user message to session")
		}

		// Save assistant response
		assistantMsg := message.NewMsg("assistant", responseText, types.RoleAssistant)
		assistantPreview := responseText
		if len(assistantPreview) > 50 {
			assistantPreview = assistantPreview[:50] + "..."
		}
		logrus.WithFields(logrus.Fields{
			"chatID":  hCtx.ChatID,
			"role":    assistantMsg.Role,
			"content": assistantPreview,
		}).Debug("Saving assistant message to session")

		if err := h.tbSessionStore.AddMessage(hCtx.ChatID, assistantMsg); err != nil {
			logrus.WithError(err).Warn("Failed to save assistant message to session")
		}

		logrus.WithField("chatID", hCtx.ChatID).Debug("Saved messages to session file")
	}

	// 10. Check if working directory was changed by change_workdir tool
	newProjectPath := agent.GetExecutor().GetWorkingDirectory()
	if newProjectPath != projectPath {
		logrus.WithFields(logrus.Fields{
			"chatID":         hCtx.ChatID,
			"oldProjectPath": projectPath,
			"newProjectPath": newProjectPath,
		}).Info("Project path changed, updating chat store")

		// Update chat store with new project path
		chat, err := h.chatStore.GetChat(hCtx.ChatID)
		if err != nil {
			logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to get chat for update")
		}

		if chat == nil {
			now := time.Now().UTC()
			chat = &Chat{
				ChatID:      hCtx.ChatID,
				Platform:    string(hCtx.Platform),
				ProjectPath: newProjectPath,
				BashCwd:     newProjectPath,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := h.chatStore.UpsertChat(chat); err != nil {
				logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to create chat")
			}
		} else {
			if err := h.chatStore.UpdateChat(hCtx.ChatID, func(c *Chat) {
				c.ProjectPath = newProjectPath
				c.BashCwd = newProjectPath
			}); err != nil {
				logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to update chat")
			}
		}
	}

	// 11. Send the response with meta header
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, responseText, behavior), hCtx.MessageID)

	return nil
}

// ============== End Agent Routing Methods ==============

// handleDirectMessage handles messages from direct chat
func (h *BotHandler) handleDirectMessage(hCtx HandlerContext) {
	// Check chat ID lock
	if h.botSetting.ChatIDLock != "" && hCtx.ChatID != h.botSetting.ChatIDLock {
		return
	}

	// Handle commands
	if strings.HasPrefix(hCtx.Text, "/") {
		h.handleSlashCommands(hCtx)
		return
	}

	// Check if waiting for custom path input
	if h.directoryBrowser.IsWaitingInput(hCtx.ChatID) {
		h.handleCustomPathInput(hCtx)
		return
	}

	// Check if there's a pending permission request and user is responding
	if h.handlePermissionTextResponse(hCtx) {
		return
	}

	// NEW: Route all messages through agent router
	// The router now defaults to @tb (Smart Guide) for new users
	// Smart Guide can help with navigation, project setup, and handoff to @cc
	if routeErr := h.routeToAgent(hCtx, hCtx.Text); routeErr != nil {
		logrus.WithError(routeErr).Error("Failed to route to agent")
	}
}

// handleGroupMessage handles messages from group chat
func (h *BotHandler) handleGroupMessage(hCtx HandlerContext) {
	logrus.Infof("Group chat ID: %s", hCtx.ChatID)

	// Check whitelist first
	if !h.chatStore.IsWhitelisted(hCtx.ChatID) {
		logrus.Debugf("Group %s is not whitelisted, ignoring message", hCtx.ChatID)
		h.SendText(hCtx, fmt.Sprintf("This group is not enabled. Please DM the bot with `%s %s` to enable.", cmdJoinPrimary, hCtx.ChatID))
		return
	}

	// Handle commands
	if strings.HasPrefix(hCtx.Text, "/") {
		h.handleSlashCommands(hCtx)
		return
	}

	// Check if waiting for custom path input
	if h.directoryBrowser.IsWaitingInput(hCtx.ChatID) {
		h.handleCustomPathInput(hCtx)
		return
	}

	// Check if there's a pending permission request and user is responding
	if h.handlePermissionTextResponse(hCtx) {
		return
	}

	// NEW: Route all messages through agent router (defaults to @tb)
	// Smart Guide can help groups with navigation, project setup, and handoff to @cc
	if routeErr := h.routeToAgent(hCtx, hCtx.Text); routeErr != nil {
		logrus.WithError(routeErr).Error("Failed to route to agent")
	}
}

// handleMediaMessage handles messages with media attachments
func (h *BotHandler) handleMediaMessage(hCtx HandlerContext) {
	// Get project path for storage, use default if not bound
	projectPath, ok := h.getProjectPath(hCtx)
	if !ok {
		projectPath = h.getDefaultProjectPath()
	}

	// Set platform-specific token on FileStore if needed
	if len(hCtx.Media) > 0 && strings.HasPrefix(hCtx.Media[0].URL, "tgfile://") {
		// Get token from bot settings (check both Auth map and legacy Token field)
		token := h.botSetting.Token
		if token == "" && len(h.botSetting.Auth) > 0 {
			token = h.botSetting.Auth["token"]
		}
		if token != "" {
			h.fileStore.SetTelegramToken(token)
		}
	}

	// 1. Download and store media files
	var fileTags []string
	for _, attachment := range hCtx.Media {
		// Check file type
		if !h.fileStore.IsAllowedType(attachment.MimeType) {
			h.SendText(hCtx, fmt.Sprintf("File type not supported: %s", attachment.MimeType))
			return
		}

		// Check file size
		if attachment.Size > 0 && !h.fileStore.IsAllowedSize(attachment.MimeType, attachment.Size) {
			maxSize := h.fileStore.maxImageSize
			if attachment.Type == "document" {
				maxSize = h.fileStore.maxDocSize
			}
			h.SendText(hCtx, fmt.Sprintf("File too large. Max size: %d MB", maxSize/1024/1024))
			return
		}

		// Download file to project's .download directory
		storedFile, err := h.fileStore.DownloadFile(h.ctx, projectPath, attachment.URL, attachment.MimeType)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Failed to download file: %v", err))
			return

		}

		// Add file tag to message
		fileTags = append(fileTags, fmt.Sprintf("<upload_file>%s</upload_file>", storedFile.RelPath))
	}

	// 2. Construct message with file tags
	message := hCtx.Text
	if len(fileTags) > 0 {
		if message == "" {
			message = strings.Join(fileTags, " ")
		} else {
			message = message + " " + strings.Join(fileTags, " ")
		}
	}

	// 3. Execute with augmented message (using Claude Code)
	h.handleAgentMessage(hCtx, agentClaudeCode, message, projectPath)
}

// getProjectPath returns the project path for the current chat
func (h *BotHandler) getProjectPath(hCtx HandlerContext) (string, bool) {
	if hCtx.IsDirect {
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

// handleSlashCommands handles slash commands
func (h *BotHandler) handleSlashCommands(hCtx HandlerContext) {
	fields := strings.Fields(hCtx.Text)
	if len(fields) == 0 {
		return
	}

	cmd := strings.ToLower(fields[0])

	switch {
	case cmd == "/bot":
		h.handleBotCommand(hCtx, fields)
		return
	case isCommandMatch(cmd, cmdHelpPrimary, cmdHelpAliases):
		h.showBotHelp(hCtx)
		return
	case isCommandMatch(cmd, cmdBindPrimary, cmdBindAliases):
		if len(fields) < 2 {
			h.handleBindInteractive(hCtx)
			return
		}
		h.handleBotBindCommand(hCtx, fields[1:])
	case isCommandMatch(cmd, cmdJoinPrimary, cmdJoinAliases):
		if !hCtx.IsDirect {
			h.SendText(hCtx, cmdJoinPrimary+" can only be used in direct chat.")
			return
		}
		h.handleJoinCommand(hCtx, fields)
		return
	case isCommandMatch(cmd, cmdProjectPrimary, cmdProjectAliases):
		h.handleBotProjectCommand(hCtx)
		return
	case isCommandMatch(cmd, cmdStatusPrimary, cmdStatusAliases):
		h.handleBotStatusCommand(hCtx)
		return
	case isCommandMatch(cmd, cmdClearPrimary, cmdClearAliases):
		h.handleClearCommand(hCtx)
		return
	case isCommandMatch(cmd, cmdBashPrimary, cmdBashAliases):
		h.handleBashCommand(hCtx, fields[1:])
		return
	case cmd == cmdMock:
		// Mock agent command for testing
		mockText := strings.TrimSpace(strings.TrimPrefix(hCtx.Text, cmdMock))
		h.handleAgentMessage(hCtx, agentMock, mockText, "")
		return
	}

	// All other slash commands go to agent router (defaults to @tb)
	// The agent router will handle the command or route to appropriate agent
	if routeErr := h.routeToAgent(hCtx, hCtx.Text); routeErr != nil {
		logrus.WithError(routeErr).Error("Failed to route command to agent")
	}
}

// SendText sends a plain text message
func (h *BotHandler) SendText(hCtx HandlerContext, text string) {
	for _, chunk := range chunkText(text, imbot.DefaultMessageLimit) {
		_, err := hCtx.Bot.SendText(context.Background(), hCtx.ChatID, chunk)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return

		}
	}
}

// sendTextWithReply sends a text message as a reply to another message
func (h *BotHandler) sendTextWithReply(hCtx HandlerContext, text string, replyTo string) {
	for _, chunk := range chunkText(text, imbot.DefaultMessageLimit) {
		_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
			Text:    chunk,
			ReplyTo: replyTo,
		})
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return
		}
	}
}

// sendTextWithActionKeyboard sends a text message with Clear/Bind action buttons
func (h *BotHandler) sendTextWithActionKeyboard(hCtx HandlerContext, text string, replyTo string) {
	kb := BuildActionKeyboard()
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	chunks := chunkText(text, imbot.DefaultMessageLimit)
	for i, chunk := range chunks {
		opts := &imbot.SendMessageOptions{
			Text: chunk,
		}
		if replyTo != "" {
			opts.ReplyTo = replyTo
		}
		if i == len(chunks)-1 {
			opts.Metadata = map[string]interface{}{
				"replyMarkup":        tgKeyboard,
				"_trackActionMenuID": true,
			}
		}

		result, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return
		}

		// Track the action menu message ID for later removal
		if i == len(chunks)-1 && result != nil {
			h.actionMenuMessageIDMu.Lock()
			h.actionMenuMessageID[hCtx.ChatID] = result.MessageID
			h.actionMenuMessageIDMu.Unlock()
		}
	}
}

// formatResponseWithMeta adds project/session/user metadata to the response
// behavior.Verbose controls whether processing messages are sent
func (h *BotHandler) formatResponseWithMeta(meta ResponseMeta, response string, behavior OutputBehavior) string {
	var buf strings.Builder

	// Show agent indicator
	if meta.AgentType != "" {
		buf.WriteString(fmt.Sprintf(FormatAgentLine, GetAgentIcon(meta.AgentType), GetAgentDisplayName(meta.AgentType)))
	}

	// Always show project path (shortened)
	if meta.ProjectPath != "" {
		buf.WriteString(fmt.Sprintf(FormatProjectLine, IconProject, ShortenPath(meta.ProjectPath)))
	}

	// Always show IDs for transparency
	if meta.ChatID != "" {
		buf.WriteString(fmt.Sprintf(FormatDebugLine, IconChat, meta.ChatID))
	}
	if meta.UserID != "" {
		buf.WriteString(fmt.Sprintf(FormatDebugLine, IconUser, meta.UserID))
	}
	if meta.SessionID != "" {
		buf.WriteString(fmt.Sprintf(FormatDebugLine, IconSession, ShortenID(meta.SessionID, 8)))
	}

	buf.WriteString(SeparatorLine + "\n\n")
	return buf.String() + response
}

// getOutputBehavior returns the output behavior for this bot handler
func (h *BotHandler) getOutputBehavior() OutputBehavior {
	return h.botSetting.GetOutputBehavior()
}

// newStreamingMessageHandler creates a new streaming message handler
func (h *BotHandler) newStreamingMessageHandler(hCtx HandlerContext) *streamingMessageHandler {
	return &streamingMessageHandler{
		bot:       hCtx.Bot,
		chatID:    hCtx.ChatID,
		replyTo:   hCtx.MessageID,
		formatter: claude.NewTextFormatter(),
	}
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

	// Auto-create session if none exists or if session is in pending state (stale)
	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed || sess.Status == session.StatusPending {
		sess = h.sessionMgr.CreateWith(hCtx.ChatID, agentType, projectPath)
		// Clear expiration for persistent sessions
		h.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{}        // Zero value means no expiration
			s.Status = session.StatusRunning // Mark as running immediately
		})

		logrus.WithFields(logrus.Fields{
			"chatID":    hCtx.ChatID,
			"sessionID": sess.ID,
			"project":   projectPath,
			"agent":     agentType,
		}).Info("Created new session for Claude Code")
	} else {
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

	// Send status message
	behavior := h.getOutputBehavior()
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, "⏳ Processing...", behavior), hCtx.MessageID)

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

	// Create composite handler that combines streaming + approval + ask handling
	compositeHandler := agentboot.NewCompositeHandler().
		SetStreamer(streamHandler).
		SetApprovalHandler(h.imPrompter).
		SetAskHandler(h.imPrompter).
		SetCompletionCallback(&CompletionCallback{hCtx: hCtx, sessionID: sessionID, sessionMgr: h.sessionMgr})

	result, err := agent.Execute(execCtx, text, agentboot.ExecutionOptions{
		ProjectPath:          projectPath,
		Handler:              compositeHandler,
		SessionID:            sessionID,
		Resume:               shouldResume,
		ChatID:               hCtx.ChatID,
		Platform:             string(hCtx.Platform),
		BotUUID:              hCtx.BotUUID,
		PermissionPromptTool: "stdio",
		PermissionMode:       string(claude.PermissionModeDefault), // Use constant for permission mode
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

	summary := h.summaryEngine.Summarize(response)
	h.sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "assistant",
		Content:   response,
		Summary:   summary,
		Timestamp: time.Now(),
	})

	h.sendTextWithActionKeyboard(hCtx, response, hCtx.MessageID)
}

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
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

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

	// Create new session if needed (including pending state sessions)
	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed || sess.Status == session.StatusPending {
		sess = h.sessionMgr.CreateWith(hCtx.ChatID, agentType, projectPath)
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

	// Send status message
	behavior := h.getOutputBehavior()
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, "🧪 Mock agent processing...", behavior), hCtx.MessageID)

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

// handleBotCommand handles /bot <subcommand> commands
func (h *BotHandler) handleBotCommand(hCtx HandlerContext, fields []string) {
	subcmd := ""
	if len(fields) >= 2 {
		subcmd = strings.ToLower(strings.TrimSpace(fields[1]))
	}

	switch subcmd {
	case "", botCommandHelp:
		h.showBotHelp(hCtx)
	case botCommandBind:
		if len(fields) < 3 {
			h.handleBindInteractive(hCtx)
			return
		}
		h.handleBotBindCommand(hCtx, fields[2:])
	case botCommandJoin:
		if !hCtx.IsDirect {
			h.SendText(hCtx, "/bot join can only be used in direct chat.")
			return
		}
		h.handleJoinCommand(hCtx, fields)
	case botCommandProject:
		h.handleBotProjectCommand(hCtx)
	case botCommandStatus:
		h.handleBotStatusCommand(hCtx)
	case botCommandClear:
		h.handleClearCommand(hCtx)
	case botCommandBash:
		h.handleBashCommand(hCtx, fields[1:])
	default:
		h.SendText(hCtx, fmt.Sprintf("Unknown bot command: %s\nUse /bot help for available commands.", subcmd))
	}
}

// showBotHelp displays the bot help message
func (h *BotHandler) showBotHelp(hCtx HandlerContext) {
	var helpText string
	if hCtx.IsDirect {
		helpText = fmt.Sprintf(directHelpTemplate, hCtx.SenderID)
	} else {
		helpText = fmt.Sprintf(groupHelpTemplate, hCtx.ChatID)
	}
	h.SendText(hCtx, helpText)
}

// handleBotBindCommand handles /bot bind <path>
func (h *BotHandler) handleBotBindCommand(hCtx HandlerContext, fields []string) {
	if len(fields) < 1 {
		h.SendText(hCtx, "Usage: "+cmdBindPrimary+" <project_path>")
		return
	}

	projectPath := strings.TrimSpace(strings.Join(fields, " "))
	if projectPath == "" {
		h.SendText(hCtx, "Usage: "+cmdBindPrimary+" <project_path>")
		return
	}

	// Expand and validate path
	expandedPath, err := ExpandPath(projectPath)
	if err != nil {
		h.SendText(hCtx, fmt.Sprintf("Invalid path: %v", err))
		return

	}

	if err := ValidateProjectPath(expandedPath); err != nil {
		h.SendText(hCtx, fmt.Sprintf("Path validation failed: %v", err))
		return
	}

	h.completeBind(hCtx, expandedPath)
}

// handleBotStatusCommand handles /bot status
func (h *BotHandler) handleBotStatusCommand(hCtx HandlerContext) {
	// Get current agent
	currentAgent, _ := h.getCurrentAgent(hCtx.ChatID)
	agentType := string(currentAgent)

	// Smart Guide is stateless
	if agentType == "tingly-box" {
		// Get project path for status
		projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		if projectPath == "" {
			if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
				projectPath = path
			}
		}

		var statusParts []string
		statusParts = append(statusParts, "Agent: Smart Guide (@tb)")
		statusParts = append(statusParts, "Status: Stateless (no session)")
		if projectPath != "" {
			statusParts = append(statusParts, fmt.Sprintf("Project: %s", projectPath))
		}
		h.SendText(hCtx, strings.Join(statusParts, "\n"))
		return
	}

	// For other agents (claude, mock), find the session
	projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	if projectPath == "" {
		if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
			projectPath = path
		}
	}
	if projectPath == "" {
		h.SendText(hCtx, "No project bound. Use "+cmdBindPrimary+" <project_path> first.")
		return
	}

	sess := h.sessionMgr.FindBy(hCtx.ChatID, agentType, projectPath)
	if sess == nil {
		h.SendText(hCtx, fmt.Sprintf("No session found for agent %s in project %s", agentType, projectPath))
		return
	}

	// Build status message
	var statusParts []string
	statusParts = append(statusParts, fmt.Sprintf("Agent: %s", agentType))
	statusParts = append(statusParts, fmt.Sprintf("Session: %s", sess.ID))
	statusParts = append(statusParts, fmt.Sprintf("Status: %s", sess.Status))

	// Show running duration if running
	if sess.Status == session.StatusRunning {
		runningFor := time.Since(sess.LastActivity).Round(time.Second)
		statusParts = append(statusParts, fmt.Sprintf("Running for: %s", runningFor))
	}

	// Show current request if any
	if sess.Request != "" {
		reqPreview := sess.Request
		if len(reqPreview) > 100 {
			reqPreview = reqPreview[:100] + "..."
		}
		statusParts = append(statusParts, fmt.Sprintf("Current task: %s", reqPreview))
	}

	// Show project path
	if sess.Project != "" {
		statusParts = append(statusParts, fmt.Sprintf("Project: %s", sess.Project))
	}

	// Show error if failed
	if sess.Status == session.StatusFailed && sess.Error != "" {
		errPreview := sess.Error
		if len(errPreview) > 100 {
			errPreview = errPreview[:100] + "..."
		}
		statusParts = append(statusParts, fmt.Sprintf("Error: %s", errPreview))
	}

	h.SendText(hCtx, strings.Join(statusParts, "\n"))
}

// handleClearCommand clears the current session context and creates a new one
func (h *BotHandler) handleClearCommand(hCtx HandlerContext) {
	// Get current project path
	projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	if projectPath == "" {
		// For group chats, also check group binding
		if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
			projectPath = path
		}
	}

	if projectPath == "" {
		h.SendText(hCtx, "No project path found. Use "+cmdBindPrimary+" <project_path> to create a session first.")
		return

	}

	// Get current agent and close the matching session
	currentAgent, _ := h.getCurrentAgent(hCtx.ChatID)
	agentType := string(currentAgent)
	switch currentAgent {
	case agentTinglyBox:
		// Smart Guide uses file-based session store
		if h.tbSessionStore != nil {
			// Clear the SmartGuide session file
			if err := h.tbSessionStore.ClearMessages(hCtx.ChatID); err != nil {
				logrus.WithError(err).Error("Failed to clear SmartGuide session")
				h.SendText(hCtx, "⚠️ Failed to clear SmartGuide session.")
				return
			}
			h.SendText(hCtx, "✅ Smart Guide (@tb) conversation history cleared.")
			logrus.WithField("chatID", hCtx.ChatID).Info("Cleared SmartGuide session")
		} else {
			h.SendText(hCtx, "Smart Guide (@tb) session store is not available.")
		}
		return

	case agentClaudeCode:
		// Claude Code uses Session Manager (existing logic)
		projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		if projectPath == "" {
			if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
				projectPath = path
			}
		}

		if projectPath == "" {
			h.SendText(hCtx, "No project path found. Use "+cmdBindPrimary+" <project_path> to create a session first.")
			return
		}

		oldSess := h.sessionMgr.FindBy(hCtx.ChatID, agentType, projectPath)
		if oldSess != nil {
			h.sessionMgr.Close(oldSess.ID)
		}

		sess := h.sessionMgr.CreateWith(hCtx.ChatID, agentType, projectPath)
		h.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{}
		})

		h.SendText(hCtx, fmt.Sprintf("✅ Claude Code (@cc) session cleared.\n\nNew session: %s\nProject: %s", sess.ID, projectPath))
		return

	default:
		h.SendText(hCtx, "Unknown agent type: "+agentType)
	}
}

// showProjectSelectionOrGuidance shows project selection if user has bound projects, otherwise shows bind confirmation
func (h *BotHandler) showProjectSelectionOrGuidance(hCtx HandlerContext) {
	if h.chatStore == nil {
		h.showBindConfirmationPrompt(hCtx, "")
		return
	}

	// For group chats, show bind confirmation
	if !hCtx.IsDirect {
		h.showBindConfirmationPrompt(hCtx, "")
		return
	}

	// For direct chats, check if user has any bound projects
	platform := string(hCtx.Platform)

	chats, err := h.chatStore.ListChatsByOwner(hCtx.SenderID, platform)
	if err == nil && len(chats) > 0 {
		// User has projects, show project selection
		h.handleBotProjectCommand(hCtx)
		return
	}

	// No projects, show bind confirmation
	h.showBindConfirmationPrompt(hCtx, "")
}

// showBindConfirmationPrompt shows a confirmation prompt for binding to current directory
func (h *BotHandler) showBindConfirmationPrompt(hCtx HandlerContext, originalMessage string) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "~" // fallback
	}
	absPath, err := filepath.Abs(cwd)
	if err == nil {
		cwd = absPath
	}

	// Store pending bind request
	h.pendingBindsMu.Lock()
	h.pendingBinds[hCtx.ChatID] = &PendingBind{
		OriginalMessage: originalMessage,
		ProposedPath:    cwd,
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}
	h.pendingBindsMu.Unlock()

	// Send confirmation with inline keyboard
	kb := BuildBindConfirmKeyboard()
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	_, err = hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text: BuildBindConfirmPrompt(cwd),
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to send bind confirmation")
	}
}

// handleBindConfirm handles the bind confirmation callback
func (h *BotHandler) handleBindConfirm(hCtx HandlerContext) {
	h.pendingBindsMu.RLock()
	pending, exists := h.pendingBinds[hCtx.ChatID]
	h.pendingBindsMu.RUnlock()

	if !exists || time.Now().After(pending.ExpiresAt) {
		h.SendText(hCtx, "Bind request expired. Please try again.")
		delete(h.pendingBinds, hCtx.ChatID)
		return

	}

	// Bind the project
	err := h.chatStore.BindProject(hCtx.ChatID, string(hCtx.Platform), hCtx.BotUUID, pending.ProposedPath)
	if err != nil {
		h.SendText(hCtx, fmt.Sprintf("Failed to bind project: %v", err))
		delete(h.pendingBinds, hCtx.ChatID)
		return

	}

	// Close the old session for this (chat, agent) combination if exists
	agentType := "claude"
	oldSess := h.sessionMgr.FindBy(hCtx.ChatID, agentType, "")
	if oldSess != nil {
		h.sessionMgr.Close(oldSess.ID)
		logrus.WithFields(logrus.Fields{
			"chatID":    hCtx.ChatID,
			"sessionID": oldSess.ID,
		}).Info("Closed old session after project change")
	}

	// Create a new session with the new project binding
	sess := h.sessionMgr.CreateWith(hCtx.ChatID, agentType, pending.ProposedPath)
	// Clear expiration for direct chat sessions
	h.sessionMgr.Update(sess.ID, func(s *session.Session) {
		s.ExpiresAt = time.Time{} // Zero value means no expiration
	})

	delete(h.pendingBinds, hCtx.ChatID)

	h.SendText(hCtx, fmt.Sprintf("✅ Bound to: `%s`", pending.ProposedPath))

	// If there was an original message, process it now
	if pending.OriginalMessage != "" {
		h.handleAgentMessage(hCtx, agentClaudeCode, pending.OriginalMessage, pending.ProposedPath)
	}
}

// handleBotProjectCommand handles /bot project - shows current project and list with keyboard
func (h *BotHandler) handleBotProjectCommand(hCtx HandlerContext) {
	if h.chatStore == nil {
		h.SendText(hCtx, "Store not available")
		return
	}

	platform := string(hCtx.Platform)

	// Get current project path for this chat
	currentPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)

	// Build message text
	var buf strings.Builder
	if currentPath != "" {
		buf.WriteString(fmt.Sprintf("Current Project:\n📁 %s\n\n", currentPath))
	} else {
		buf.WriteString("No project bound to this chat.\n\n")
	}

	// Get all projects for user
	var projectPaths []string
	if hCtx.IsDirect {
		chats, err := h.chatStore.ListChatsByOwner(hCtx.SenderID, platform)
		if err == nil {
			seen := make(map[string]bool)
			for _, chat := range chats {
				if chat.ProjectPath != "" && !seen[chat.ProjectPath] {
					projectPaths = append(projectPaths, chat.ProjectPath)
					seen[chat.ProjectPath] = true
				}
			}
		}
	}

	if len(projectPaths) > 0 {
		buf.WriteString("Your Projects:\n")
	} else {
		buf.WriteString("No projects found.")
	}

	// Build keyboard with projects
	var rows [][]imbot.InlineKeyboardButton
	for _, path := range projectPaths {
		marker := ""
		if path == currentPath {
			marker = " ✓"
		}
		btn := imbot.InlineKeyboardButton{
			Text:         fmt.Sprintf("📁 %s%s", filepath.Base(path), marker),
			CallbackData: imbot.FormatCallbackData("project", "switch", path),
		}
		rows = append(rows, []imbot.InlineKeyboardButton{btn})
	}

	// Add "Bind New" button
	rows = append(rows, []imbot.InlineKeyboardButton{{
		Text:         "📁 Bind New Project",
		CallbackData: imbot.FormatCallbackData("action", "bind"),
	}})

	keyboard := imbot.InlineKeyboardMarkup{InlineKeyboard: rows}
	tgKeyboard := convertActionKeyboardToTelegram(keyboard)

	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      buf.String(),
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send project list")
	}
}

// handleProjectSwitch handles switching to a different project
func (h *BotHandler) handleProjectSwitch(hCtx HandlerContext, projectPath string) {
	if h.chatStore == nil {
		h.SendText(hCtx, "Store not available")
		return

	}

	// Bind the project to this chat
	if err := h.chatStore.BindProject(hCtx.ChatID, string(hCtx.Platform), projectPath, hCtx.SenderID); err != nil {
		h.SendText(hCtx, "Failed to switch project")
		return
	}

	// Get current agent and close old session
	currentAgent, _ := h.getCurrentAgent(hCtx.ChatID)
	agentType := string(currentAgent)

	// Close old session for this (chat, agent) with different project
	// Find any session for this (chat, agent) and close it
	// Note: We need to close all sessions for this (chat, agent) since project changed
	if agentType != "tingly-box" {
		sessions := h.sessionMgr.ListByChat(hCtx.ChatID)
		for _, sess := range sessions {
			if sess.Agent == agentType && sess.Project != projectPath {
				h.sessionMgr.Close(sess.ID)
			}
		}
	}

	logrus.Infof("Project switched: chat=%s path=%s agent=%s", hCtx.ChatID, projectPath, agentType)
	h.SendText(hCtx, fmt.Sprintf("✅ Switched to: %s", projectPath))
}

// handleBindInteractive starts an interactive directory browser for binding
func (h *BotHandler) handleBindInteractive(hCtx HandlerContext) {
	// Start from home directory
	_, err := h.directoryBrowser.Start(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Error("Failed to start directory browser")
		h.SendText(hCtx, fmt.Sprintf("Failed to start directory browser: %v", err))
		return

	}

	logrus.Infof("Bind flow started for chat %s", hCtx.ChatID)

	// Send directory browser
	_, err = SendDirectoryBrowser(h.ctx, hCtx.Bot, h.directoryBrowser, hCtx.ChatID, "")
	if err != nil {
		logrus.WithError(err).Error("Failed to send directory browser")
		h.SendText(hCtx, fmt.Sprintf("Failed to send directory browser: %v", err))
		return

	}
}

// completeBind completes the project binding process
func (h *BotHandler) completeBind(hCtx HandlerContext, projectPath string) {
	// Expand path (handles ~, etc.)
	expandedPath, err := ExpandPath(projectPath)
	if err != nil {
		h.SendText(hCtx, fmt.Sprintf("Invalid path: %v", err))
		return

	}

	// Only validate if the path should already exist
	if _, err := os.Stat(expandedPath); err == nil {
		if err := ValidateProjectPath(expandedPath); err != nil {
			h.SendText(hCtx, fmt.Sprintf("Path validation failed: %v", err))
			return
		}
	}

	platform := string(hCtx.Platform)

	// Bind project to chat using ChatStore
	if err := h.chatStore.BindProject(hCtx.ChatID, platform, expandedPath, hCtx.SenderID); err != nil {
		h.SendText(hCtx, fmt.Sprintf("Failed to bind project: %v", err))
		return
	}

	// With new design, sessions are created on-demand when agent processes a message
	// No need to create session here

	logrus.Infof("Project bound: chat=%s path=%s", hCtx.ChatID, expandedPath)

	if hCtx.IsDirect {
		h.SendText(hCtx, fmt.Sprintf("✅ Project bound: %s\n\nYou can now send messages directly.", expandedPath))
	} else {
		h.SendText(hCtx, fmt.Sprintf("✅ Group bound to project: %s", expandedPath))
	}
}

// handleJoinCommand handles the /join command to add a group to whitelist
func (h *BotHandler) handleJoinCommand(hCtx HandlerContext, fields []string) {
	if len(fields) < 2 {
		h.SendText(hCtx, "Usage: /join <group_id|@username|invite_link>")
		return
	}

	input := strings.TrimSpace(strings.Join(fields[1:], " "))
	if input == "" {
		h.SendText(hCtx, "Usage: /join <group_id|@username|invite_link>")
		return
	}

	// Try to cast bot to TelegramBot interface
	tgBot, ok := imbot.AsTelegramBot(hCtx.Bot)
	if !ok {
		h.SendText(hCtx, "Join command is only supported for Telegram bot.")
		return
	}

	// Resolve the chat ID
	groupID, err := tgBot.ResolveChatID(input)
	if err != nil {
		logrus.WithError(err).Error("Failed to resolve chat ID")
		h.SendText(hCtx, fmt.Sprintf("Failed to resolve chat ID: %v\n\nNote: Bot must already be a member of the group to add it to whitelist.", err))
		return

	}

	// Check if already whitelisted
	if h.chatStore.IsWhitelisted(groupID) {
		h.SendText(hCtx, fmt.Sprintf("Group %s is already in whitelist.", groupID))
		return
	}

	// Add group to whitelist
	platform := string(hCtx.Platform)
	if err := h.chatStore.AddToWhitelist(groupID, platform, hCtx.SenderID); err != nil {
		logrus.WithError(err).Error("Failed to add group to whitelist")
		h.SendText(hCtx, fmt.Sprintf("Failed to add group to whitelist: %v", err))
		return
	}

	h.SendText(hCtx, fmt.Sprintf("Successfully added group to whitelist.\nGroup ID: %s", groupID))
	logrus.Infof("Group %s added to whitelist by %s", groupID, hCtx.SenderID)
}

// handleBashCommand handles /bot bash <cmd>
func (h *BotHandler) handleBashCommand(hCtx HandlerContext, fields []string) {
	if len(fields) < 2 {
		h.SendText(hCtx, "Usage: /bash <command>")
		return
	}
	allowlist := normalizeAllowlistToMap(h.botSetting.BashAllowlist)
	if len(allowlist) == 0 {
		allowlist = defaultBashAllowlist
	}
	subcommand := strings.ToLower(strings.TrimSpace(fields[1]))
	if _, ok := allowlist[subcommand]; !ok {
		h.SendText(hCtx, "Command not allowed.")
		return
	}

	// Get project path from Chat instead of session
	projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	bashCwd, _, err := h.chatStore.GetBashCwd(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load bash cwd")
	}
	baseDir := bashCwd
	if baseDir == "" {
		baseDir = projectPath
	}

	switch subcommand {
	case "pwd":
		if baseDir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				h.SendText(hCtx, "Unable to resolve working directory.")
				return

			}
			h.SendText(hCtx, cwd)
			return
		}
		h.SendText(hCtx, baseDir)
	case "cd":
		if len(fields) < 3 {
			h.SendText(hCtx, "Usage: /bash cd <path>")
			return
		}
		nextPath := strings.TrimSpace(strings.Join(fields[2:], " "))
		if nextPath == "" {
			h.SendText(hCtx, "Usage: /bash cd <path>")
			return
		}
		cdBase := baseDir
		if cdBase == "" {
			cwd, err := os.Getwd()
			if err != nil {
				h.SendText(hCtx, "Unable to resolve working directory.")
				return

			}
			cdBase = cwd
		}
		if !filepath.IsAbs(nextPath) {
			nextPath = filepath.Join(cdBase, nextPath)
		}
		if stat, err := os.Stat(nextPath); err != nil || !stat.IsDir() {
			h.SendText(hCtx, "Directory not found.")
			return
		}
		absPath, err := filepath.Abs(nextPath)
		if err == nil {
			nextPath = absPath
		}
		if err := h.chatStore.SetBashCwd(hCtx.ChatID, nextPath); err != nil {
			logrus.WithError(err).Warn("Failed to update bash cwd")
		}
		h.SendText(hCtx, fmt.Sprintf("Bash working directory set to %s", nextPath))
	case "ls":
		if baseDir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				h.SendText(hCtx, "Unable to resolve working directory.")
				return

			}
			baseDir = cwd
		}
		var args []string
		if len(fields) > 2 {
			args = append(args, fields[2:]...)
		}
		execCtx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(execCtx, "ls", args...)
		cmd.Dir = baseDir
		output, err := cmd.CombinedOutput()
		if err != nil && len(output) == 0 {
			h.SendText(hCtx, fmt.Sprintf("Command failed: %v", err))
			return

		}
		h.SendText(hCtx, strings.TrimSpace(string(output)))
	default:
		h.SendText(hCtx, "Command not allowed.")
	}
}

// handleCallbackQuery handles callback queries from inline keyboards
func (h *BotHandler) handleCallbackQuery(bot imbot.Bot, chatID string, msg imbot.Message) {
	callbackData, _ := msg.Metadata["callback_data"].(string)
	if callbackData == "" {
		return
	}

	parts := imbot.ParseCallbackData(callbackData)
	if len(parts) == 0 {
		return
	}

	// Create a minimal handler context for callbacks
	hCtx := HandlerContext{
		Bot:       bot,
		ChatID:    chatID,
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Platform:  msg.Platform,
		Metadata:  msg.Metadata,
	}

	action := parts[0]

	switch action {
	case "perm":
		// Handle permission request response
		h.handlePermissionCallback(hCtx, parts)

	case "action":
		if len(parts) < 2 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "clear":
			// Remove the action keyboard before handling
			h.removeActionKeyboard(bot, chatID)
			h.handleClearCommand(hCtx)
		case "bind":
			// Remove the action keyboard before handling
			h.removeActionKeyboard(bot, chatID)
			// Start interactive bind
			// Start interactive bind
			h.handleBindInteractive(hCtx)
		case "project":
			// Remove the action keyboard before handling
			h.removeActionKeyboard(bot, chatID)
			// Start interactive bind
			// Start interactive bind
			h.handleBotProjectCommand(hCtx)
		}

	case "project":
		// Remove the action keyboard before handling
		h.removeActionKeyboard(bot, chatID)
		// Start interactive bind
		if len(parts) < 3 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "switch":
			projectID := parts[2]
			h.handleProjectSwitch(hCtx, projectID)
		}

	case "bind":
		// Remove the action keyboard before handling
		h.removeActionKeyboard(bot, chatID)
		// Start interactive bind
		if len(parts) < 2 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "confirm":
			// Confirm bind to current directory
			h.handleBindConfirm(hCtx)

		case "dir":
			// Navigate to directory by index
			if len(parts) < 3 {
				return
			}
			indexStr := parts[2]
			var index int
			if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
				logrus.WithError(err).Warn("Failed to parse directory index")
				return
			}
			if err := h.directoryBrowser.NavigateByIndex(chatID, index); err != nil {
				logrus.WithError(err).Warn("Failed to navigate directory")
				return
			}
			// Get message ID from metadata for editing
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "up":
			// Navigate to parent directory
			if err := h.directoryBrowser.NavigateUp(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to navigate up")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "prev":
			if err := h.directoryBrowser.PrevPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to previous page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "next":
			if err := h.directoryBrowser.NextPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to next page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "select":
			// Select current directory (path is in state)
			currentPath := h.directoryBrowser.GetCurrentPath(chatID)
			if currentPath == "" {
				logrus.Warn("No current path in bind flow")
				return
			}
			// Get message ID from state before clearing
			msgID := ""
			if state := h.directoryBrowser.GetState(chatID); state != nil {
				msgID = state.MessageID
			}
			h.completeBind(hCtx, currentPath)
			h.directoryBrowser.Clear(chatID)
			// Edit message to show success and remove keyboard
			if msgID != "" {
				editDirectoryBrowserMessage(h.ctx, bot, chatID, msgID, fmt.Sprintf("✅ Bound to: `%s`", currentPath))
			}

		case "custom":
			// Start custom path input mode
			h.handleCustomPathPrompt(hCtx)

		case "create":
			// Create directory and bind (path from custom input, encoded)
			if len(parts) < 3 {
				return
			}
			encodedPath := parts[2]
			path := imbot.ParseDirPath(encodedPath)
			// Create the directory
			if err := os.MkdirAll(path, 0755); err != nil {
				logrus.WithError(err).Error("Failed to create directory")
				h.SendText(hCtx, fmt.Sprintf("Failed to create directory: %v", err))
				return
			}

		case "cancel":
			h.directoryBrowser.Clear(chatID)
			// Get message ID from metadata for editing
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			// Edit message to show cancel and remove keyboard
			editDirectoryBrowserMessage(h.ctx, bot, chatID, msgID, "❌ Bind cancelled.")
			h.SendText(hCtx, "Bind cancelled.")
		}
	}
}

// handleCustomPathPrompt sends the custom path input prompt
func (h *BotHandler) handleCustomPathPrompt(hCtx HandlerContext) {
	// Ensure state exists
	state := h.directoryBrowser.GetState(hCtx.ChatID)
	if state == nil {
		// Start a new bind flow if none exists
		var err error
		state, err = h.directoryBrowser.Start(hCtx.ChatID)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Failed to start bind flow: %v", err))
			return

		}
	}

	// Set waiting for input state
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, true, "")

	// Send prompt with cancel keyboard
	kb := BuildCancelKeyboard()
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	result, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      BuildCustomPathPrompt(),
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send custom path prompt")
		return

	}

	// Store prompt message ID
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, true, result.MessageID)
}

// handleCustomPathInput handles the user's custom path input
func (h *BotHandler) handleCustomPathInput(hCtx HandlerContext) {
	// Get current path from browser state
	state := h.directoryBrowser.GetState(hCtx.ChatID)
	currentPath := ""
	if state != nil {
		currentPath = state.CurrentPath
	}

	// Expand path relative to current directory
	var expandedPath string
	if filepath.IsAbs(hCtx.Text) || strings.HasPrefix(hCtx.Text, "~") {
		// Absolute path or home-relative path
		var err error
		expandedPath, err = ExpandPath(hCtx.Text)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Invalid path: %v", err))
			return

		}
	} else if currentPath != "" {
		// Relative path - expand relative to current directory
		expandedPath = filepath.Join(currentPath, hCtx.Text)
	} else {
		// No current path, use ExpandPath
		var err error
		expandedPath, err = ExpandPath(hCtx.Text)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Invalid path: %v", err))
			return

		}
	}

	// Clean the path
	expandedPath = filepath.Clean(expandedPath)

	// Check if path exists
	info, err := os.Stat(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist, ask for confirmation to create
			h.handleCreateConfirm(hCtx, expandedPath)
			return

		}
		h.SendText(hCtx, fmt.Sprintf("Cannot access path: %v", err))
		return
	}

	if !info.IsDir() {
		h.SendText(hCtx, "The path is not a directory. Please provide a directory path.")
		return
	}

	// Path exists and is a directory, complete the bind
	h.completeBind(hCtx, expandedPath)
	h.directoryBrowser.Clear(hCtx.ChatID)
}

// handlePermissionCallback handles permission request callback responses
func (h *BotHandler) handlePermissionCallback(hCtx HandlerContext, parts []string) {
	if len(parts) < 3 {
		logrus.WithField("parts", parts).Warn("Invalid permission callback data")
		return
	}

	subAction := parts[1]
	requestID := parts[2]

	// Check if the request exists
	pendingReq, exists := h.imPrompter.GetPendingRequest(requestID)
	if !exists {
		logrus.WithField("request_id", requestID).Warn("Permission request not found or expired")
		h.SendText(hCtx, "⚠️ This permission request has expired or already been answered.")
		return
	}

	var resultText string

	switch subAction {
	case "allow":
		if err := h.imPrompter.SubmitDecision(requestID, true, false, ""); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit permission decision")
			h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
			return
		}
		resultText = "✅ Permission granted"
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"tool_name":  pendingReq.ToolName,
			"user_id":    hCtx.SenderID,
		}).Info("User approved tool permission")

	case "deny":
		if err := h.imPrompter.SubmitDecision(requestID, false, false, ""); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit permission decision")
			h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
			return
		}
		resultText = "❌ Permission denied"
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"tool_name":  pendingReq.ToolName,
			"user_id":    hCtx.SenderID,
		}).Info("User denied tool permission")

	case "always":
		if err := h.imPrompter.SubmitDecision(requestID, true, true, ""); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit permission decision")
			h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
			return
		}
		resultText = "🔄 Always allowed"
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"tool_name":  pendingReq.ToolName,
			"user_id":    hCtx.SenderID,
		}).Info("User approved tool permission (always)")

	case "option":
		// Handle multi-option selection (e.g., AskUserQuestion)
		if len(parts) < 4 {
			logrus.WithField("parts", parts).Warn("Invalid option callback data")
			return
		}
		optionIndex := parts[3]

		// Convert index to label from the pending request
		optionLabel := optionIndex
		if questions, ok := pendingReq.Input["questions"].([]interface{}); ok && len(questions) > 0 {
			if question, ok := questions[0].(map[string]interface{}); ok {
				if options, ok := question["options"].([]interface{}); ok {
					// Parse index
					var idx int
					if _, err := fmt.Sscanf(optionIndex, "%d", &idx); err == nil && idx >= 0 && idx < len(options) {
						if option, ok := options[idx].(map[string]interface{}); ok {
							if label, ok := option["label"].(string); ok {
								optionLabel = label
							}
						}
					}
				}
			}
		}

		// Submit as a structured response with the label
		if err := h.imPrompter.SubmitUserResponse(requestID, ask.Response{
			Type: "selection",
			Data: optionLabel,
		}); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit option selection")
			h.SendText(hCtx, fmt.Sprintf("Failed to process option selection: %v", err))
			return
		}
		resultText = fmt.Sprintf("✅ Selected: %s", optionLabel)
		logrus.WithFields(logrus.Fields{
			"request_id":   requestID,
			"tool_name":    pendingReq.ToolName,
			"option_index": optionIndex,
			"option_label": optionLabel,
			"user_id":      hCtx.SenderID,
		}).Info("User selected option")

	default:
		logrus.WithField("action", subAction).Warn("Unknown permission action")
		return
	}

	// Send feedback to user
	h.SendText(hCtx, fmt.Sprintf("%s for tool: `%s`", resultText, pendingReq.ToolName))
}

// handlePermissionTextResponse handles text-based permission responses
// Returns true if the message was a valid permission response, false otherwise
func (h *BotHandler) handlePermissionTextResponse(hCtx HandlerContext) bool {
	// Check if there are pending permission requests for this chat
	pendingReqs := h.imPrompter.GetPendingRequestsForChat(hCtx.ChatID)
	if len(pendingReqs) == 0 {
		return false
	}

	// Get the most recent pending request for this chat
	// (usually there's only one at a time)
	latestReq := pendingReqs[0]

	// For AskUserQuestion, try to parse as option selection first
	if latestReq.ToolName == "AskUserQuestion" {
		// Try to submit as a text selection
		if err := h.imPrompter.SubmitUserResponse(latestReq.ID, ask.Response{
			Type: "text",
			Data: hCtx.Text,
		}); err == nil {
			h.SendText(hCtx, fmt.Sprintf("✅ Selected: %s", hCtx.Text))
			logrus.WithFields(logrus.Fields{
				"request_id": latestReq.ID,
				"tool_name":  latestReq.ToolName,
				"user_id":    hCtx.SenderID,
				"selection":  hCtx.Text,
			}).Info("User selected option via text")
			return true
		}
	}

	// Try to parse the text as a standard permission response
	approved, remember, isValid := ask.ParseTextResponse(hCtx.Text)
	if !isValid {
		// Not a valid permission response, let other handlers process it
		return false
	}

	// Submit the decision
	if err := h.imPrompter.SubmitDecision(latestReq.ID, approved, remember, ""); err != nil {
		logrus.WithError(err).WithField("request_id", latestReq.ID).Error("Failed to submit permission decision")
		h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
		return true
	}

	// Send feedback to user
	var resultText string
	if remember {
		resultText = "🔄 Always allowed"
	} else if approved {
		resultText = "✅ Permission granted"
	} else {
		resultText = "❌ Permission denied"
	}

	h.SendText(hCtx, fmt.Sprintf("%s for tool: `%s`", resultText, latestReq.ToolName))

	logrus.WithFields(logrus.Fields{
		"request_id": latestReq.ID,
		"tool_name":  latestReq.ToolName,
		"user_id":    hCtx.SenderID,
		"approved":   approved,
		"remember":   remember,
	}).Info("User responded to permission request via text")

	return true
}

// handleCreateConfirm sends a confirmation prompt for creating a directory
func (h *BotHandler) handleCreateConfirm(hCtx HandlerContext, path string) {
	// Reset waiting input state (no longer waiting for text input)
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, false, "")

	kb, text := BuildCreateConfirmKeyboard(path)
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send create confirmation")
	}
}

// RequestInteraction sends an interaction request using the new interaction system
// This is a convenience method for BotHandler to request platform-agnostic interactions
func (h *BotHandler) RequestInteraction(ctx context.Context, hCtx HandlerContext, req imbot.InteractionRequest) (*imbot.InteractionResponse, error) {
	// Set the bot and platform info from the handler context
	req.BotUUID = hCtx.BotUUID
	req.Platform = hCtx.Platform
	req.ChatID = hCtx.ChatID

	// Set default timeout if not specified
	if req.Timeout == 0 {
		req.Timeout = 5 * time.Minute
	}

	return h.interaction.RequestInteraction(ctx, req)
}

// RequestConfirmation requests a yes/no confirmation from the user
// Uses the new interaction system with platform-agnostic UI
func (h *BotHandler) RequestConfirmation(ctx context.Context, hCtx HandlerContext, message, requestID string) (bool, error) {
	builder := imbot.NewInteractionBuilder()
	builder.AddConfirm(requestID)

	req := imbot.InteractionRequest{
		ID:           requestID,
		Message:      message,
		ParseMode:    imbot.ParseModeMarkdown,
		Mode:         imbot.ModeAuto,
		Interactions: builder.Build(),
		Timeout:      5 * time.Minute,
	}

	resp, err := h.RequestInteraction(ctx, hCtx, req)
	if err != nil {
		return false, err
	}

	return resp.IsConfirm(), nil
}

// RequestOptionSelection requests the user to select from a list of options
// Uses the new interaction system with platform-agnostic UI
func (h *BotHandler) RequestOptionSelection(ctx context.Context, hCtx HandlerContext, message, requestID string, options []imbot.Option) (int, *imbot.Interaction, error) {
	builder := imbot.NewInteractionBuilder()
	builder.AddOptions(requestID, options)

	req := imbot.InteractionRequest{
		ID:           requestID,
		Message:      message,
		ParseMode:    imbot.ParseModeMarkdown,
		Mode:         imbot.ModeAuto,
		Interactions: builder.Build(),
		Timeout:      5 * time.Minute,
	}

	resp, err := h.RequestInteraction(ctx, hCtx, req)
	if err != nil {
		return -1, nil, err
	}

	// Find the selected index
	for i, opt := range options {
		if opt.Value == resp.Action.Value {
			return i, &resp.Action, nil
		}
	}

	return -1, &resp.Action, nil
}

// removeActionKeyboard removes the action keyboard menu from the chat
func (h *BotHandler) removeActionKeyboard(bot imbot.Bot, chatID string) {
	h.actionMenuMessageIDMu.RLock()
	msgID, ok := h.actionMenuMessageID[chatID]
	h.actionMenuMessageIDMu.RUnlock()

	if !ok || msgID == "" {
		return
	}

	// Try to cast to TelegramBot and remove the keyboard
	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		if err := tgBot.RemoveMessageKeyboard(context.Background(), chatID, msgID); err != nil {
			logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to remove action keyboard")
		} else {
			// Successfully removed, clear the tracking
			h.actionMenuMessageIDMu.Lock()
			delete(h.actionMenuMessageID, chatID)
			h.actionMenuMessageIDMu.Unlock()
		}
	}
}

// editDirectoryBrowserMessage edits the directory browser message to show status and remove keyboard

// editDirectoryBrowserMessage edits the directory browser message to show status and remove keyboard
func editDirectoryBrowserMessage(ctx context.Context, bot imbot.Bot, chatID string, msgID string, text string) {
	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		// Remove the keyboard first
		if err := tgBot.RemoveMessageKeyboard(ctx, chatID, msgID); err != nil {
			logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to remove directory browser keyboard")
		} else {
			// Successfully removed keyboard, now edit the text
			if err := tgBot.EditMessageWithKeyboard(ctx, chatID, msgID, text, nil); err != nil {
				logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to edit directory browser text")
			}
		}
	}
}
