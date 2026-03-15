package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	mock "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// BotHandler encapsulates all bot message handling logic and dependencies
type BotHandler struct {
	ctx              context.Context
	botSetting       BotSetting
	chatStore        ChatStoreInterface // Use interface for flexibility
	sessionMgr       *session.Manager
	agentBoot        *agentboot.AgentBoot
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

	// verbose controls whether to show intermediate messages (onMessage details)
	// true = show all messages (default), false = show only final results
	verbose   bool
	verboseMu sync.RWMutex
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
		verbose:             true, // Default to verbose mode
	}
}

// GetVerbose returns the current verbose mode setting for a chat
// Checks chat store first, then bot setting default
func (h *BotHandler) GetVerbose(chatID string) bool {
	// Try to get verbose from chat store
	if h.chatStore != nil {
		chat, err := h.chatStore.GetChat(chatID)
		if err == nil && chat != nil && chat.Verbose != nil {
			return *chat.Verbose
		}
	}

	// Fallback to bot setting default
	botSetting := h.botSetting.GetOutputBehavior()
	return botSetting.Verbose
}

// SetVerbose sets the verbose mode for a chat
func (h *BotHandler) SetVerbose(chatID string, verbose bool) {
	// Update in chat store
	if h.chatStore != nil {
		err := h.chatStore.UpdateChat(chatID, func(c *Chat) {
			c.Verbose = &verbose
		})
		if err != nil {
			logrus.WithError(err).WithField("chatID", chatID).Warn("Failed to update verbose in chat store")
		}
	}

	// Also update in-memory default (fallback)
	h.verboseMu.Lock()
	h.verboseMu.Unlock()
	h.verbose = verbose
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
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

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
	return newStreamingMessageHandler(hCtx.Bot, hCtx.ChatID, hCtx.MessageID, h.GetVerbose(hCtx.ChatID))
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
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

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
