package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
)

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
		// Pairing gate: when RequirePairing is enabled, only paired chats may
		// reach the regular handlers. Unpaired chats are limited to /bind.
		if h.botSetting.IsRequirePairing() && !h.chatStore.IsChatPaired(chatID, botUUID) {
			text := strings.TrimSpace(msg.GetText())
			if !isBindCommand(text) {
				h.auditWarn("imbot.pair.unpaired_message", msg.Sender.ID,
					"rejected unpaired direct message", map[string]interface{}{
						"bot_uuid": botUUID,
						"chat_id":  chatID,
						"platform": string(platform),
					})
				h.SendText(hCtx, pairingHintMessage())
				return
			}
			// fall through; the /bind handler verifies the code
		}
	case msg.IsGroupMessage():
		logrus.Infof("Group chat ID: %s", chatID)
		// Check whitelist first
		if !h.chatStore.IsWhitelisted(chatID) {
			logrus.Debugf("Group %s is not whitelisted, ignoring message", chatID)
			h.SendText(hCtx, fmt.Sprintf("This group is not enabled. Please DM the bot with `%s %s` to enable.", cmdJoinPrimary, chatID))
			return
		}
		// When pairing is required, the operator who whitelisted the group
		// must themselves be paired in DM.
		if h.botSetting.IsRequirePairing() && !h.isWhitelisterPaired(chatID, botUUID) {
			h.auditWarn("imbot.pair.unpaired_message", msg.Sender.ID,
				"rejected group whitelisted by unpaired operator",
				map[string]interface{}{
					"bot_uuid": botUUID,
					"chat_id":  chatID,
					"platform": string(platform),
				})
			h.SendText(hCtx, "🔒 This group's operator has not paired with the bot. Ask them to send /bind <code> in a DM first.")
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
			h.reactReceived(hCtx)
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

	// React to indicate the message is being processed (after stop check, before all other handling)
	h.reactReceived(hCtx)

	// Handoff commands take precedence over the slash dispatcher: /cc, /tb
	// look like slash commands but they're really handoff sugar. Without
	// this check they'd fall to handleSlashCommands → "Unknown command"
	// since the registry doesn't (and shouldn't) own them.
	if _, isHandoff, _ := smart_guide.DetectHandoffCommand(hCtx.Text()); isHandoff {
		if routeErr := h.routeToAgent(hCtx, hCtx.Text()); routeErr != nil {
			logrus.WithError(routeErr).Error("Failed to route handoff command")
			errMsg := fmt.Sprintf("⚠️ **Error**: %v", routeErr)
			if strings.Contains(routeErr.Error(), "already in progress") || strings.Contains(routeErr.Error(), "already in use") {
				errMsg = fmt.Sprintf("⚠️ **Session Busy**\n\nAnother execution is already in progress for this chat.\n\nPlease:\n• Wait for the current task to complete\n• Use `/stop` to cancel the current execution\n\nTechnical details: %v", routeErr)
			}
			h.SendText(hCtx, errMsg)
		}
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
		// Send error message to user
		errMsg := fmt.Sprintf("⚠️ **Error**: %v", routeErr)
		if strings.Contains(routeErr.Error(), "already in progress") || strings.Contains(routeErr.Error(), "already in use") {
			errMsg = fmt.Sprintf("⚠️ **Session Busy**\n\nAnother execution is already in progress for this chat.\n\nPlease:\n• Wait for the current task to complete\n• Use `/stop` to cancel the current execution\n\nTechnical details: %v", routeErr)
		}
		h.SendText(hCtx, errMsg)
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

// handlePermissionTextResponse handles text-based permission responses.
// Returns true if the message was a valid permission response, false otherwise.
// Only reachable in standalone (host-less) mode: the managed path's host
// router claims prompt replies first. Mechanics shared via prompt_reply.go.
func (h *BotHandler) handlePermissionTextResponse(hCtx HandlerContext) bool {
	return handlePromptTextReply(h.imPrompter,
		func(text string) { h.SendText(hCtx, text) },
		hCtx.ChatID, hCtx.SenderID, hCtx.Text())
}

// SendText sends a plain text message
// Note: Platform handles chunking internally via BaseBot.ChunkText()
func (h *BotHandler) reactReceived(hCtx HandlerContext) {
	if hCtx.MessageID == "" {
		return
	}
	emoji := imbot.ResolveReaction(hCtx.Platform, imbot.ReactionToken(imbot.ReactionReceived))
	if err := hCtx.Bot.React(context.Background(), hCtx.MessageID, emoji); err != nil {
		logrus.WithError(err).WithField("messageID", hCtx.MessageID).Warn("React received failed")
	}
}

// reactDone sends a "done" reaction on the user's message to indicate processing is complete.
// Errors are silently ignored — platforms that don't support reactions degrade gracefully.
func (h *BotHandler) reactDone(hCtx HandlerContext) {
	if hCtx.MessageID == "" {
		return
	}
	emoji := imbot.ResolveReaction(hCtx.Platform, imbot.ReactionToken(imbot.ReactionDone))
	if err := hCtx.Bot.React(context.Background(), hCtx.MessageID, emoji); err != nil {
		logrus.WithError(err).WithField("messageID", hCtx.MessageID).Warn("React done failed")
	}
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
