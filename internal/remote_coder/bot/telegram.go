package bot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/launcher"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/summarizer"
)

const (
	listSummaryLimit      = 160
	telegramStartRetries  = 10
	telegramStartDelay    = 5 * time.Second
	telegramStartMaxDelay = 5 * time.Minute
)

// Agent routing constants
const (
	agentClaudeCode = "claude_code"
)

// Bot command constants
const (
	botCommandHelp    = "help"
	botCommandBind    = "bind"
	botCommandJoin    = "join"
	botCommandProject = "project"
	botCommandStatus  = "status"
	botCommandClear   = "clear"
	botCommandBash    = "bash"
)

// Callback action constants
const (
	callbackActionClear   = "action:clear"
	callbackActionBind    = "action:bind"
	callbackProjectSwitch = "project:switch"
	callbackBindNav       = "bind:nav"
	callbackBindPrev      = "bind:prev"
	callbackBindNext      = "bind:next"
	callbackBindSelect    = "bind:select"
	callbackBindCancel    = "bind:cancel"
)

var defaultBashAllowlist = map[string]struct{}{
	"cd":  {},
	"ls":  {},
	"pwd": {},
}

// RunTelegramBot starts a Telegram bot that proxies messages to remote-coder sessions.
func RunTelegramBot(ctx context.Context, store *Store, sessionMgr *session.Manager) error {
	delay := telegramStartDelay
	for attempt := 1; attempt <= telegramStartRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		if err := runTelegramBotOnce(ctx, store, sessionMgr); err != nil {
			if attempt == telegramStartRetries {
				return err
			}
			logrus.WithError(err).Warnf("Remote-coder Telegram bot failed to start; retrying in %s (%d/%d)", delay, attempt, telegramStartRetries)
			if !sleepWithContext(ctx, delay) {
				return nil
			}
			delay *= 2
			if delay > telegramStartMaxDelay {
				delay = telegramStartMaxDelay
			}
			continue
		}
		return nil
	}
	return nil
}

func runTelegramBotOnce(ctx context.Context, store *Store, sessionMgr *session.Manager) error {
	if store == nil {
		return fmt.Errorf("bot store is nil")
	}

	settings, err := store.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to load bot settings: %w", err)
	}
	if strings.TrimSpace(settings.Token) == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}
	platform := strings.TrimSpace(settings.Platform)
	if platform == "" {
		platform = "telegram"
	}
	if platform != "telegram" {
		return fmt.Errorf("unsupported bot platform: %s", platform)
	}

	if sessionMgr == nil {
		return fmt.Errorf("session manager is nil")
	}

	claudeLauncher := launcher.NewClaudeCodeLauncher()
	summaryEngine := summarizer.NewEngine()
	directoryBrowser := NewDirectoryBrowser()

	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
		imbot.WithReconnectDelay(3000),
	)

	options := map[string]interface{}{
		"updateTimeout": 30,
	}
	if strings.TrimSpace(settings.ProxyURL) != "" {
		options["proxy"] = strings.TrimSpace(settings.ProxyURL)
	}

	err = manager.AddBot(&imbot.Config{
		Platform: imbot.PlatformTelegram,
		Enabled:  true,
		Auth: imbot.AuthConfig{
			Type:  "token",
			Token: settings.Token,
		},
		Options: options,
	})
	if err != nil {
		return fmt.Errorf("failed to start telegram bot: %w", err)
	}

	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
		if platform != imbot.PlatformTelegram {
			return
		}
		go handleTelegramMessage(ctx, manager, store, sessionMgr, claudeLauncher, summaryEngine, directoryBrowser, msg)
	})

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	<-ctx.Done()
	return nil
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// getReplyTarget returns the reply target ID for the message.
// Different platforms may use different IDs:
// - Telegram: Recipient.ID (chat ID)
// - DingTalk/Feishu: Recipient.ID (conversation ID)
func getReplyTarget(msg imbot.Message) string {
	return strings.TrimSpace(msg.Recipient.ID)
}

func handleTelegramMessage(
	ctx context.Context,
	manager *imbot.Manager,
	store *Store,
	sessionMgr *session.Manager,
	ccLauncher *launcher.ClaudeCodeLauncher,
	summaryEngine *summarizer.Engine,
	directoryBrowser *DirectoryBrowser,
	msg imbot.Message,
) {
	bot := manager.GetBot(msg.Platform)
	if bot == nil {
		return
	}

	// get recipient, different platform may require different source and id
	// Telegram: Recipient.ID (chat ID)
	// DingTalk/Feishu: Recipient.ID (conversation ID)
	chatID := getReplyTarget(msg)
	if chatID == "" {
		return
	}

	// Check if this is a callback query
	if isCallback, _ := msg.Metadata["is_callback"].(bool); isCallback {
		handleCallbackQuery(ctx, bot, store, sessionMgr, directoryBrowser, chatID, msg)
		return
	}

	// Determine if message is from direct chat or group
	isDirectChat := msg.IsDirectMessage()
	isGroupChat := msg.IsGroupMessage()

	// For group messages, check whitelist first (before text content check)
	// This allows showing /join prompt when bot is added to a group (non-text service message)
	if isGroupChat {
		logrus.Infof("Group chat ID: %s", chatID)

		// Check whitelist first
		if !store.IsGroupWhitelisted(chatID) {
			logrus.Debugf("Group %s is not whitelisted, ignoring message", chatID)
			sendText(bot, chatID, fmt.Sprintf("This group is not enabled. Please DM the bot with `/join %s` to enable.", chatID))
			return
		}
	}

	if !msg.IsTextContent() {
		sendText(bot, chatID, "Only text messages are supported.")
		return
	}

	text := strings.TrimSpace(msg.GetText())
	if text == "" {
		return
	}

	if isDirectChat {
		settings, err := store.GetSettings()
		if err != nil {
			logrus.WithError(err).Warn("Failed to load bot settings")
		}
		if settings.ChatIDLock != "" && chatID != settings.ChatIDLock {
			return
		}
	}

	if strings.HasPrefix(text, "/") {
		// Get the command (first word)
		fields := strings.Fields(text)
		if len(fields) > 0 {
			cmd := strings.ToLower(fields[0])
			switch cmd {
			case "/bot":
				handleBotCommand(ctx, bot, store, sessionMgr, directoryBrowser, chatID, text, msg.Sender.ID, isDirectChat, isGroupChat)
				return
			case "/clear":
				handleClearCommand(bot, store, sessionMgr, chatID)
				return
			}
		}
		// All other slash commands go to Claude Code
		sessionID, ok, err := store.GetSessionForChat(chatID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to load session mapping")
		}
		if ok && sessionID != "" {
			handleAgentMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agentClaudeCode, text, msg.Sender.ID, msg.ID)
			return
		}
		// No session - show guidance
		sendText(bot, chatID, "No active session. Use /bot bind <project_path> to create one.")
		return
	}

	// Check if waiting for custom path input
	if directoryBrowser.IsWaitingInput(chatID) {
		handleCustomPathInput(ctx, bot, store, sessionMgr, directoryBrowser, chatID, text, msg.Sender.ID, isDirectChat, isGroupChat)
		return
	}

	// In group chat, check for project binding (whitelist already checked above)
	if isGroupChat {
		if projectPath, ok := getProjectPathForGroup(store, chatID, string(msg.Platform)); ok {
			// Route to Claude Code with the bound project path
			handleAgentMessageWithProject(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agentClaudeCode, text, msg.Sender.ID, projectPath, msg.ID)
			return
		}
		// No binding, show guidance
		sendText(bot, chatID, "No project bound to this group. Use /bind <path> to bind a project.")
		return
	}

	// No agent mentioned - check if there's an active session to auto-route to cc
	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	if ok && sessionID != "" {
		// Has active session, auto-route to cc
		handleAgentMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agentClaudeCode, text, msg.Sender.ID, msg.ID)
		return
	}

	// No session - show guidance
	sendText(bot, chatID, "No active session. Use /bot bind <project_path> to create one.")
}

// handleAgentMessage routes message to the appropriate agent handler.
func handleAgentMessage(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	ccLauncher *launcher.ClaudeCodeLauncher,
	summaryEngine *summarizer.Engine,
	chatID string,
	agent string,
	text string,
	senderID string,
	replyTo string,
) {
	handleAgentMessageWithProject(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agent, text, senderID, "", replyTo)
}

// handleAgentMessageWithProject routes message to the appropriate agent handler with a specific project path.
func handleAgentMessageWithProject(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	ccLauncher *launcher.ClaudeCodeLauncher,
	summaryEngine *summarizer.Engine,
	chatID string,
	agent string,
	text string,
	senderID string,
	projectPathOverride string,
	replyTo string,
) {
	logrus.WithFields(logrus.Fields{
		"agent":    agent,
		"chatID":   chatID,
		"senderID": senderID,
	}).Infof("Agent call: %s", text)

	switch agent {
	case agentClaudeCode:
		handleClaudeCodeMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, text, senderID, projectPathOverride, replyTo)
	default:
		sendText(bot, chatID, fmt.Sprintf("Unknown agent: %s", agent))
	}
}

// getProjectPathForGroup retrieves the project path bound to a group chat.
func getProjectPathForGroup(store *Store, chatID string, platform string) (string, bool) {
	if store == nil || store.ChatStore() == nil {
		return "", false
	}
	path, ok, _ := store.ChatStore().GetProjectPath(chatID)
	return path, ok
}

// handleClaudeCodeMessage executes a message through Claude Code.
func handleClaudeCodeMessage(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	ccLauncher *launcher.ClaudeCodeLauncher,
	summaryEngine *summarizer.Engine,
	chatID string,
	text string,
	senderID string,
	projectPathOverride string,
	replyTo string,
) {
	if strings.TrimSpace(text) == "" {
		sendText(bot, chatID, "Please provide a message for Claude Code.")
		return
	}

	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}

	var sess *session.Session
	if ok && sessionID != "" {
		if s, exists := sessionMgr.GetOrLoad(sessionID); exists {
			sess = s
		}
	}

	// Auto-create session for group chats with project override (persistent, no expiration)
	if (sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed) && projectPathOverride != "" {
		sess = sessionMgr.Create()
		sessionID = sess.ID
		sessionMgr.SetContext(sessionID, "project_path", projectPathOverride)
		// Clear expiration for group sessions - they should be persistent
		sessionMgr.Update(sessionID, func(s *session.Session) {
			s.ExpiresAt = time.Time{} // Zero value means no expiration
		})
		if err := store.SetSessionForChat(chatID, sessionID); err != nil {
			logrus.WithError(err).Warn("Failed to save session mapping")
		}
		ok = true // Mark as having a valid session
	}

	if !ok || sessionID == "" {
		sendText(bot, chatID, "No session mapped. Use /bot bind <project_path> to create one.")
		return
	}

	// Refresh session activity for group chats to keep them alive
	if projectPathOverride != "" && sess != nil {
		sessionMgr.Update(sessionID, func(s *session.Session) {
			s.LastActivity = time.Now()
		})
	}

	// Use override project path if provided, otherwise get from session context
	projectPath := projectPathOverride
	if projectPath == "" && sess != nil && sess.Context != nil {
		if v, ok := sess.Context["project_path"]; ok {
			if pv, ok := v.(string); ok {
				projectPath = strings.TrimSpace(pv)
			}
		}
	}
	if projectPath == "" {
		sendText(bot, chatID, "Project path is required. Use /bot bind <project_path> first.")
		return
	}

	// Build meta for messages
	meta := ResponseMeta{
		ProjectPath: projectPath,
		SessionID:   sessionID,
		ChatID:      chatID,
		UserID:      senderID,
	}

	sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	})

	sessionMgr.SetRunning(sessionID)

	// Send status message to indicate processing started (with meta header and reply)
	sendTextWithReply(bot, chatID, formatResponseWithMeta(meta, "⏳ Processing..."), replyTo)

	// Use context.Background() to avoid cancellation when bot reconnects
	// The 10-minute timeout is sufficient to prevent runaway executions
	execCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := ccLauncher.Execute(execCtx, text, launcher.ExecuteOptions{
		ProjectPath: projectPath,
	})
	response := ""
	if result != nil {
		response = result.Output
		if err != nil && result.Error != "" {
			response = result.Error
		}
	} else if err != nil {
		response = fmt.Sprintf("Execution failed: %v", err)
	}

	if err != nil {
		sessionMgr.SetFailed(sessionID, response)
		logrus.WithError(err).Warn("Remote-coder execution failed")
		sendTextWithReply(bot, chatID, response, replyTo)
		return
	}

	sessionMgr.SetCompleted(sessionID, response)

	summary := summaryEngine.Summarize(response)
	sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "assistant",
		Content:   response,
		Summary:   summary,
		Timestamp: time.Now(),
	})

	// Send response with action keyboard (Clear/Bind buttons)
	sendTextWithActionKeyboard(bot, chatID, response, replyTo)
}

// handleBotCommand handles /bot <subcommand> commands
func handleBotCommand(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, directoryBrowser *DirectoryBrowser, chatID string, text string, senderID string, isDirectChat bool, isGroupChat bool) {
	fields := strings.Fields(text)
	subcmd := ""
	if len(fields) >= 2 {
		subcmd = strings.ToLower(strings.TrimSpace(fields[1]))
	}

	switch subcmd {
	case "", botCommandHelp:
		showBotHelp(bot, chatID, senderID, isDirectChat)
	case botCommandBind:
		if len(fields) < 3 {
			// Start interactive directory browser
			handleBindInteractive(ctx, bot, store, sessionMgr, directoryBrowser, chatID, senderID, isDirectChat, isGroupChat)
			return
		}
		handleBotBindCommand(ctx, bot, store, sessionMgr, chatID, fields[2:], senderID, isDirectChat, isGroupChat)
	case botCommandJoin:
		if !isDirectChat {
			sendText(bot, chatID, "/bot join can only be used in direct chat.")
			return
		}
		handleJoinCommand(bot, store, chatID, fields, senderID)
	case botCommandProject:
		handleBotProjectCommand(ctx, bot, store, sessionMgr, chatID, senderID, isDirectChat, isGroupChat)
	case botCommandStatus:
		handleBotStatusCommand(bot, store, sessionMgr, chatID)
	case botCommandClear:
		handleClearCommand(bot, store, sessionMgr, chatID)
	case botCommandBash:
		handleBashCommand(ctx, bot, store, sessionMgr, chatID, fields[1:])
	default:
		sendText(bot, chatID, fmt.Sprintf("Unknown bot command: %s\nUse /bot help for available commands.", subcmd))
	}
}

// showBotHelp displays the bot help message
func showBotHelp(bot imbot.Bot, chatID string, senderID string, isDirectChat bool) {
	var helpText string
	if isDirectChat {
		helpText = fmt.Sprintf(`Your User ID: %s

Bot Commands:
/bot help - Show this help
/bot bind [path] - Bind a project
/bot project - Show & switch projects
/bot status - Show session status
/bot clear - Clear session context
/bot bash <cmd> - Execute allowed bash (cd, ls, pwd)
/bot join <group> - Add group to whitelist

All other messages are sent to Claude Code.
Use /help to see Claude Code's commands.`, senderID)
	} else {
		helpText = fmt.Sprintf(`Group Chat ID: %s

Bot Commands:
/bot help - Show this help
/bot bind [path] - Bind a project to this group
/bot project - Show current project info
/bot status - Show session status
/bot clear - Clear session context

All other messages are sent to Claude Code.`, chatID)
	}
	sendText(bot, chatID, helpText)
}

// handleBotBindCommand handles /bot bind <path>
func handleBotBindCommand(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, fields []string, senderID string, isDirectChat bool, isGroupChat bool) {
	if len(fields) < 1 {
		sendText(bot, chatID, "Usage: /bot bind <project_path>")
		return
	}

	projectPath := strings.TrimSpace(strings.Join(fields, " "))
	if projectPath == "" {
		sendText(bot, chatID, "Usage: /bot bind <project_path>")
		return
	}

	// Expand and validate path
	expandedPath, err := ExpandPath(projectPath)
	if err != nil {
		sendText(bot, chatID, fmt.Sprintf("Invalid path: %v", err))
		return
	}

	if err := ValidateProjectPath(expandedPath); err != nil {
		sendText(bot, chatID, fmt.Sprintf("Path validation failed: %v", err))
		return
	}

	completeBind(ctx, bot, store, sessionMgr, chatID, expandedPath, senderID, isDirectChat, isGroupChat)
}

// handleBotStatusCommand handles /bot status
func handleBotStatusCommand(bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string) {
	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	if !ok || sessionID == "" {
		sendText(bot, chatID, "No session mapped. Use /bot bind <project_path> to create one.")
		return
	}
	sess, exists := sessionMgr.GetOrLoad(sessionID)
	if !exists {
		sendText(bot, chatID, "Session not found.")
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

	sendText(bot, chatID, strings.Join(statusParts, "\n"))
}

// handleBotProjectCommand handles /bot project - shows current project and list with keyboard
func handleBotProjectCommand(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, senderID string, isDirectChat bool, isGroupChat bool) {
	if store == nil || store.ChatStore() == nil {
		sendText(bot, chatID, "Store not available")
		return
	}

	chatStore := store.ChatStore()
	platform := string(imbot.PlatformTelegram)

	// Get current project path for this chat
	currentPath, _, _ := chatStore.GetProjectPath(chatID)

	// Build message text
	var buf strings.Builder
	if currentPath != "" {
		buf.WriteString(fmt.Sprintf("Current Project:\n📁 %s\n\n", currentPath))
	} else {
		buf.WriteString("No project bound to this chat.\n\n")
	}

	// Get all projects for user
	var projectPaths []string
	if isDirectChat {
		chats, err := chatStore.ListChatsByOwner(senderID, platform)
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

	_, err := bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
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
func handleProjectSwitch(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, projectPath string, senderID string) {
	if store == nil || store.ChatStore() == nil {
		sendText(bot, chatID, "Store not available")
		return
	}

	// Bind the project to this chat
	if err := store.ChatStore().BindProject(chatID, string(imbot.PlatformTelegram), projectPath, senderID); err != nil {
		sendText(bot, chatID, "Failed to switch project")
		return
	}

	// Create new session with the selected project
	sess := sessionMgr.Create()
	sessionMgr.SetContext(sess.ID, "project_path", projectPath)

	if err := store.SetSessionForChat(chatID, sess.ID); err != nil {
		logrus.WithError(err).Warn("Failed to update session mapping")
		sendText(bot, chatID, "Failed to switch project")
		return
	}

	logrus.Infof("Project switched: chat=%s path=%s session=%s", chatID, projectPath, sess.ID)
	sendText(bot, chatID, fmt.Sprintf("✅ Switched to: %s\nSession: %s", projectPath, sess.ID))
}

// handleClearCommand clears the current session context and creates a new one
func handleClearCommand(bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string) {
	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}

	var projectPath string
	if ok && sessionID != "" {
		if sess, exists := sessionMgr.GetOrLoad(sessionID); exists && sess.Context != nil {
			if v, ok := sess.Context["project_path"]; ok {
				if pv, ok := v.(string); ok {
					projectPath = pv
				}
			}
		}
	}

	// For group chats, also check group binding if no project path from session
	if projectPath == "" {
		if path, found := getProjectPathForGroup(store, chatID, string(imbot.PlatformTelegram)); found {
			projectPath = path
		}
	}

	if projectPath == "" {
		sendText(bot, chatID, "No project path found. Use /bot bind <project_path> to create a session first.")
		return
	}

	// Create new session with same project path
	sess := sessionMgr.Create()
	sessionMgr.SetContext(sess.ID, "project_path", projectPath)

	if err := store.SetSessionForChat(chatID, sess.ID); err != nil {
		logrus.WithError(err).Warn("Failed to update session mapping")
		sendText(bot, chatID, "Failed to clear context.")
		return
	}

	sendText(bot, chatID, fmt.Sprintf("Context cleared. New session: %s\nProject: %s", sess.ID, projectPath))
}

func handleBashCommand(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, fields []string) {
	if len(fields) < 2 {
		sendText(bot, chatID, "Usage: /bash <command>")
		return
	}
	settings, err := store.GetSettings()
	if err != nil {
		logrus.WithError(err).Warn("Failed to load bot settings")
	}
	allowlist := normalizeAllowlistToMap(settings.BashAllowlist)
	if len(allowlist) == 0 {
		allowlist = defaultBashAllowlist
	}
	subcommand := strings.ToLower(strings.TrimSpace(fields[1]))
	if _, ok := allowlist[subcommand]; !ok {
		sendText(bot, chatID, "Command not allowed.")
		return
	}

	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	var sess *session.Session
	if ok && sessionID != "" {
		if s, exists := sessionMgr.GetOrLoad(sessionID); exists {
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
	bashCwd, _, err := store.GetBashCwd(chatID)
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
				sendText(bot, chatID, "Unable to resolve working directory.")
				return
			}
			sendText(bot, chatID, cwd)
			return
		}
		sendText(bot, chatID, baseDir)
	case "cd":
		if len(fields) < 3 {
			sendText(bot, chatID, "Usage: /bash cd <path>")
			return
		}
		nextPath := strings.TrimSpace(strings.Join(fields[2:], " "))
		if nextPath == "" {
			sendText(bot, chatID, "Usage: /bash cd <path>")
			return
		}
		cdBase := baseDir
		if cdBase == "" {
			cwd, err := os.Getwd()
			if err != nil {
				sendText(bot, chatID, "Unable to resolve working directory.")
				return
			}
			cdBase = cwd
		}
		if !filepath.IsAbs(nextPath) {
			nextPath = filepath.Join(cdBase, nextPath)
		}
		if stat, err := os.Stat(nextPath); err != nil || !stat.IsDir() {
			sendText(bot, chatID, "Directory not found.")
			return
		}
		absPath, err := filepath.Abs(nextPath)
		if err == nil {
			nextPath = absPath
		}
		if err := store.SetBashCwd(chatID, nextPath); err != nil {
			logrus.WithError(err).Warn("Failed to update bash cwd")
		}
		sendText(bot, chatID, fmt.Sprintf("Bash working directory set to %s", nextPath))
	case "ls":
		if baseDir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				sendText(bot, chatID, "Unable to resolve working directory.")
				return
			}
			baseDir = cwd
		}
		var args []string
		if len(fields) > 2 {
			args = append(args, fields[2:]...)
		}
		execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(execCtx, "ls", args...)
		cmd.Dir = baseDir
		output, err := cmd.CombinedOutput()
		if err != nil && len(output) == 0 {
			sendText(bot, chatID, fmt.Sprintf("Command failed: %v", err))
			return
		}
		sendText(bot, chatID, strings.TrimSpace(string(output)))
	default:
		sendText(bot, chatID, "Command not allowed.")
	}
}

func normalizeAllowlistToMap(values []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, entry := range values {
		entry = strings.TrimSpace(strings.ToLower(entry))
		if entry == "" {
			continue
		}
		out[entry] = struct{}{}
	}
	return out
}

// handleCallbackQuery handles callback queries from inline keyboards
func handleCallbackQuery(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	directoryBrowser *DirectoryBrowser,
	chatID string,
	msg imbot.Message,
) {
	callbackData, _ := msg.Metadata["callback_data"].(string)
	if callbackData == "" {
		return
	}

	parts := imbot.ParseCallbackData(callbackData)
	if len(parts) == 0 {
		return
	}

	action := parts[0]

	switch action {
	case "action":
		if len(parts) < 2 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "clear":
			handleClearCommand(bot, store, sessionMgr, chatID)
		case "bind":
			// Start interactive bind
			handleBindInteractive(ctx, bot, store, sessionMgr, directoryBrowser, chatID, msg.Sender.ID, true, false)
		}

	case "project":
		if len(parts) < 3 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "switch":
			projectID := parts[2]
			handleProjectSwitch(ctx, bot, store, sessionMgr, chatID, projectID, msg.Sender.ID)
		}

	case "bind":
		if len(parts) < 2 {
			return
		}
		subAction := parts[1]
		switch subAction {
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
			if err := directoryBrowser.NavigateByIndex(chatID, index); err != nil {
				logrus.WithError(err).Warn("Failed to navigate directory")
				return
			}
			// Get message ID from metadata for editing
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(ctx, bot, directoryBrowser, chatID, msgID)

		case "up":
			// Navigate to parent directory
			if err := directoryBrowser.NavigateUp(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to navigate up")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(ctx, bot, directoryBrowser, chatID, msgID)

		case "prev":
			if err := directoryBrowser.PrevPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to previous page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(ctx, bot, directoryBrowser, chatID, msgID)

		case "next":
			if err := directoryBrowser.NextPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to next page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(ctx, bot, directoryBrowser, chatID, msgID)

		case "select":
			// Select current directory (path is in state)
			currentPath := directoryBrowser.GetCurrentPath(chatID)
			if currentPath == "" {
				logrus.Warn("No current path in bind flow")
				return
			}
			// Complete the bind
			completeBind(ctx, bot, store, sessionMgr, chatID, currentPath, msg.Sender.ID, true, false)
			directoryBrowser.Clear(chatID)

		case "custom":
			// Start custom path input mode
			handleCustomPathPrompt(ctx, bot, directoryBrowser, chatID)

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
				sendText(bot, chatID, fmt.Sprintf("Failed to create directory: %v", err))
				return
			}
			// Complete the bind
			completeBind(ctx, bot, store, sessionMgr, chatID, path, msg.Sender.ID, true, false)
			directoryBrowser.Clear(chatID)

		case "cancel":
			directoryBrowser.Clear(chatID)
			sendText(bot, chatID, "Bind cancelled.")
		}
	}
}

// handleBindInteractive starts an interactive directory browser for binding
func handleBindInteractive(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	directoryBrowser *DirectoryBrowser,
	chatID string,
	senderID string,
	isDirectChat bool,
	isGroupChat bool,
) {
	// Start from home directory
	_, err := directoryBrowser.Start(chatID)
	if err != nil {
		logrus.WithError(err).Error("Failed to start directory browser")
		sendText(bot, chatID, fmt.Sprintf("Failed to start directory browser: %v", err))
		return
	}

	logrus.Infof("Bind flow started for chat %s", chatID)

	// Send directory browser
	_, err = SendDirectoryBrowser(ctx, bot, directoryBrowser, chatID, "")
	if err != nil {
		logrus.WithError(err).Error("Failed to send directory browser")
		sendText(bot, chatID, fmt.Sprintf("Failed to send directory browser: %v", err))
		return
	}
}

// handleCustomPathPrompt sends the custom path input prompt
func handleCustomPathPrompt(
	ctx context.Context,
	bot imbot.Bot,
	directoryBrowser *DirectoryBrowser,
	chatID string,
) {
	// Ensure state exists
	state := directoryBrowser.GetState(chatID)
	if state == nil {
		// Start a new bind flow if none exists
		var err error
		state, err = directoryBrowser.Start(chatID)
		if err != nil {
			sendText(bot, chatID, fmt.Sprintf("Failed to start bind flow: %v", err))
			return
		}
	}

	// Set waiting for input state
	directoryBrowser.SetWaitingInput(chatID, true, "")

	// Send prompt with cancel keyboard
	kb := BuildCancelKeyboard()
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	result, err := bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
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
	directoryBrowser.SetWaitingInput(chatID, true, result.MessageID)
}

// handleCustomPathInput handles the user's custom path input
func handleCustomPathInput(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	directoryBrowser *DirectoryBrowser,
	chatID string,
	text string,
	senderID string,
	isDirectChat bool,
	isGroupChat bool,
) {
	// Get current path from browser state
	state := directoryBrowser.GetState(chatID)
	currentPath := ""
	if state != nil {
		currentPath = state.CurrentPath
	}

	// Expand path relative to current directory
	var expandedPath string
	if filepath.IsAbs(text) || strings.HasPrefix(text, "~") {
		// Absolute path or home-relative path
		var err error
		expandedPath, err = ExpandPath(text)
		if err != nil {
			sendText(bot, chatID, fmt.Sprintf("Invalid path: %v", err))
			return
		}
	} else if currentPath != "" {
		// Relative path - expand relative to current directory
		expandedPath = filepath.Join(currentPath, text)
	} else {
		// No current path, use ExpandPath
		var err error
		expandedPath, err = ExpandPath(text)
		if err != nil {
			sendText(bot, chatID, fmt.Sprintf("Invalid path: %v", err))
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
			handleCreateConfirm(ctx, bot, directoryBrowser, chatID, expandedPath)
			return
		}
		sendText(bot, chatID, fmt.Sprintf("Cannot access path: %v", err))
		return
	}

	if !info.IsDir() {
		sendText(bot, chatID, "The path is not a directory. Please provide a directory path.")
		return
	}

	// Path exists and is a directory, complete the bind
	completeBind(ctx, bot, store, sessionMgr, chatID, expandedPath, senderID, isDirectChat, isGroupChat)
	directoryBrowser.Clear(chatID)
}

// handleCreateConfirm sends a confirmation prompt for creating a directory
func handleCreateConfirm(
	ctx context.Context,
	bot imbot.Bot,
	directoryBrowser *DirectoryBrowser,
	chatID string,
	path string,
) {
	// Reset waiting input state (no longer waiting for text input)
	directoryBrowser.SetWaitingInput(chatID, false, "")

	kb, text := BuildCreateConfirmKeyboard(path)
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	_, err := bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
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

// completeBind completes the project binding process
func completeBind(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	chatID string,
	projectPath string,
	senderID string,
	isDirectChat bool,
	isGroupChat bool,
) {
	// Expand path (handles ~, etc.)
	expandedPath, err := ExpandPath(projectPath)
	if err != nil {
		sendText(bot, chatID, fmt.Sprintf("Invalid path: %v", err))
		return
	}

	// Only validate if the path should already exist
	// For newly created paths, skip this check
	if _, err := os.Stat(expandedPath); err == nil {
		if err := ValidateProjectPath(expandedPath); err != nil {
			sendText(bot, chatID, fmt.Sprintf("Path validation failed: %v", err))
			return
		}
	}

	platform := string(imbot.PlatformTelegram)

	// Bind project to chat using ChatStore
	if err := store.ChatStore().BindProject(chatID, platform, expandedPath, senderID); err != nil {
		sendText(bot, chatID, fmt.Sprintf("Failed to bind project: %v", err))
		return
	}

	// Create session and bind to chat
	sess := sessionMgr.Create()
	sessionMgr.SetContext(sess.ID, "project_path", expandedPath)

	if err := store.SetSessionForChat(chatID, sess.ID); err != nil {
		logrus.WithError(err).Warn("Failed to save session mapping")
		sendText(bot, chatID, fmt.Sprintf("Project bound but failed to create session: %v", err))
		return
	}

	logrus.Infof("Project bound: chat=%s path=%s session=%s", chatID, expandedPath, sess.ID)

	if isDirectChat {
		sendText(bot, chatID, fmt.Sprintf("✅ Project bound: %s\nSession: %s\n\nYou can now send messages directly.", expandedPath, sess.ID))
	} else {
		sendText(bot, chatID, fmt.Sprintf("✅ Group bound to project: %s", expandedPath))
	}
}

// handleJoinCommand handles the /join command to add a group to whitelist
func handleJoinCommand(bot imbot.Bot, store *Store, chatID string, fields []string, senderID string) {
	if len(fields) < 2 {
		sendText(bot, chatID, "Usage: /join <group_id|@username|invite_link>")
		return
	}

	input := strings.TrimSpace(strings.Join(fields[1:], " "))
	if input == "" {
		sendText(bot, chatID, "Usage: /join <group_id|@username|invite_link>")
		return
	}

	// Try to cast bot to TelegramBot interface
	tgBot, ok := imbot.AsTelegramBot(bot)
	if !ok {
		sendText(bot, chatID, "Join command is only supported for Telegram bot.")
		return
	}

	// Resolve the chat ID
	groupID, err := tgBot.ResolveChatID(input)
	if err != nil {
		logrus.WithError(err).Error("Failed to resolve chat ID")
		sendText(bot, chatID, fmt.Sprintf("Failed to resolve chat ID: %v\n\nNote: Bot must already be a member of the group to add it to whitelist.", err))
		return
	}

	// Check if already whitelisted
	if store.IsGroupWhitelisted(groupID) {
		sendText(bot, chatID, fmt.Sprintf("Group %s is already in whitelist.", groupID))
		return
	}

	// Add group to whitelist
	platform := string(imbot.PlatformTelegram)
	if err := store.AddGroupToWhitelist(groupID, platform, senderID); err != nil {
		logrus.WithError(err).Error("Failed to add group to whitelist")
		sendText(bot, chatID, fmt.Sprintf("Failed to add group to whitelist: %v", err))
		return
	}

	sendText(bot, chatID, fmt.Sprintf("Successfully added group to whitelist.\nGroup ID: %s", groupID))
	logrus.Infof("Group %s added to whitelist by %s", groupID, senderID)
}

func lastAssistantSummary(sessionMgr *session.Manager, sessionID string) string {
	if sessionMgr == nil {
		return ""
	}
	msgs, ok := sessionMgr.GetMessages(sessionID)
	if !ok {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Role != "assistant" {
			continue
		}
		text := strings.TrimSpace(msg.Summary)
		if text == "" {
			text = strings.TrimSpace(msg.Content)
		}
		if text == "" {
			return ""
		}
		if len(text) > listSummaryLimit {
			return text[:listSummaryLimit] + "..."
		}
		return text
	}
	return ""
}

// ResponseMeta contains metadata for response formatting
type ResponseMeta struct {
	ProjectPath string
	SessionID   string
	ChatID      string
	UserID      string
}

// formatResponseWithMeta adds project/session/user metadata to the response for better readability.
func formatResponseWithMeta(meta ResponseMeta, response string) string {
	var buf strings.Builder
	if meta.ProjectPath != "" {
		// Show only the last 2 directories for brevity
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

func sendText(bot imbot.Bot, chatID string, text string) {
	for _, chunk := range chunkText(text, imbot.DefaultMessageLimit) {
		_, err := bot.SendText(context.Background(), chatID, chunk)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return
		}
	}
}

// sendTextWithReply sends a text message as a reply to another message
func sendTextWithReply(bot imbot.Bot, chatID string, text string, replyTo string) {
	for _, chunk := range chunkText(text, imbot.DefaultMessageLimit) {
		_, err := bot.SendMessage(context.Background(), chatID, &imbot.SendMessageOptions{
			Text:    chunk,
			ReplyTo: replyTo,
		})
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return
		}
	}
}

func chunkText(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	remaining := text
	for len(remaining) > 0 {
		if len(remaining) <= limit {
			chunks = append(chunks, remaining)
			break
		}
		chunks = append(chunks, remaining[:limit])
		remaining = remaining[limit:]
	}
	return chunks
}

// sendTextWithActionKeyboard sends a text message with Clear/Bind action buttons
func sendTextWithActionKeyboard(bot imbot.Bot, chatID string, text string, replyTo string) {
	// Build action keyboard
	kb := BuildActionKeyboard()
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	// Send with keyboard on the last chunk
	chunks := chunkText(text, imbot.DefaultMessageLimit)
	for i, chunk := range chunks {
		opts := &imbot.SendMessageOptions{
			Text: chunk,
		}
		if replyTo != "" {
			opts.ReplyTo = replyTo
		}
		// Only add keyboard to the last chunk
		if i == len(chunks)-1 {
			opts.Metadata = map[string]interface{}{
				"replyMarkup": tgKeyboard,
			}
		}

		_, err := bot.SendMessage(context.Background(), chatID, opts)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return
		}
	}
}

// convertActionKeyboardToTelegram converts imbot.InlineKeyboardMarkup to tgbotapi.InlineKeyboardMarkup
func convertActionKeyboardToTelegram(kb imbot.InlineKeyboardMarkup) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, row := range kb.InlineKeyboard {
		var buttons []tgbotapi.InlineKeyboardButton
		for _, btn := range row {
			tgBtn := tgbotapi.InlineKeyboardButton{
				Text: btn.Text,
			}
			if btn.CallbackData != "" {
				tgBtn.CallbackData = &btn.CallbackData
			}
			if btn.URL != "" {
				tgBtn.URL = &btn.URL
			}
			buttons = append(buttons, tgBtn)
		}
		rows = append(rows, buttons)
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}
