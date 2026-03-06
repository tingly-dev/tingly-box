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

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	mock "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/summarizer"
)

// BotHandler encapsulates all bot message handling logic and dependencies
type BotHandler struct {
	ctx                context.Context
	botSetting         BotSetting
	chatStore          *ChatStore
	sessionMgr         *session.Manager
	agentBoot          *agentboot.AgentBoot
	summaryEngine      *summarizer.Engine
	directoryBrowser   *DirectoryBrowser   // DEPRECATED: Use directoryBrowserV2 instead
	directoryBrowserV2 *DirectoryBrowserV2 // New interaction-based directory browser
	manager            *imbot.Manager
	imPrompter         *IMPrompter
	fileStore          *FileStore
	interaction        *imbot.InteractionHandler // New interaction handler

	// runningCancel tracks cancel functions for active executions per chatID
	runningCancel   map[string]context.CancelFunc
	runningCancelMu sync.RWMutex

	// pendingBinds tracks bind confirmation requests for unbound chats
	pendingBinds   map[string]*PendingBind
	pendingBindsMu sync.RWMutex
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
	chatStore *ChatStore,
	sessionMgr *session.Manager,
	agentBoot *agentboot.AgentBoot,
	summaryEngine *summarizer.Engine,
	directoryBrowser *DirectoryBrowser,
	manager *imbot.Manager,
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

	return &BotHandler{
		ctx:                ctx,
		botSetting:         botSetting,
		chatStore:          chatStore,
		sessionMgr:         sessionMgr,
		agentBoot:          agentBoot,
		summaryEngine:      summaryEngine,
		directoryBrowser:   directoryBrowser,        // DEPRECATED: Kept for backward compatibility
		directoryBrowserV2: NewDirectoryBrowserV2(), // New interaction-based browser
		manager:            manager,
		imPrompter:         imPrompter,
		fileStore:          fileStore,
		interaction:        interactionHandler,
		runningCancel:      make(map[string]context.CancelFunc),
		pendingBinds:       make(map[string]*PendingBind),
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

	// Check for active session or show project selection
	sessionID, ok, err := h.chatStore.GetSession(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	if ok && sessionID != "" {
		h.handleAgentMessage(hCtx, agentClaudeCode, hCtx.Text, "")
		return
	}

	h.showProjectSelectionOrGuidance(hCtx)
}

// handleGroupMessage handles messages from group chat
func (h *BotHandler) handleGroupMessage(hCtx HandlerContext) {
	logrus.Infof("Group chat ID: %s", hCtx.ChatID)

	// Check whitelist first
	if !h.chatStore.IsWhitelisted(hCtx.ChatID) {
		logrus.Debugf("Group %s is not whitelisted, ignoring message", hCtx.ChatID)
		h.SendText(hCtx, fmt.Sprintf("This group is not enabled. Please DM the bot with `/join %s` to enable.", hCtx.ChatID))
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

	// Check for project binding
	if projectPath, ok := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); ok {
		h.handleAgentMessage(hCtx, agentClaudeCode, hCtx.Text, projectPath)
		return
	}

	h.SendText(hCtx, "No project bound to this group. Use /bind <path> to bind a project.")
}

// handleMediaMessage handles messages with media attachments
func (h *BotHandler) handleMediaMessage(hCtx HandlerContext) {
	// Get project path for storage
	projectPath, ok := h.getProjectPath(hCtx)
	if !ok {
		h.SendText(hCtx, "No project bound. Please bind a project first.")
		return
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
		sessionID, ok, _ := h.chatStore.GetSession(hCtx.ChatID)
		if ok {
			// Get project path from session context
			if session, exists := h.sessionMgr.Get(sessionID); exists && session.Context != nil {
				if projectPath, ok := session.Context["project_path"].(string); ok {
					return projectPath, true
				}
			}
		}
	} else {
		// Group chat: get bound project path
		return getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform))
	}
	return "", false
}

// handleSlashCommands handles slash commands
func (h *BotHandler) handleSlashCommands(hCtx HandlerContext) {
	fields := strings.Fields(hCtx.Text)
	if len(fields) == 0 {
		return
	}

	cmd := strings.ToLower(fields[0])

	switch cmd {
	case "/bot":
		h.handleBotCommand(hCtx, fields)
		return
	case "/bot_help", "/bot_h":
		h.showBotHelp(hCtx)
		return
	case "/bot_bind", "/bot_b":
		if len(fields) < 2 {
			h.handleBindInteractive(hCtx)
			return
		}
		h.handleBotBindCommand(hCtx, fields[1:])
	case "/bot_join", "/bot_j":
		if !hCtx.IsDirect {
			h.SendText(hCtx, "/bot_join can only be used in direct chat.")
			return
		}
		h.handleJoinCommand(hCtx, fields)
		return
	case "/bot_project", "/bot_p":
		h.handleBotProjectCommand(hCtx)
		return
	case "/bot_status", "/bot_s":
		h.handleBotStatusCommand(hCtx)
		return
	case "/bot_clear":
		h.handleClearCommand(hCtx)
		return
	case "/bot_bash":
		h.handleBashCommand(hCtx, fields[1:])
		return
	case "/clear":
		h.handleClearCommand(hCtx)
		return
	case "/mock":
		// Mock agent command for testing
		mockText := strings.TrimSpace(strings.TrimPrefix(hCtx.Text, "/mock"))
		h.handleAgentMessage(hCtx, agentMock, mockText, "")
		return
	case "/start", "/help", "/h":
		h.showBotHelp(hCtx)
		return
	}

	// All other slash commands go to Claude Code
	sessionID, ok, err := h.chatStore.GetSession(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	if ok && sessionID != "" {
		h.handleAgentMessage(hCtx, agentClaudeCode, hCtx.Text, "")
		return
	}

	h.showProjectSelectionOrGuidance(hCtx)
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
// DEPRECATED: Uses old V1 keyboard pattern. New code should use BuildActionInteractionsV2()
// and send via the interaction handler for multi-platform support.
func (h *BotHandler) sendTextWithActionKeyboard(hCtx HandlerContext, text string, replyTo string) {
	// TODO: Migrate to v2 interaction system
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
				"replyMarkup": tgKeyboard,
			}
		}

		_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return
		}
	}
}

// formatResponseWithMeta adds project/session/user metadata to the response
func (h *BotHandler) formatResponseWithMeta(meta ResponseMeta, response string) string {
	var buf strings.Builder
	if meta.ProjectPath != "" {
		shortPath := meta.ProjectPath
		parts := strings.Split(meta.ProjectPath, string(filepath.Separator))
		if len(parts) > 2 {
			shortPath = filepath.Join(parts[len(parts)-2], parts[len(parts)-1])
		}
		buf.WriteString(fmt.Sprintf("📁 %s\n", shortPath))
	}
	if meta.ChatID != "" {
		buf.WriteString(fmt.Sprintf("💬 %s\n", meta.ChatID))
	}
	if meta.UserID != "" {
		buf.WriteString(fmt.Sprintf("👤 %s\n", meta.UserID))
	}
	if meta.SessionID != "" {
		buf.WriteString(fmt.Sprintf("🔄 %s\n", meta.SessionID))
	}

	buf.WriteString("━━━━━━━━━━━━━━━━━━━━\n\n")
	return buf.String() + response
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

	sessionID, ok, err := h.chatStore.GetSession(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}

	var sess *session.Session
	if ok && sessionID != "" {
		if s, exists := h.sessionMgr.GetOrLoad(sessionID); exists {
			sess = s
		}
	}

	// Auto-create session for group chats with project override (persistent, no expiration)
	if (sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed) && projectPathOverride != "" {
		sess = h.sessionMgr.Create()
		sessionID = sess.ID
		h.sessionMgr.SetContext(sessionID, "project_path", projectPathOverride)
		// Clear expiration for group sessions
		h.sessionMgr.Update(sessionID, func(s *session.Session) {
			s.ExpiresAt = time.Time{} // Zero value means no expiration
		})
		if err := h.chatStore.SetSession(hCtx.ChatID, sessionID); err != nil {
			logrus.WithError(err).Warn("Failed to save session mapping")
		}
		ok = true
	}

	if !ok || sessionID == "" {
		h.SendText(hCtx, "No session mapped. Use /bot_bind <project_path> to create one.")
		return
	}

	// Refresh session activity for group chats
	if projectPathOverride != "" && sess != nil {
		h.sessionMgr.Update(sessionID, func(s *session.Session) {
			s.LastActivity = time.Now()
		})
	}

	// Get project path
	projectPath := projectPathOverride
	if projectPath == "" && sess != nil && sess.Context != nil {
		if v, ok := sess.Context["project_path"]; ok {
			if pv, ok := v.(string); ok {
				projectPath = strings.TrimSpace(pv)
			}
		}
	}
	if projectPath == "" {
		h.SendText(hCtx, "Project path is required. Use /bot_bind <project_path> first.")
		return
	}

	// Build meta
	meta := ResponseMeta{
		ProjectPath: projectPath,
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
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, "⏳ Processing..."), hCtx.MessageID)

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
			"replyMarkup": tgKeyboard,
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

	// Get or create a session for mock agent (simpler than claude code)
	sessionID, ok, err := h.chatStore.GetSession(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}

	var sess *session.Session
	if ok && sessionID != "" {
		if s, exists := h.sessionMgr.GetOrLoad(sessionID); exists {
			sess = s
		}
	}

	// Create new session if needed
	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed {
		sess = h.sessionMgr.Create()
		sessionID = sess.ID
		if projectPathOverride != "" {
			h.sessionMgr.SetContext(sessionID, "project_path", projectPathOverride)
		}
		if err := h.chatStore.SetSession(hCtx.ChatID, sessionID); err != nil {
			logrus.WithError(err).Warn("Failed to save session mapping")
		}
	}

	// Get project path (optional for mock)
	projectPath := projectPathOverride
	if projectPath == "" && sess != nil && sess.Context != nil {
		if v, ok := sess.Context["project_path"]; ok {
			if pv, ok := v.(string); ok {
				projectPath = strings.TrimSpace(pv)
			}
		}
	}

	// Build meta
	meta := ResponseMeta{
		ProjectPath: projectPath,
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
	h.sendTextWithReply(hCtx, h.formatResponseWithMeta(meta, "🧪 Mock agent processing..."), hCtx.MessageID)

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
		helpText = fmt.Sprintf(`Your User ID: %s

Bot Commands:
/help, /h - Show this help
/stop - Stop current task
/clear - Clear context, stop task, and create new session
/bot_bind [path] - Bind a project
/bot_project - Show & switch projects
/bot_status - Show session status
/bot_bash <cmd> - Execute allowed bash (cd, ls, pwd)
/bot_join <group> - Add group to whitelist
/mock <msg> - Test with mock agent (permission flow)

All other messages are sent to Claude Code.`, hCtx.SenderID)
	} else {
		helpText = fmt.Sprintf(`Group Chat ID: %s

Bot Commands:
/help, /h - Show this help
/stop - Stop current task
/clear - Clear context, stop task, and create new session
/bot bind [path], /bot_bind [path] - Bind a project to this group
/bot_project - Show current project info
/bot_status - Show session status
/mock <msg> - Test with mock agent (permission flow)

All other messages are sent to Claude Code.`, hCtx.ChatID)
	}
	h.SendText(hCtx, helpText)
}

// handleBotBindCommand handles /bot bind <path>
func (h *BotHandler) handleBotBindCommand(hCtx HandlerContext, fields []string) {
	if len(fields) < 1 {
		h.SendText(hCtx, "Usage: /bot_bind <project_path>")
		return
	}

	projectPath := strings.TrimSpace(strings.Join(fields, " "))
	if projectPath == "" {
		h.SendText(hCtx, "Usage: /bot_bind <project_path>")
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
	sessionID, ok, err := h.chatStore.GetSession(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	if !ok || sessionID == "" {
		h.SendText(hCtx, "No session mapped. Use /bot_bind <project_path> to create one.")
		return
	}
	sess, exists := h.sessionMgr.GetOrLoad(sessionID)
	if !exists {
		h.SendText(hCtx, "Session not found.")
		return
	}

	// Build status message
	var statusParts []string
	statusParts = append(statusParts, fmt.Sprintf("Session: %s", sessionID))
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
	if sess.Context != nil {
		if v, ok := sess.Context["project_path"]; ok {
			if pv, ok := v.(string); ok {
				statusParts = append(statusParts, fmt.Sprintf("Project: %s", pv))
			}
		}
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
	sessionID, ok, err := h.chatStore.GetSession(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}

	var projectPath string
	if ok && sessionID != "" {
		if sess, exists := h.sessionMgr.GetOrLoad(sessionID); exists && sess.Context != nil {
			if v, ok := sess.Context["project_path"]; ok {
				if pv, ok := v.(string); ok {
					projectPath = pv
				}
			}
		}
	}

	// For group chats, also check group binding if no project path from session
	if projectPath == "" {
		if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
			projectPath = path
		}
	}

	if projectPath == "" {
		h.SendText(hCtx, "No project path found. Use /bot_bind <project_path> to create a session first.")
		return
	}

	// Create new session with same project path
	sess := h.sessionMgr.Create()
	h.sessionMgr.SetContext(sess.ID, "project_path", projectPath)

	if err := h.chatStore.SetSession(hCtx.ChatID, sess.ID); err != nil {
		logrus.WithError(err).Warn("Failed to update session mapping")
		h.SendText(hCtx, "Failed to clear context.")
		return
	}

	h.SendText(hCtx, fmt.Sprintf("Context cleared. New session: %s\nProject: %s", sess.ID, projectPath))
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
	platform := string(imbot.PlatformTelegram)

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
			"replyMarkup": tgKeyboard,
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

	// Create a new session
	sess := h.sessionMgr.Create()
	sessionID := sess.ID
	h.sessionMgr.SetContext(sessionID, "project_path", pending.ProposedPath)
	// Clear expiration for direct chat sessions
	h.sessionMgr.Update(sessionID, func(s *session.Session) {
		s.ExpiresAt = time.Time{} // Zero value means no expiration
	})

	if err := h.chatStore.SetSession(hCtx.ChatID, sessionID); err != nil {
		logrus.WithError(err).Warn("Failed to save session mapping")
	}

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

	platform := string(imbot.PlatformTelegram)

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
			"replyMarkup": tgKeyboard,
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
	if err := h.chatStore.BindProject(hCtx.ChatID, string(imbot.PlatformTelegram), projectPath, hCtx.SenderID); err != nil {
		h.SendText(hCtx, "Failed to switch project")
		return
	}

	// Create new session with the selected project
	sess := h.sessionMgr.Create()
	h.sessionMgr.SetContext(sess.ID, "project_path", projectPath)

	if err := h.chatStore.SetSession(hCtx.ChatID, sess.ID); err != nil {
		logrus.WithError(err).Warn("Failed to update session mapping")
		h.SendText(hCtx, "Failed to switch project")
		return
	}

	logrus.Infof("Project switched: chat=%s path=%s session=%s", hCtx.ChatID, projectPath, sess.ID)
	h.SendText(hCtx, fmt.Sprintf("✅ Switched to: %s\nSession: %s", projectPath, sess.ID))
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

	platform := string(imbot.PlatformTelegram)

	// Bind project to chat using ChatStore
	if err := h.chatStore.BindProject(hCtx.ChatID, platform, expandedPath, hCtx.SenderID); err != nil {
		h.SendText(hCtx, fmt.Sprintf("Failed to bind project: %v", err))
		return
	}

	// Create session and bind to chat
	sess := h.sessionMgr.Create()
	h.sessionMgr.SetContext(sess.ID, "project_path", expandedPath)

	if err := h.chatStore.SetSession(hCtx.ChatID, sess.ID); err != nil {
		logrus.WithError(err).Warn("Failed to save session mapping")
		h.SendText(hCtx, fmt.Sprintf("Project bound but failed to create session: %v", err))
		return
	}

	logrus.Infof("Project bound: chat=%s path=%s session=%s", hCtx.ChatID, expandedPath, sess.ID)

	if hCtx.IsDirect {
		h.SendText(hCtx, fmt.Sprintf("✅ Project bound: %s\nSession: %s\n\nYou can now send messages directly.", expandedPath, sess.ID))
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
	platform := string(imbot.PlatformTelegram)
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

	sessionID, ok, err := h.chatStore.GetSession(hCtx.ChatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	var sess *session.Session
	if ok && sessionID != "" {
		if s, exists := h.sessionMgr.GetOrLoad(sessionID); exists {
			sess = s
		}
	}
	projectPath := ""
	if sess != nil && sess.Context != nil {
		if v, ok := sess.Context["project_path"]; ok {
			if pv, ok := v.(string); ok {
				projectPath = pv
			}
		}
	}
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
// DEPRECATED: Use HandleCallbackQueryV2() for new interaction system
// This method handles legacy callbacks for backward compatibility
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
			h.handleClearCommand(hCtx)
		case "bind":
			// Start interactive bind
			h.handleBindInteractive(hCtx)
		case "project":
			// Start interactive bind
			h.handleBotProjectCommand(hCtx)
		}

	case "project":
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
			// Complete the bind
			h.completeBind(hCtx, currentPath)
			h.directoryBrowser.Clear(chatID)

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
			// Complete the bind
			h.completeBind(hCtx, path)
			h.directoryBrowser.Clear(chatID)

		case "cancel":
			h.directoryBrowser.Clear(chatID)
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
			"replyMarkup": tgKeyboard,
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
			"replyMarkup": tgKeyboard,
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
