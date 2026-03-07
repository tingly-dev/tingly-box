package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

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
		sessions, err := store.GetRecentSessionsFiltered(ctx, currentProject, 3, filter)
		if err == nil && len(sessions) > 0 {
			for _, s := range sessions {
				allSessions = append(allSessions, sessionWithProject{
					Session:   s,
					Project:   currentProject,
					IsCurrent: true,
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
		isCurrent := ""
		if item.IsCurrent {
			isCurrent = "📍 "
		}

		// Format: [index] Session ID (status)
		status := "✅"
		if sess.Status != "complete" {
			status = "⚠️"
		}

		// Truncate first message
		firstMsg := truncateString(sess.FirstMessage, 50)
		if firstMsg == "" {
			firstMsg = "(empty)"
		}

		// Calculate time ago
		timeAgo := formatTimeAgo(sess.StartTime)

		// Show project name (last component of path)
		projectName := formatProjectName(item.Project)

		msg.WriteString(fmt.Sprintf("%d. %s%s`%s` • %s\n", i+1, isCurrent, status,
			shortSessionID(sess.SessionID), projectName))
		msg.WriteString(fmt.Sprintf("   %s\n", firstMsg))
		msg.WriteString(fmt.Sprintf("   %s • %d turns\n\n", timeAgo, sess.NumTurns))
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

	// Parse selection (expecting a single digit 1-5)
	if len(selection) != 1 || selection[0] < '1' || selection[0] > '5' {
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

	// Set the session for this chat
	if err := h.chatStore.SetSession(hCtx.ChatID, selectedSession.SessionID); err != nil {
		logrus.WithError(err).Error("Failed to set session")
		return false
	}

	// Build confirmation message
	var msg strings.Builder
	msg.WriteString("✅ *Resumed session*\n\n")
	msg.WriteString(fmt.Sprintf("Session: `%s`\n", shortSessionID(selectedSession.SessionID)))

	// Show last user message for context
	if selectedSession.LastUserMessage != "" {
		lastMsg := truncateString(selectedSession.LastUserMessage, 80)
		msg.WriteString(fmt.Sprintf("\nLast message: %s\n", lastMsg))
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
	Session   session.SessionMetadata
	Project   string
	IsCurrent bool
}

// shortSessionID returns a shortened version of the session ID
func shortSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "..."
}

// truncateString truncates a string to max length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Try to truncate at a word boundary
	if maxLen > 3 {
		truncated := s[:maxLen-3]
		if lastSpace := strings.LastIndexAny(truncated, " \t\n"); lastSpace > maxLen/2 {
			return s[:lastSpace] + "..."
		}
	}
	return s[:maxLen-3] + "..."
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
