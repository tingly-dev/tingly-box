package bot

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot/session"
	"github.com/tingly-dev/tingly-box/agentboot/session/claude"
)

// handleResumeCommand handles the /resume command to list and select recent sessions
// Shows sessions from all projects the chat has used before
func (h *BotHandler) handleResumeCommand(hCtx HandlerContext) {
	// Create session store
	store, err := claude.NewStore("")
	if err != nil {
		h.SendText(hCtx, "Failed to access session store.")
		logrus.WithError(err).Warn("Failed to create session store")
		return
	}

	// Get current chat's project path (for prioritization)
	chat, err := h.chatStore.GetChat(hCtx.ChatID)
	currentProject := ""
	if err == nil && chat != nil {
		currentProject = chat.ProjectPath
	}

	// Get all project paths that have sessions
	// For simplicity, we'll show the current project's sessions first, then list others
	ctx := context.Background()
	filter := claude.DefaultSessionFilter()

	// Collect sessions from current project first (if set)
	var allSessions []sessionWithProject
	if currentProject != "" {
		sessions, err := store.GetRecentSessionsFiltered(ctx, currentProject, 5, filter)
		if err == nil && len(sessions) > 0 {
			for _, s := range sessions {
				allSessions = append(allSessions, sessionWithProject{
					Session: s,
					Project: currentProject,
				})
			}
		}
	}

	// Get a few more from other projects (limit to avoid too much output)
	// In a real implementation, you might want to track which projects this chat has used
	// For now, we'll just show current project sessions
	if len(allSessions) == 0 {
		h.SendText(hCtx, "No recent sessions found. use /cd to connect to a project first.")
		return
	}

	// Build message with session list
	var msg strings.Builder
	msg.WriteString("📜 *Recent Sessions*\n\n")

	for i, item := range allSessions {
		sess := item.Session

		// Truncate first message
		firstMsg := truncateString(sess.FirstMessage, 60)
		if firstMsg == "" {
			firstMsg = "(empty)"
		}

		// Calculate time ago
		timeAgo := formatTimeAgo(sess.StartTime)

		// Show project name (last component of path)
		projectName := formatProjectName(item.Project)

		msg.WriteString(fmt.Sprintf("%d. %s • %s\n", i+1, projectName, timeAgo))
		msg.WriteString(fmt.Sprintf("   %s\n", firstMsg))

		// Show last message (user or assistant) if different from first
		lastMsg := sess.LastUserMessage
		if lastMsg == "" {
			lastMsg = sess.LastAssistantMessage
		}
		if lastMsg != "" && lastMsg != sess.FirstMessage {
			lastMsg = truncateString(lastMsg, 60)
			msg.WriteString(fmt.Sprintf("   ─ %s\n", lastMsg))
		}

		msg.WriteString("\n")
	}

	msg.WriteString("Reply with the number to resume a session.")

	h.SendText(hCtx, msg.String())
}

// handleResumeSelection handles the user's selection from the resume list
// Returns true if the message was a valid selection and was handled
func (h *BotHandler) handleResumeSelection(hCtx HandlerContext, selectionStr string) bool {
	// Validate selection
	selection := strings.TrimSpace(selectionStr)
	if selection == "" {
		return false
	}

	// Parse selection (expecting a single digit 1-9)
	if len(selection) != 1 || selection[0] < '1' || selection[0] > '9' {
		return false
	}

	index := int(selection[0] - '0')

	// Get current chat's project path
	chat, err := h.chatStore.GetChat(hCtx.ChatID)
	if err != nil || chat == nil || chat.ProjectPath == "" {
		return false
	}

	// Get recent sessions from current project
	store, _ := claude.NewStore("")
	ctx := context.Background()
	filter := claude.DefaultSessionFilter()
	sessions, err := store.GetRecentSessionsFiltered(ctx, chat.ProjectPath, 5, filter)
	if err != nil || len(sessions) == 0 {
		return false
	}

	// Validate index
	if index < 1 || index > len(sessions) {
		return false
	}

	selectedSession := sessions[index-1]

	// Set the session and project for this chat
	if err := h.chatStore.SetSessionWithProject(hCtx.ChatID, selectedSession.SessionID, chat.ProjectPath); err != nil {
		logrus.WithError(err).Error("Failed to set session")
		return false
	}

	// Build confirmation message
	var msg strings.Builder
	msg.WriteString("✅ *Resumed session*\n\n")

	// Show last message for context (prefer user, fallback to assistant)
	lastMsg := selectedSession.LastAssistantMessage
	if lastMsg == "" {
		lastMsg = selectedSession.LastUserMessage
	}
	if lastMsg != "" {
		lastMsg = truncateString(lastMsg, 80)
		msg.WriteString(fmt.Sprintf("Last message: %s\n", lastMsg))
	}

	msg.WriteString("\nYou can continue your conversation.")
	msg.WriteString("\n\nUse /clear to start a new session.")

	h.SendText(hCtx, msg.String())

	logrus.WithFields(logrus.Fields{
		"chat_id":    hCtx.ChatID,
		"session_id": selectedSession.SessionID,
		"project":    chat.ProjectPath,
	}).Info("Session resumed")

	return true
}

// sessionWithProject wraps a session with its project context
type sessionWithProject struct {
	Session session.SessionMetadata
	Project string
}

// truncateString truncates a string to max length (in runes, not bytes) with ellipsis
func truncateString(s string, maxLen int) string {
	// Count runes (UTF-8 characters) instead of bytes
	runeCount := utf8.RuneCountInString(s)
	if runeCount <= maxLen {
		return s
	}
	// Truncate at rune boundary
	runes := []rune(s)
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	// Try to truncate at a word boundary
	truncated := string(runes[:maxLen-3])
	if lastSpace := strings.LastIndexAny(truncated, " \t\n"); lastSpace > maxLen/2 {
		return s[:lastSpace] + "..."
	}
	return truncated + "..."
}

// formatTimeAgo returns a human-readable time ago string
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}

	duration := time.Since(t)
	if duration < time.Minute {
		return "just now"
	}
	if duration < time.Hour {
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	}
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	}
	if duration < 30*24*time.Hour {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}

	return t.Format("Jan 2")
}

// formatProjectName extracts a short name from the project path
func formatProjectName(path string) string {
	if path == "" {
		return "unknown"
	}
	// Get the last component of the path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		if len(name) > 15 {
			return name[:12] + "..."
		}
		return name
	}
	return path
}
