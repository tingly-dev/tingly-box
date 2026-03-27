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
	"github.com/tingly-dev/tingly-box/agentboot/ask"
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

	// commandRegistry holds the strongly-typed command registry
	commandRegistry *imbot.CommandRegistry

	// commandAdapter bridges BotHandler to the command system
	commandAdapter BotHandlerAdapter
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
	Message   imbot.Message
}

func (c *HandlerContext) IsDirect() bool {
	return c.Message.IsDirectMessage()
}

func (c *HandlerContext) IsGroup() bool {
	return c.Message.IsGroupMessage()
}

func (c *HandlerContext) Text() string {
	return strings.TrimSpace(c.Message.GetText())
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
// Returns false for platforms that don't support verbose mode (e.g., Weixin)
func (h *BotHandler) GetVerbose(chatID string) bool {
	// Check if platform supports verbose mode
	//if !SupportsVerboseMode(h.botSetting.Platform) {
	//	return false
	//}

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

	chatID := msg.GetReplyTarget()
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
	hCtx := HandlerContext{
		Bot:       bot,
		BotUUID:   botUUID,
		ChatID:    chatID,
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Platform:  platform,
		Message:   msg,
	}

	switch {
	case msg.IsDirectMessage():
		logrus.Infof("Chat ID: %s", chatID)
		// Check chat ID lock
		if h.botSetting.ChatIDLock != "" && chatID != h.botSetting.ChatIDLock {
			return
		}
	case msg.IsGroupMessage():
		logrus.Infof("Group chat ID: %s", chatID)
		// Check whitelist first
		if !h.chatStore.IsWhitelisted(chatID) {
			logrus.Debugf("Group %s is not whitelisted, ignoring message", chatID)
			h.SendText(hCtx, fmt.Sprintf("This group is not enabled. Please DM the bot with `%s %s` to enable.", cmdJoinPrimary, chatID))
			return
		}
	default:
		logrus.Errorf("Unsupported message from upstream: %v", msg)
		h.SendText(hCtx, fmt.Sprintf("Unsupported message from upstream %s %s.", msg.ChatType, chatID))
		return
	}

	// Handle media content (with or without text)
	if msg.IsMediaContent() {
		media := msg.GetMedia()
		if len(media) > 0 {
			h.handleMediaMessage(hCtx, media)
		} else {
			h.SendText(hCtx, fmt.Sprintf("Empty media from %s %s.", msg.ChatType, chatID))
		}
		return
	}

	// HandleMediaContent checks if message is media content
	// Handle text-only messages
	logrus.Debugf("Message content check: IsMediaContent=%v, IsTextContent=%v",
		msg.IsMediaContent(), msg.IsTextContent())
	if !msg.IsTextContent() {
		h.SendText(hCtx, "Only text and media messages are supported.")
		return
	}

	text := hCtx.Text()
	logrus.Debugf("Text content: text_len=%d, text=%q", len(text), text)
	if hCtx.Text() == "" {
		logrus.Warn("Text content is empty, returning")
		return
	}

	// Check for stop commands FIRST (highest priority)
	// Supports: /stop, stop, /clear (stop+clear)
	if isStopCommand(hCtx.Text()) {
		h.handleStopCommand(hCtx, hCtx.Text() == "/clear")
		return
	}

	// Handle commands
	if strings.HasPrefix(hCtx.Text(), "/") {
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
	if routeErr := h.routeToAgent(hCtx, hCtx.Text()); routeErr != nil {
		logrus.WithError(routeErr).Error("Failed to route to agent")
	}
}

// handleMediaMessage handles messages with media attachments
func (h *BotHandler) handleMediaMessage(hCtx HandlerContext, media []imbot.MediaAttachment) {
	// Get project path for storage, use default if not bound
	projectPath, ok := h.getProjectPath(hCtx)
	if !ok {
		projectPath = h.getDefaultProjectPath()
	}

	// Set platform-specific token on FileStore if needed
	if len(media) > 0 && strings.HasPrefix(media[0].URL, "tgfile://") {
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
	for _, attachment := range media {
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
	message := hCtx.Text()
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

	input := hCtx.Text()

	// For AskUserQuestion, try to parse as option selection first
	if latestReq.ToolName == "AskUserQuestion" {
		// Try to submit as a text selection
		if err := h.imPrompter.SubmitUserResponse(latestReq.ID, ask.Response{
			Type: "text",
			Data: input,
		}); err == nil {
			h.SendText(hCtx, fmt.Sprintf("✅ Selected: %s", input))
			logrus.WithFields(logrus.Fields{
				"request_id": latestReq.ID,
				"tool_name":  latestReq.ToolName,
				"user_id":    hCtx.SenderID,
				"selection":  input,
			}).Info("User selected option via text")
			return true
		}
	}

	// Try to parse the text as a standard permission response
	approved, remember, isValid := ask.ParseTextResponse(input)
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

// SendText sends a plain text message
// Note: Platform handles chunking internally via BaseBot.ChunkText()
func (h *BotHandler) SendText(hCtx HandlerContext, text string) {
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
	}
	// Forward context_token from incoming message metadata (required by Weixin)
	if hCtx.Message.Metadata != nil {
		if ct, ok := hCtx.Message.Metadata["context_token"].(string); ok {
			if opts.Metadata == nil {
				opts.Metadata = make(map[string]interface{})
			}
			opts.Metadata["context_token"] = ct
		}
	}
	resp, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts)
	_ = resp
	if err != nil {
		logrus.WithError(err).Warn("Failed to send message")
	}
}

// sendTextWithReply sends a text message as a reply to another message
// Note: Platform handles chunking internally via BaseBot.ChunkText()
func (h *BotHandler) sendTextWithReply(hCtx HandlerContext, text string, replyTo string) {
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		ReplyTo:   replyTo,
	}
	// Forward context_token from incoming message metadata (required by Weixin)
	if hCtx.Message.Metadata != nil {
		if ct, ok := hCtx.Message.Metadata["context_token"].(string); ok {
			if opts.Metadata == nil {
				opts.Metadata = make(map[string]interface{})
			}
			opts.Metadata["context_token"] = ct
		}
	}
	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts)
	if err != nil {
		logrus.WithError(err).Warn("Failed to send message")
	}
}

// sendTextWithActionKeyboard sends a text message with Clear/Bind action buttons
// Note: Manual chunking is kept here because the keyboard should only be attached to the LAST chunk.
// The platform's BaseBot.ChunkText() doesn't support "last chunk only" metadata yet.
// TODO: Add platform support for metadata that only applies to the last chunk.
func (h *BotHandler) sendTextWithActionKeyboard(hCtx HandlerContext, text string, replyTo string) {
	kb := BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	// Extract context_token from incoming message metadata (required by Weixin)
	var contextToken string
	if hCtx.Message.Metadata != nil {
		if ct, ok := hCtx.Message.Metadata["context_token"].(string); ok {
			contextToken = ct
		}
	}

	// Use public ChunkText API with smart break-point detection
	chunks := hCtx.Bot.ChunkText(text)
	for i, chunk := range chunks {
		opts := &imbot.SendMessageOptions{
			Text: chunk,
		}
		if replyTo != "" {
			opts.ReplyTo = replyTo
		}
		// Only attach keyboard to the last chunk
		if i == len(chunks)-1 {
			opts.Metadata = map[string]interface{}{
				"replyMarkup":        tgKeyboard,
				"_trackActionMenuID": true,
			}
		}
		// Forward context_token for Weixin
		if contextToken != "" {
			if opts.Metadata == nil {
				opts.Metadata = make(map[string]interface{})
			}
			opts.Metadata["context_token"] = contextToken
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
// Meta information includes: agent type, project path, chat_id, user_id, session_id
// behavior.Debug controls whether meta information is shown
// formatResponseWithMeta adds project/session/user metadata to the response
// Meta information includes: agent type, project path, chat_id, user_id, session_id
// Set showMeta=true to display meta (e.g., for help), false for regular messages
// behavior.Verbose controls whether processing messages are sent (handled elsewhere)
func (h *BotHandler) formatResponseWithMeta(meta ResponseMeta, response string, showMeta bool) string {
	var buf strings.Builder

	// Show meta information only when explicitly requested
	if showMeta {
		// Show agent indicator
		if meta.AgentType != "" {
			buf.WriteString(fmt.Sprintf(FormatAgentLine, GetAgentIcon(meta.AgentType), GetAgentDisplayName(meta.AgentType)))
		}

		// Always show project path (shortened)
		if meta.ProjectPath != "" {
			buf.WriteString(fmt.Sprintf(FormatProjectLine, IconProject, ShortenPath(meta.ProjectPath)))
		}

		// Show IDs for transparency
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
	}

	return buf.String() + response
}

// getOutputBehaviorForChat returns the output behavior for a specific chat
// Combines bot-level defaults with chat-level overrides
func (h *BotHandler) getOutputBehaviorForChat(chatID string) OutputBehavior {
	return h.botSetting.GetOutputBehavior()
}

// newStreamingMessageHandler creates a new streaming message handler
func (h *BotHandler) newStreamingMessageHandler(hCtx HandlerContext) *streamingMessageHandler {
	return newStreamingMessageHandler(hCtx.Bot, hCtx.ChatID, hCtx.MessageID, h.GetVerbose(hCtx.ChatID))
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

	if hCtx.IsDirect() {
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
	input := hCtx.Text()
	if filepath.IsAbs(hCtx.Text()) || strings.HasPrefix(input, "~") {
		// Absolute path or home-relative path
		var err error
		expandedPath, err = ExpandPath(input)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Invalid path: %v", err))
			return

		}
	} else if currentPath != "" {
		// Relative path - expand relative to current directory
		expandedPath = filepath.Join(currentPath, input)
	} else {
		// No current path, use ExpandPath
		var err error
		expandedPath, err = ExpandPath(hCtx.Text())
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

// BuildBindConfirmPrompt returns the text for bind confirmation prompt
func BuildBindConfirmPrompt(proposedPath string) string {
	return fmt.Sprintf("📁 *No project bound.*\n\nBind to current directory?\n\n`%s`", proposedPath)
}

// BuildCustomPathPrompt returns the text for custom path input prompt
func BuildCustomPathPrompt() string {
	return "✏️ *Please type the path you want to /cd:*\n\n" +
		"Examples:\n" +
		"• my-project (relative to current)\n" +
		"• ~/workspace/new-project\n" +
		"• /home/user/my-project\n\n" +
		"The directory will be created if it doesn't exist.\n\n" +
		"Type your path or click Cancel below."
}
