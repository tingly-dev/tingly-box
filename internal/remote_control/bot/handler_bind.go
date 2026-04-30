package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
)

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
	_, err = feature.SendDirectoryBrowser(h.ctx, hCtx.Bot, h.directoryBrowser, hCtx.ChatID, "")
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

	// Also update bash cwd to match the new project path
	if err := h.chatStore.SetBashCwd(hCtx.ChatID, expandedPath); err != nil {
		logrus.WithError(err).Warn("Failed to update bash cwd after project bind")
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

// reactReceived sends a "received" reaction on the user's message to indicate it is being processed.
// Errors are silently ignored — platforms that don't support reactions degrade gracefully.
