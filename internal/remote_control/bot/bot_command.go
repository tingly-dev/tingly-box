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
			formattedHelp := h.formatHelpWithHeader(hCtx, helpText)
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

// handleBotProjectCommand handles /project - shows current project and list with keyboard.
// Kept as a direct BotHandler method since it needs inline keyboard + directory browser integration.
func (h *BotHandler) handleBotProjectCommand(hCtx HandlerContext) {
	if h.chatStore == nil {
		h.SendText(hCtx, "Store not available")
		return
	}

	platform := string(hCtx.Platform)
	currentPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)

	var buf strings.Builder
	if currentPath != "" {
		buf.WriteString(fmt.Sprintf("Current Project:\n📁 %s\n\n", currentPath))
	} else {
		buf.WriteString("No project bound to this chat.\n\n")
	}

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

// formatHelpWithHeader formats help text with meta information
func (h *BotHandler) formatHelpWithHeader(hCtx HandlerContext, helpText string) string {
	projectPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	currentAgent, _ := h.getCurrentAgent(hCtx.ChatID)

	meta := ResponseMeta{
		ProjectPath: projectPath,
		AgentType:   string(currentAgent),
		ChatID:      hCtx.ChatID,
		UserID:      hCtx.SenderID,
		SessionID:   hCtx.ChatID,
	}

	return h.formatResponseWithHeader(meta, helpText, true)
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
