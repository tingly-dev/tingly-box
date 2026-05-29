package bot

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
)

// cmdJoinPrimary is the join command name (used in HandleMessage for group whitelist message)
const cmdJoinPrimary = "/join"

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
		if clearSession {
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
		h.handleClearCommand(hCtx)
		return
	}

	h.SendText(hCtx, "🛑 Task stopped.")
}

// handleSlashCommands handles slash commands via the registry
func (h *BotHandler) handleSlashCommands(hCtx HandlerContext) {
	input := hCtx.Text()
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return
	}

	cmd := strings.ToLower(fields[0])

	// Handle /bot prefix: re-route as the subcommand
	if cmd == "/bot" && len(fields) >= 2 {
		subcmd := strings.ToLower(strings.TrimSpace(fields[1]))
		// Map /bot subcommands to registry command names
		switch subcmd {
		case "b", "bind":
			cmd = "/cd"
			fields = append([]string{"/cd"}, fields[2:]...)
		default:
			cmd = "/" + subcmd
			fields = append([]string{cmd}, fields[2:]...)
		}
	}

	// Use the command registry
	if h.commandRegistry != nil {
		cmdName := strings.TrimPrefix(cmd, "/")

		// Special handling for help: build dynamic help text
		if cmdName == "" || cmdName == "help" || cmdName == "h" || cmdName == "start" {
			helpText := h.commandRegistry.BuildHelpText(hCtx.IsDirect())

			helpText += "\n\n"
			helpText += "@cc to handoff control to Claude Code\n"
			helpText += "@tb to handoff control to Tingly Box Smart Guide\n"

			helpText += fmt.Sprintf("\nYour ID: %s", hCtx.SenderID)
			formattedHelp := h.formatHelpWithFooter(hCtx, helpText)
			h.SendText(hCtx, formattedHelp)
			return
		}

		handler, ok := h.commandRegistry.Match(cmdName)
		if ok {
			cmdCtx := imbot.NewHandlerContext(hCtx.Bot, hCtx.ChatID, hCtx.SenderID, hCtx.Platform).
				WithText(hCtx.Text()).
				WithDirectMessage(hCtx.IsDirect()).
				WithMessageID(hCtx.MessageID)

			if err := handler(cmdCtx, fields[1:]); err != nil {
				logrus.WithError(err).WithField("command", cmdName).Error("Command handler failed")
			}
			return
		}
	}

	// Unknown slash command: respond with help hint instead of routing to agent
	h.SendText(hCtx, fmt.Sprintf("Unknown command: %s\nUse /help to see available commands.", cmd))
}

// handleClearCommand clears the current session context and creates a new one
func (h *BotHandler) handleClearCommand(hCtx HandlerContext) {
	currentAgent, _ := h.getCurrentAgent(hCtx.ChatID)
	agentType := string(currentAgent)

	switch currentAgent {
	case agentTinglyBox:
		if h.tbSessionStore != nil {
			if err := h.tbSessionStore.Delete(hCtx.ChatID); err != nil {
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
		projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		if projectPath == "" {
			if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
				projectPath = path
			}
		}

		defaultPath := h.getDefaultProjectPath()
		if projectPath == "" {
			projectPath = defaultPath
		}

		oldSess := h.sessionMgr.FindBy(hCtx.ChatID, agentType, projectPath)
		agentName := "Claude Code (@cc)"
		if currentAgent == agentMock {
			agentName = "Mock Agent (@mock)"
		}

		if oldSess != nil {
			h.sessionMgr.Close(oldSess.ID)
			h.SendText(hCtx, fmt.Sprintf("✅ %s session cleared.\n\nSend a message to start a new session.\nDefault path: %s", agentName, ShortenPath(defaultPath)))
		} else {
			h.SendText(hCtx, fmt.Sprintf("No active %s session found.\n\nSend a message to start a new session.\nDefault path: %s", agentName, ShortenPath(defaultPath)))
		}
		return

	default:
		h.SendText(hCtx, "Unknown agent type: "+agentType)
	}
}

// buildProjectText builds the plain-text body for /project replies. The same
// string is used by every platform so the information shown is always identical.
func buildProjectText(currentPath string, projectPaths []string) string {
	var buf strings.Builder
	if currentPath != "" {
		buf.WriteString(fmt.Sprintf("Current Project:\n📁 %s\n\n", currentPath))
	} else {
		buf.WriteString("No project bound to this chat.\n\n")
	}
	if len(projectPaths) > 0 {
		buf.WriteString("Your Projects:\n")
		for i, path := range projectPaths {
			marker := ""
			if path == currentPath {
				marker = " ✓"
			}
			buf.WriteString(fmt.Sprintf("  %d. %s%s\n", i+1, path, marker))
		}
		buf.WriteString("\nUse /cd <number> or /cd <path> to switch.")
	} else {
		buf.WriteString("Use /cd <path> to bind a project.")
	}
	return buf.String()
}

// buildProjectKeyboard builds the inline keyboard for interactive /project replies.
// Button labels include the index so they map 1-to-1 to the numbered text list.
func buildProjectKeyboard(currentPath string, projectPaths []string) imbot.InlineKeyboardMarkup {
	var rows [][]imbot.InlineKeyboardButton
	for i, path := range projectPaths {
		marker := ""
		if path == currentPath {
			marker = " ✓"
		}
		rows = append(rows, []imbot.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%d. 📁 %s%s", i+1, filepath.Base(path), marker),
			CallbackData: imbot.FormatCallbackData("project", "switch", path),
		}})
	}
	rows = append(rows, []imbot.InlineKeyboardButton{{
		Text:         "📁 Bind New Project",
		CallbackData: imbot.FormatCallbackData("action", "bind"),
	}})
	return imbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// handleBotProjectCommand handles the inline-keyboard action-menu "project" button.
// Always runs from a Telegram callback, so always sends with a keyboard.
func (h *BotHandler) handleBotProjectCommand(hCtx HandlerContext) {
	if h.chatStore == nil {
		h.SendText(hCtx, "Store not available")
		return
	}

	currentPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	projectPaths, _ := h.chatStore.ListChatProjectPaths(hCtx.ChatID)

	text := buildProjectText(currentPath, projectPaths)
	keyboard := buildProjectKeyboard(currentPath, projectPaths)
	tgKeyboard := imbot.BuildTelegramActionKeyboard(keyboard)

	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:     text,
		Metadata: buildTrackedReplyMetadata(tgKeyboard),
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send project list")
	}
}

// formatHelpWithFooter formats help text with the standard reply footer
// (agent + project path) appended at the bottom.
func (h *BotHandler) formatHelpWithFooter(hCtx HandlerContext, helpText string) string {
	projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	if projectPath == "" {
		if path, found := getProjectPathForGroup(h.chatStore, hCtx.ChatID, string(hCtx.Platform)); found {
			projectPath = path
		}
	}
	currentAgent, _ := h.getCurrentAgent(hCtx.ChatID)

	meta := ResponseMeta{
		ProjectPath: projectPath,
		AgentType:   string(currentAgent),
	}

	return h.formatResponseWithFooter(meta, helpText)
}

// handleResumePick processes a tap on a /resume keyboard button. Mirrors the
// `/resume <n>` text path: validate agent + project, arm the picked session,
// and acknowledge in chat. We resolve project/agent via the chat itself
// (rather than trusting callback payloads) so a stale keyboard cannot be used
// to act on a different project than the one currently bound.
func (h *BotHandler) handleResumePick(hCtx HandlerContext, sessionID string, msg imbot.Message) {
	if h.commandAdapter == nil {
		h.SendText(hCtx, "Command adapter not initialized.")
		return
	}
	agentType, _ := h.commandAdapter.GetCurrentAgent(hCtx.ChatID)
	if agentType != AgentNameClaude {
		h.SendText(hCtx, "⚠️ /resume only works with Claude Code (@cc). Switch with: @cc")
		return
	}
	projectPath := resolveProjectPath(h.commandAdapter, hCtx.ChatID, string(hCtx.Platform))
	if projectPath == "" {
		h.SendText(hCtx, "No project bound. Use /cd <path> first.")
		return
	}
	if err := h.commandAdapter.PrepareResume(hCtx.ChatID, agentType, projectPath, sessionID); err != nil {
		h.SendText(hCtx, fmt.Sprintf("Failed to arm resume: %v", err))
		return
	}

	// Strip the keyboard from the original listing message so the user can't
	// double-tap into a stale state. Best-effort; ignore failures.
	if msgID, _ := msg.Metadata["message_id"].(string); msgID != "" {
		if tgBot, ok := imbot.AsTelegramBot(hCtx.Bot); ok {
			if err := tgBot.RemoveMessageKeyboard(context.Background(), hCtx.ChatID, msgID); err != nil {
				logrus.WithError(err).Debug("Failed to remove resume keyboard")
			}
		}
	}

	h.SendText(hCtx, fmt.Sprintf(
		"✅ Armed resume for session %s.\nSend your next message to continue, or /clear to abort.",
		shortSessionID(sessionID)))
}

// handleResumeCancel removes the resume keyboard and confirms cancellation.
// No state to clean up — armed state only flips on `pick`.
func (h *BotHandler) handleResumeCancel(hCtx HandlerContext, msg imbot.Message) {
	if msgID, _ := msg.Metadata["message_id"].(string); msgID != "" {
		if tgBot, ok := imbot.AsTelegramBot(hCtx.Bot); ok {
			_ = tgBot.RemoveMessageKeyboard(context.Background(), hCtx.ChatID, msgID)
		}
	}
	h.SendText(hCtx, "Resume cancelled.")
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
