package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
)

// HandlerContextV2 contains per-message context data for v2 handlers
type HandlerContextV2 struct {
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

// NewHandlerContextV2 creates a new handler context from a message
func NewHandlerContextV2(msg imbot.Message, platform imbot.Platform, botUUID string) HandlerContextV2 {
	mediaAttachments := msg.GetMedia()
	return HandlerContextV2{
		Bot:       nil, // Will be set by caller
		BotUUID:   botUUID,
		ChatID:    getReplyTarget(msg),
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Platform:  platform,
		IsDirect:  msg.IsDirectMessage(),
		IsGroup:   msg.IsGroupMessage(),
		Text:      strings.TrimSpace(msg.GetText()),
		Media:     mediaAttachments,
		Metadata:  msg.Metadata,
	}
}

// HandleCallbackQueryV2 handles callback queries using the new interaction system
// This replaces the old handleCallbackQuery with platform-agnostic interaction handling
func (h *BotHandler) HandleCallbackQueryV2(hCtx HandlerContextV2) error {
	callbackData, _ := hCtx.Metadata["callback_data"].(string)
	if callbackData == "" {
		return fmt.Errorf("no callback data")
	}

	// Check if this is a new interaction callback (ia: prefix)
	if strings.HasPrefix(callbackData, "ia:") {
		// Parse interaction callback
		// Format: ia:requestID:interactionID:value
		parts := strings.Split(callbackData, ":")
		if len(parts) < 3 {
			return fmt.Errorf("invalid interaction callback format")
		}

		requestID := parts[1]
		interactionID := parts[2]
		value := ""
		if len(parts) >= 4 {
			value = strings.Join(parts[3:], ":")
		}

		// Get pending request
		_, ok := h.interaction.GetPendingRequest(requestID)
		if !ok {
			return fmt.Errorf("interaction request not found or expired")
		}

		// Build interaction response
		resp := &imbot.InteractionResponse{
			RequestID: requestID,
			Action: imbot.Interaction{
				ID:    interactionID,
				Value: value,
			},
			Timestamp: time.Now(),
		}

		// Submit response
		if err := h.interaction.SubmitResponse(requestID, *resp); err != nil {
			return fmt.Errorf("failed to submit interaction response: %w", err)
		}

		// Handle the interaction based on request type
		return h.handleInteractionResponseV2(hCtx, resp)
	}

	// Legacy callback handling (for backward compatibility during migration)
	return h.handleLegacyCallbackV2(hCtx, callbackData)
}

// handleInteractionResponseV2 handles responses from the new interaction system
func (h *BotHandler) handleInteractionResponseV2(hCtx HandlerContextV2, resp *imbot.InteractionResponse) error {
	requestID := resp.RequestID
	actionID := resp.Action.ID
	value := resp.Action.Value

	logrus.WithFields(logrus.Fields{
		"request_id": requestID,
		"action_id":  actionID,
		"value":      value,
		"chat_id":    hCtx.ChatID,
	}).Debug("Handling interaction response")

	// Check request type from ID prefix
	if strings.HasPrefix(requestID, "bind-browser-") {
		return h.handleBindBrowserInteractionV2(hCtx, actionID, value)
	}

	if strings.HasPrefix(requestID, "bind-confirm-") {
		return h.handleBindConfirmInteractionV2(hCtx, actionID)
	}

	if strings.HasPrefix(requestID, "create-confirm-") {
		return h.handleCreateConfirmInteractionV2(hCtx, actionID, value)
	}

	if strings.HasPrefix(requestID, "action-menu-") {
		return h.handleActionMenuInteractionV2(hCtx, actionID)
	}

	if strings.HasPrefix(requestID, "custom-path-") {
		return h.handleCustomPathInteractionV2(hCtx, actionID)
	}

	if strings.HasPrefix(requestID, "project-select-") {
		return h.handleProjectSelectInteractionV2(hCtx, actionID, value)
	}

	return fmt.Errorf("unknown interaction request type: %s", requestID)
}

// handleBindBrowserInteractionV2 handles directory browser interactions
func (h *BotHandler) handleBindBrowserInteractionV2(hCtx HandlerContextV2, actionID, value string) error {
	switch actionID {
	case "nav-up":
		if err := h.directoryBrowserV2.NavigateUp(hCtx.ChatID); err != nil {
			return err
		}
		_, _ = SendDirectoryBrowserV2(h.ctx, h, h.directoryBrowserV2, hCtx.ChatID, hCtx.Platform, hCtx.BotUUID, hCtx.MessageID)
		return nil

	case "nav-prev":
		if err := h.directoryBrowserV2.PrevPage(hCtx.ChatID); err != nil {
			return err
		}
		_, _ = SendDirectoryBrowserV2(h.ctx, h, h.directoryBrowserV2, hCtx.ChatID, hCtx.Platform, hCtx.BotUUID, hCtx.MessageID)
		return nil

	case "nav-next":
		if err := h.directoryBrowserV2.NextPage(hCtx.ChatID); err != nil {
			return err
		}
		_, _ = SendDirectoryBrowserV2(h.ctx, h, h.directoryBrowserV2, hCtx.ChatID, hCtx.Platform, hCtx.BotUUID, hCtx.MessageID)
		return nil

	case "select":
		currentPath := h.directoryBrowserV2.GetCurrentPath(hCtx.ChatID)
		if currentPath == "" {
			return fmt.Errorf("no current path in bind flow")
		}
		h.completeBind(hCtx.toV1(), currentPath)
		h.directoryBrowserV2.Clear(hCtx.ChatID)
		return nil

	case "custom":
		h.handleCustomPathPromptV2(hCtx)
		return nil

	case "cancel":
		h.directoryBrowserV2.Clear(hCtx.ChatID)
		h.SendText(hCtx.toV1(), "Bind cancelled.")
		return nil

	default:
		// Check if it's a directory navigation (dir- prefix)
		if strings.HasPrefix(actionID, "dir-") {
			// Extract index from value (format: dir:123)
			indexStr := strings.TrimPrefix(value, "dir:")
			var index int
			if _, err := fmt.Sscanf(indexStr, "%d", &index); err == nil {
				if err := h.directoryBrowserV2.NavigateByIndex(hCtx.ChatID, index); err != nil {
					logrus.WithError(err).Warn("Failed to navigate directory")
					return err
				}
				_, _ = SendDirectoryBrowserV2(h.ctx, h, h.directoryBrowserV2, hCtx.ChatID, hCtx.Platform, hCtx.BotUUID, hCtx.MessageID)
				return nil
			}
		}
	}

	return fmt.Errorf("unknown bind browser action: %s", actionID)
}

// handleBindConfirmInteractionV2 handles bind confirmation interactions
func (h *BotHandler) handleBindConfirmInteractionV2(hCtx HandlerContextV2, actionID string) error {
	switch actionID {
	case "confirm":
		h.handleBindConfirm(hCtx.toV1())
		return nil
	case "custom":
		h.handleCustomPathPromptV2(hCtx)
		return nil
	case "cancel":
		h.SendText(hCtx.toV1(), "Bind cancelled.")
		return nil
	}
	return fmt.Errorf("unknown bind confirm action: %s", actionID)
}

// handleCreateConfirmInteractionV2 handles directory creation confirmation interactions
func (h *BotHandler) handleCreateConfirmInteractionV2(hCtx HandlerContextV2, actionID, value string) error {
	switch actionID {
	case "create":
		// Extract path from value (format: create:/path/to/dir)
		path := strings.TrimPrefix(value, "create:")
		if err := os.MkdirAll(path, 0755); err != nil {
			h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to create directory: %v", err))
			return err
		}
		h.completeBind(hCtx.toV1(), path)
		h.directoryBrowserV2.Clear(hCtx.ChatID)
		return nil
	case "cancel":
		h.SendText(hCtx.toV1(), "Cancelled.")
		return nil
	}
	return fmt.Errorf("unknown create confirm action: %s", actionID)
}

// handleActionMenuInteractionV2 handles action menu interactions
func (h *BotHandler) handleActionMenuInteractionV2(hCtx HandlerContextV2, actionID string) error {
	switch actionID {
	case "clear":
		h.handleClearCommand(hCtx.toV1())
		return nil
	case "bind":
		h.handleBindInteractiveV2(hCtx)
		return nil
	case "project":
		h.handleBotProjectCommandV2(hCtx)
		return nil
	}
	return fmt.Errorf("unknown action menu action: %s", actionID)
}

// handleCustomPathInteractionV2 handles custom path input interactions
func (h *BotHandler) handleCustomPathInteractionV2(hCtx HandlerContextV2, actionID string) error {
	if actionID == "cancel" {
		h.directoryBrowserV2.Clear(hCtx.ChatID)
		h.SendText(hCtx.toV1(), "Cancelled.")
		return nil
	}
	return fmt.Errorf("unknown custom path action: %s", actionID)
}

// handleProjectSelectInteractionV2 handles project selection interactions
func (h *BotHandler) handleProjectSelectInteractionV2(hCtx HandlerContextV2, actionID, value string) error {
	// Extract project ID from value
	projectID := strings.TrimPrefix(value, "project:")
	h.handleProjectSwitch(hCtx.toV1(), projectID)
	return nil
}

// handleLegacyCallbackV2 handles legacy callbacks during migration period
// TODO: Remove once all clients are migrated to v2
func (h *BotHandler) handleLegacyCallbackV2(hCtx HandlerContextV2, callbackData string) error {
	// Delegate to old handler for backward compatibility
	parts := imbot.ParseCallbackData(callbackData)
	if len(parts) == 0 {
		return fmt.Errorf("invalid callback data")
	}

	action := parts[0]
	hCtxV1 := hCtx.toV1()

	switch action {
	case "perm":
		h.handlePermissionCallback(hCtxV1, parts)
	case "action":
		if len(parts) < 2 {
			return nil
		}
		switch parts[1] {
		case "clear":
			h.handleClearCommand(hCtxV1)
		case "bind":
			h.handleBindInteractive(hCtxV1)
		case "project":
			h.handleBotProjectCommand(hCtxV1)
		}
	case "project":
		if len(parts) < 3 {
			return nil
		}
		if parts[1] == "switch" {
			h.handleProjectSwitch(hCtxV1, parts[2])
		}
	case "bind":
		// Handle bind callbacks
		h.handleBindCallbackV2(hCtxV1, parts)
	}

	return nil
}

// handleBindCallbackV2 handles legacy bind callbacks
func (h *BotHandler) handleBindCallbackV2(hCtx HandlerContext, parts []string) {
	if len(parts) < 2 {
		return
	}

	subAction := parts[1]
	chatID := hCtx.ChatID

	switch subAction {
	case "confirm":
		h.handleBindConfirm(hCtx)
	case "dir":
		if len(parts) < 3 {
			return
		}
		indexStr := parts[2]
		var index int
		if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
			return
		}
		// Use old directory browser for legacy callbacks
		if err := h.directoryBrowser.NavigateByIndex(chatID, index); err != nil {
			return
		}
		msgID, _ := hCtx.Metadata["message_id"].(string)
		if msgID == "" {
			msgID = hCtx.MessageID
		}
		_, _ = SendDirectoryBrowser(h.ctx, hCtx.Bot, h.directoryBrowser, chatID, msgID)
	case "up":
		if err := h.directoryBrowser.NavigateUp(chatID); err != nil {
			return
		}
		msgID, _ := hCtx.Metadata["message_id"].(string)
		if msgID == "" {
			msgID = hCtx.MessageID
		}
		_, _ = SendDirectoryBrowser(h.ctx, hCtx.Bot, h.directoryBrowser, chatID, msgID)
	case "prev":
		if err := h.directoryBrowser.PrevPage(chatID); err != nil {
			return
		}
		msgID, _ := hCtx.Metadata["message_id"].(string)
		if msgID == "" {
			msgID = hCtx.MessageID
		}
		_, _ = SendDirectoryBrowser(h.ctx, hCtx.Bot, h.directoryBrowser, chatID, msgID)
	case "next":
		if err := h.directoryBrowser.NextPage(chatID); err != nil {
			return
		}
		msgID, _ := hCtx.Metadata["message_id"].(string)
		if msgID == "" {
			msgID = hCtx.MessageID
		}
		_, _ = SendDirectoryBrowser(h.ctx, hCtx.Bot, h.directoryBrowser, chatID, msgID)
	case "select":
		currentPath := h.directoryBrowser.GetCurrentPath(chatID)
		if currentPath == "" {
			return
		}
		h.completeBind(hCtx, currentPath)
		h.directoryBrowser.Clear(chatID)
	case "custom":
		h.handleCustomPathPrompt(hCtx)
	case "create":
		if len(parts) < 3 {
			return
		}
		encodedPath := parts[2]
		path := imbot.ParseDirPath(encodedPath)
		if err := os.MkdirAll(path, 0755); err != nil {
			h.SendText(hCtx, fmt.Sprintf("Failed to create directory: %v", err))
			return
		}
		h.completeBind(hCtx, path)
		h.directoryBrowser.Clear(chatID)
	case "cancel":
		h.directoryBrowser.Clear(chatID)
		h.SendText(hCtx, "Bind cancelled.")
	}
}

// V2 handler methods using the new interaction system

// handleBindInteractiveV2 starts the interactive bind flow using the new interaction system
func (h *BotHandler) handleBindInteractiveV2(hCtx HandlerContextV2) {
	// Ensure state exists
	state := h.directoryBrowserV2.GetState(hCtx.ChatID)
	if state == nil {
		var err error
		state, err = h.directoryBrowserV2.Start(hCtx.ChatID)
		if err != nil {
			h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to start bind flow: %v", err))
			return
		}
	}

	// Send directory browser
	_, err := SendDirectoryBrowserV2(h.ctx, h, h.directoryBrowserV2, hCtx.ChatID, hCtx.Platform, hCtx.BotUUID, "")
	if err != nil {
		h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to send directory browser: %v", err))
	}
}

// handleBotProjectCommandV2 shows project selection using the new interaction system
func (h *BotHandler) handleBotProjectCommandV2(hCtx HandlerContextV2) {
	chatID := hCtx.ChatID
	platform := string(hCtx.Platform)

	// Get current project path for this chat
	currentPath, _, _ := h.chatStore.GetProjectPath(chatID)

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

	if len(projectPaths) == 0 {
		// No projects, show bind confirmation
		h.showBindConfirmationPromptV2(hCtx, "")
		return
	}

	// Build interactions for project list
	builder := imbot.NewInteractionBuilder()

	for _, path := range projectPaths {
		marker := ""
		if path == currentPath {
			marker = " ✓"
		}
		buttonText := fmt.Sprintf("📁 %s%s", filepath.Base(path), marker)
		callbackData := fmt.Sprintf("project:%s", path)
		builder.AddButton("project-"+path, buttonText, callbackData)
	}

	// Add "Bind New" button
	builder.AddButton("bind-new", "📁 Bind New Project", "bind-new")
	builder.AddCancel("cancel")

	bot := h.manager.GetBot(hCtx.BotUUID, hCtx.Platform)
	if bot == nil {
		h.SendText(hCtx.toV1(), "Bot not available")
		return
	}

	_, err := bot.SendMessage(h.ctx, chatID, &imbot.SendMessageOptions{
		Text:      "📁 *Your Projects:*\n\nSelect a project to switch to:",
		ParseMode: imbot.ParseModeMarkdown,
	})
	if err != nil {
		h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to send project list: %v", err))
	}
}

// showBindConfirmationPromptV2 shows bind confirmation using the new interaction system
func (h *BotHandler) showBindConfirmationPromptV2(hCtx HandlerContextV2, originalMessage string) {
	cwd, err := os.Getwd()
	if err != nil {
		h.SendText(hCtx.toV1(), "Failed to get current directory")
		return
	}

	interactions := BuildBindConfirmInteractionsV2()
	text := BuildBindConfirmPromptV2(cwd)

	// Create interaction request
	requestID := fmt.Sprintf("bind-confirm-%s-%d", hCtx.ChatID, time.Now().UnixNano())
	req := imbot.InteractionRequest{
		ID:        requestID,
		ChatID:    hCtx.ChatID,
		Platform:  hCtx.Platform,
		BotUUID:   hCtx.BotUUID,
		Message:   text,
		ParseMode: imbot.ParseModeMarkdown,
		Mode:      imbot.ModeAuto,
		Interactions: interactions,
		Timeout:   5 * time.Minute,
	}

	// Send interaction
	bot := h.manager.GetBot(hCtx.BotUUID, hCtx.Platform)
	if bot == nil {
		h.SendText(hCtx.toV1(), "Bot not available")
		return
	}

	_, err = bot.SendMessage(h.ctx, hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      req.Message,
		ParseMode: req.ParseMode,
	})
	if err != nil {
		h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to send bind confirmation: %v", err))
	}
}

// handleCustomPathPromptV2 sends the custom path input prompt using the new interaction system
func (h *BotHandler) handleCustomPathPromptV2(hCtx HandlerContextV2) {
	state := h.directoryBrowserV2.GetState(hCtx.ChatID)
	if state == nil {
		var err error
		state, err = h.directoryBrowserV2.Start(hCtx.ChatID)
		if err != nil {
			h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to start bind flow: %v", err))
			return
		}
	}

	h.directoryBrowserV2.SetWaitingInput(hCtx.ChatID, true, "")

	interactions := BuildCancelInteractionsV2()
	text := BuildCustomPathPromptV2()

	// Create interaction request
	requestID := fmt.Sprintf("custom-path-%s-%d", hCtx.ChatID, time.Now().UnixNano())
	req := imbot.InteractionRequest{
		ID:        requestID,
		ChatID:    hCtx.ChatID,
		Platform:  hCtx.Platform,
		BotUUID:   hCtx.BotUUID,
		Message:   text,
		ParseMode: imbot.ParseModeMarkdown,
		Mode:      imbot.ModeAuto,
		Interactions: interactions,
		Timeout:   5 * time.Minute,
	}

	bot := h.manager.GetBot(hCtx.BotUUID, hCtx.Platform)
	if bot == nil {
		h.SendText(hCtx.toV1(), "Bot not available")
		return
	}

	result, err := bot.SendMessage(h.ctx, hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      req.Message,
		ParseMode: req.ParseMode,
	})
	if err != nil {
		h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to send custom path prompt: %v", err))
		return
	}

	h.directoryBrowserV2.SetWaitingInput(hCtx.ChatID, true, result.MessageID)
}

// handleCustomPathInputV2 handles the user's custom path input
func (h *BotHandler) handleCustomPathInputV2(hCtx HandlerContextV2) {
	state := h.directoryBrowserV2.GetState(hCtx.ChatID)
	currentPath := ""
	if state != nil {
		currentPath = state.CurrentPath
	}

	var expandedPath string
	var err error

	if filepath.IsAbs(hCtx.Text) || strings.HasPrefix(hCtx.Text, "~") {
		expandedPath, err = ExpandPathV2(hCtx.Text)
		if err != nil {
			h.SendText(hCtx.toV1(), fmt.Sprintf("Invalid path: %v", err))
			return
		}
	} else if currentPath != "" {
		expandedPath = filepath.Join(currentPath, hCtx.Text)
	} else {
		expandedPath, err = ExpandPathV2(hCtx.Text)
		if err != nil {
			h.SendText(hCtx.toV1(), fmt.Sprintf("Invalid path: %v", err))
			return
		}
	}

	expandedPath = filepath.Clean(expandedPath)

	info, err := os.Stat(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			h.handleCreateConfirmV2(hCtx, expandedPath)
			return
		}
		h.SendText(hCtx.toV1(), fmt.Sprintf("Cannot access path: %v", err))
		return
	}

	if !info.IsDir() {
		h.SendText(hCtx.toV1(), "The path is not a directory.")
		return
	}

	h.completeBind(hCtx.toV1(), expandedPath)
	h.directoryBrowserV2.Clear(hCtx.ChatID)
}

// handleCreateConfirmV2 sends directory creation confirmation using the new interaction system
func (h *BotHandler) handleCreateConfirmV2(hCtx HandlerContextV2, path string) {
	interactions, text := BuildCreateConfirmInteractionsV2(path)

	// Create interaction request
	requestID := fmt.Sprintf("create-confirm-%s-%d", hCtx.ChatID, time.Now().UnixNano())
	req := imbot.InteractionRequest{
		ID:        requestID,
		ChatID:    hCtx.ChatID,
		Platform:  hCtx.Platform,
		BotUUID:   hCtx.BotUUID,
		Message:   text,
		ParseMode: imbot.ParseModeMarkdown,
		Mode:      imbot.ModeAuto,
		Interactions: interactions,
		Timeout:   5 * time.Minute,
	}

	bot := h.manager.GetBot(hCtx.BotUUID, hCtx.Platform)
	if bot == nil {
		h.SendText(hCtx.toV1(), "Bot not available")
		return
	}

	_, err := bot.SendMessage(h.ctx, hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      req.Message,
		ParseMode: req.ParseMode,
	})
	if err != nil {
		h.SendText(hCtx.toV1(), fmt.Sprintf("Failed to send create confirmation: %v", err))
	}
}

// Helper methods for v2

// toV1 converts HandlerContextV2 to HandlerContext for compatibility with v1 methods
func (hCtx HandlerContextV2) toV1() HandlerContext {
	return HandlerContext{
		Bot:       hCtx.Bot,
		BotUUID:   hCtx.BotUUID,
		ChatID:    hCtx.ChatID,
		SenderID:  hCtx.SenderID,
		MessageID: hCtx.MessageID,
		Platform:  hCtx.Platform,
		IsDirect:  hCtx.IsDirect,
		IsGroup:   hCtx.IsGroup,
		Text:      hCtx.Text,
		Media:     hCtx.Media,
		Metadata:  hCtx.Metadata,
	}
}
