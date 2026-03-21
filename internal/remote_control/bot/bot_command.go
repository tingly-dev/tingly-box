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
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
)

// Bot command subcommand constants (used after /bot prefix)
const (
	botCommandHelp    = "help"
	botCommandBind    = "bind"
	botCommandJoin    = "join"
	botCommandProject = "project"
	botCommandStatus  = "status"
	botCommandClear   = "clear"
	botCommandBash    = "bash"
)

// Slash command constants with aliases
// Primary command is the recommended one to show in help and error messages
var (
	// Help commands
	cmdHelpPrimary = "/help"
	cmdHelpAliases = []string{"/h", "/", "/start"}

	// Bind/CD commands - /cd is primary for simplicity
	cmdBindPrimary = "/cd"
	cmdBindAliases = []string{"/bind", "/bot_bind", "/bot_b"}

	// Join commands - /join is primary
	cmdJoinPrimary = "/join"

	// Project commands - /project is primary
	cmdProjectPrimary = "/project"

	// Status commands - /status is primary
	cmdStatusPrimary = "/status"

	// Clear commands - /clear is primary
	cmdClearPrimary = "/clear"

	// Bash commands - /bash is primary
	cmdBashPrimary = "/bash"

	// Yolo command - toggle auto-approve mode for current session
	cmdYoloPrimary = "/yolo"

	// Verbose commands - control message verbosity
	cmdVerbosePrimary   = "/verbose"
	cmdNoVerbosePrimary = "/noverbose"
)

// Constants for bot help messages
const (
	directHelpTemplate = `Your User ID: %s

Bot Commands:
/help - Show this help
/stop - Stop current task
/interrupt - Alias for /stop
/clear - Clear context, stop task, and create new session
/cd [path] - Bind and cd into a project
/project - Show & switch projects
/status - Show session status
/bash <cmd> - Execute allowed bash (cd, ls, pwd)
/join <group> - Add group to whitelist
/mock <msg> - Test with mock agent (permission flow)
/yolo - Toggle auto-approve mode for current session (Claude Code only)
/verbose - Show all message details (default)
/noverbose - Hide intermediate messages, show only final results

@cc to handoff control to Claude Code.
@tb to handoff control to Tingly Box Smart Guide.`

	groupHelpTemplate = `Group Chat ID: %s

Bot Commands:
/help - Show this help
/stop - Stop current task
/interrupt - Alias for /stop
/clear - Clear context, stop task, and create new session
/cd [path] - Bind and cd into a project to this group
/project - Show current project info
/status - Show session status
/mock <msg> - Test with mock agent (permission flow)
/yolo - Toggle auto-approve mode for current session (Claude Code only)
/verbose - Show all message details (default)
/noverbose - Hide intermediate messages, show only final results

@cc to handoff control to Claude Code.
@tb to handoff control to Tingly Box Smart Guide.`
)

// handleBotCommand handles /bot <subcommand> commands
func (h *BotHandler) handleBotCommand(hCtx HandlerContext, fields []string) {
	subcmd := ""
	if len(fields) >= 2 {
		subcmd = strings.ToLower(strings.TrimSpace(fields[1]))
	}

	switch subcmd {
	case "", botCommandHelp:
		h.handleBotHelpCommand(hCtx)
	case botCommandBind:
		if len(fields) < 3 {
			h.handleBindInteractive(hCtx)
			return
		}
		h.handleBotBindCommand(hCtx, fields[2:])
	case botCommandJoin:
		if hCtx.IsDirect() {
			h.handleJoinCommand(hCtx, fields)
			return
		} else {
			h.SendText(hCtx, "/bot join can only be used in general chat.")
			return
		}
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

// isCommandMatch checks if the given command matches the primary or any alias
func isCommandMatch(cmd string, primary string, aliases []string) bool {
	if cmd == primary {
		return true
	}
	for _, alias := range aliases {
		if cmd == alias {
			return true
		}
	}
	return false
}

// handleBotHelpCommand displays the bot help message
func (h *BotHandler) handleBotHelpCommand(hCtx HandlerContext) {
	// Try to use command registry for dynamic help generation
	if h.commandRegistry != nil {
		helpText := h.commandRegistry.BuildHelpText(hCtx.IsDirect())
		helpText += fmt.Sprintf("\nYour ID: %s", hCtx.SenderID)
		h.SendText(hCtx, helpText)
		return
	}

	// Fallback to hardcoded templates
	var helpText string
	if hCtx.IsDirect() {
		helpText = fmt.Sprintf(directHelpTemplate, hCtx.SenderID)
	} else {
		helpText = fmt.Sprintf(groupHelpTemplate, hCtx.ChatID)
	}
	h.SendText(hCtx, helpText)
}

// isStopCommand checks if the text is a stop command
// Supports: /stop, stop, /interrupt, /clear
func isStopCommand(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return trimmed == "/stop" || trimmed == "stop" || trimmed == "/interrupt" || trimmed == "/clear"
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

// handleSlashCommands handles slash commands
func (h *BotHandler) handleSlashCommands(hCtx HandlerContext) {
	input := hCtx.Text()
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return
	}

	cmd := strings.ToLower(fields[0])

	// Special case: /bot subcommands are handled separately
	if cmd == "/bot" {
		h.handleBotCommand(hCtx, fields)
		return
	}

	// Try the new command registry first (if initialized)
	if h.commandRegistry != nil {
		// Extract command name (remove slash)
		cmdName := strings.TrimPrefix(cmd, "/")

		// Look up handler via registry
		handler, ok := h.commandRegistry.Match(cmdName)
		if ok {
			// Create command context using imbot types
			cmdCtx := imbot.NewHandlerContext(hCtx.Bot, hCtx.ChatID, hCtx.SenderID, hCtx.Platform).
				WithText(hCtx.Text()).
				WithDirectMessage(hCtx.IsDirect()).
				WithMessageID(hCtx.MessageID)

			// Execute handler
			if err := handler(cmdCtx, fields[1:]); err != nil {
				logrus.WithError(err).WithField("command", cmdName).Error("Command handler failed")
			}
			return
		}
	}

	// Fallback to old switch-based dispatch for backward compatibility
	switch {
	case isCommandMatch(cmd, cmdHelpPrimary, cmdHelpAliases):
		h.handleBotHelpCommand(hCtx)
		return
	case isCommandMatch(cmd, cmdBindPrimary, cmdBindAliases):
		if len(fields) < 2 {
			h.handleBindInteractive(hCtx)
			return
		}
		h.handleBotBindCommand(hCtx, fields[1:])
	case cmd == cmdJoinPrimary:
		if hCtx.IsDirect() {
			h.handleJoinCommand(hCtx, fields)
			return
		} else {
			h.SendText(hCtx, cmdJoinPrimary+" can only be used in general chat.")
			return
		}

	case cmd == cmdProjectPrimary:
		h.handleBotProjectCommand(hCtx)
		return
	case cmd == cmdStatusPrimary:
		h.handleBotStatusCommand(hCtx)
		return
	case cmd == cmdClearPrimary:
		h.handleClearCommand(hCtx)
		return
	case cmd == cmdBashPrimary:
		h.handleBashCommand(hCtx, fields[1:])
		return
	case cmd == cmdYoloPrimary:
		// Yolo mode toggle command
		h.handleYoloCommand(hCtx)
		return
	case cmd == cmdVerbosePrimary:
		// Verbose mode - show all messages
		h.SetVerbose(hCtx.ChatID, true)
		h.SendText(hCtx, "✅ Verbose mode enabled\n\nAll message details will be shown.")
		return
	case cmd == cmdNoVerbosePrimary:
		// NoVerbose mode - hide intermediate messages
		h.SetVerbose(hCtx.ChatID, false)
		h.SendText(hCtx, "🔇 Quiet mode enabled\n\nOnly final results will be shown. Use /verbose to show all details.")
		return
	}

	// All other slash commands go to agent router (defaults to @tb)
	// The agent router will handle the command or route to appropriate agent
	if routeErr := h.routeToAgent(hCtx, input); routeErr != nil {
		logrus.WithError(routeErr).Error("Failed to route command to agent")
	}
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
			h.SendText(hCtx, "✅ Smart Guide (@tb) conversation history cleared.\n\nSend a message to start a new session.")
			logrus.WithField("chatID", hCtx.ChatID).Info("Cleared SmartGuide session")
		} else {
			h.SendText(hCtx, "Smart Guide (@tb) session store is not available.")
		}
		return

	case agentClaudeCode, agentMock:
		// Get project path
		projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		if projectPath == "" {
			// For group chats, also check group binding
			if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
				projectPath = path
			}
		}

		// Use default path if no project bound
		defaultPath := h.getDefaultProjectPath()
		if projectPath == "" {
			projectPath = defaultPath
		}

		// Close the existing session if found
		oldSess := h.sessionMgr.FindBy(hCtx.ChatID, agentType, projectPath)
		if oldSess != nil {
			h.sessionMgr.Close(oldSess.ID)
			agentName := "Claude Code (@cc)"
			if currentAgent == agentMock {
				agentName = "Mock Agent (@mock)"
			}
			h.SendText(hCtx, fmt.Sprintf("✅ %s session cleared.\n\nSend a message to start a new session.\nDefault path: %s", agentName, ShortenPath(defaultPath)))
		} else {
			agentName := "Claude Code (@cc)"
			if currentAgent == agentMock {
				agentName = "Mock Agent (@mock)"
			}
			h.SendText(hCtx, fmt.Sprintf("No active %s session found.\n\nSend a message to start a new session.\nDefault path: %s", agentName, ShortenPath(defaultPath)))
		}
		return

	default:
		h.SendText(hCtx, "Unknown agent type: "+agentType)
	}
}

// handleYoloCommand toggles auto permission mode for the current Claude Code session
// In auto mode, all permission requests are auto-approved for the current session only
func (h *BotHandler) handleYoloCommand(hCtx HandlerContext) {
	// Auto mode only applies to Claude Code agent
	currentAgent, _ := h.getCurrentAgent(hCtx.ChatID)
	if currentAgent != agentClaudeCode {
		h.SendText(hCtx, "⚠️ Auto-approve mode is only available for Claude Code (@cc).\n\nSwitch to Claude Code first with: @cc")
		return
	}

	// Get project path
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

	// Find existing session for Claude Code
	sess := h.sessionMgr.FindBy(hCtx.ChatID, "claude", projectPath)
	if sess == nil {
		// Create a new session if none exists
		sess = h.sessionMgr.CreateWith(hCtx.ChatID, "claude", projectPath)
		h.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{} // Persistent session
		})
	}

	// Toggle permission mode between "auto" and "manual"
	newMode := "auto"
	if sess.PermissionMode == "auto" {
		newMode = "manual"
	}
	h.sessionMgr.Update(sess.ID, func(s *session.Session) {
		s.PermissionMode = newMode
	})

	// Send confirmation message
	if newMode == "auto" {
		h.SendText(hCtx, "🚀 **YOLO MODE ENABLED**\n\nAll permissions will be auto-approved for this session.\n⚠️ Use with caution!\n\nSession: "+sess.ID+"\nProject: "+projectPath)
	} else {
		h.SendText(hCtx, "🔒 **YOLO MODE DISABLED**\n\nBack to normal approval mode.\nAll permission requests will require confirmation.\n\nSession: "+sess.ID+"\nProject: "+projectPath)
	}

	logrus.WithFields(logrus.Fields{
		"chatID":         hCtx.ChatID,
		"sessionID":      sess.ID,
		"projectPath":    projectPath,
		"permissionMode": newMode,
	}).Info("Permission mode toggled")
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
	if hCtx.IsDirect() {
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
	tgKeyboard := imbot.BuildTelegramActionKeyboard(keyboard)

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

// normalizeAllowlistToMap converts a string slice to a map for O(1) lookups
func normalizeAllowlistToMap(values []string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, v := range values {
		normalized := strings.ToLower(strings.TrimSpace(v))
		if normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}
