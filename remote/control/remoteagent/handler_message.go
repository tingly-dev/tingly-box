package remoteagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/smart_guide"

	"github.com/tingly-dev/tingly-box/imbot"
)

func (h *BotHandler) HandleMessage(msg imbot.Message, platform imbot.Platform, botUUID string) {
	b := h.manager.GetBot(botUUID, platform)
	if b == nil {
		return
	}

	chatID := msg.GetReplyTarget()
	if chatID == "" {
		return
	}

	// Blocklist gate: a disabled chat's traffic — messages AND button
	// callbacks — is dropped before any handler runs. Silently: replying
	// would give a blocked party a probe signal and generate outbound
	// traffic for exactly the chats disable exists to stop.
	//
	// This check is duplicated in bot/manager.go's disabledChatGate on
	// purpose: that gate runs at the host level, ahead of every consumer
	// (including promptReplyRouter), and is what stops a disabled chat with a
	// pending permission prompt from being answered in managed mode. This
	// copy is the standalone / host-less path (the CLI and the test harness —
	// see remoteagent.BootForTest), which never enters manager.go's dispatch
	// chain. Two paths into the handler → two gates; see the spec
	// (bot-chat-lifecycle-collapse §3b).
	if h.chatStore.IsChatDisabled(chatID) {
		logrus.Debugf("chat %s is disabled, dropping message", chatID)
		return
	}

	// An action firing (a button press) dispatches on its payload, not on
	// message text.
	if msg.IsCallback() {
		h.handleCallbackQuery(b, chatID, msg)
		return
	}

	// Build the per-message context once, ahead of the access gates and every
	// handler below — a cheap struct literal with no side effects.
	hCtx := HandlerContext{
		Bot:       b,
		BotUUID:   botUUID,
		ChatID:    chatID,
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Platform:  platform,
		Message:   msg,
	}

	// Access gates: pairing for a DM or the legacy whitelist for a group.
	// handleInboundGate returns
	// true when the message is rejected — a rejection reply was already sent
	// (or, for the disabled case above, silently dropped).
	if h.handleInboundGate(hCtx) {
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
	if text == "" {
		logrus.Warn("Text content is empty, returning")
		return
	}

	// Check for stop commands FIRST (highest priority)
	// Supports: /stop, stop, /clear (stop+clear)
	if isStopCommand(text) {
		h.handleStopCommand(hCtx, text == "/clear")
		return
	}

	// React to indicate the message is being processed (after stop check, before all other handling)
	h.reactReceived(hCtx)

	// Handoff commands take precedence over the slash dispatcher: /cc, /tb
	// look like slash commands but they're really handoff sugar. Without
	// this check they'd fall to handleSlashCommands → "Unknown command"
	// since the registry doesn't (and shouldn't) own them.
	if _, isHandoff, _ := smart_guide.DetectHandoffCommand(text); isHandoff {
		if routeErr := h.routeToAgent(hCtx, text); routeErr != nil {
			logrus.WithError(routeErr).Error("Failed to route handoff command")
			h.SendText(hCtx, executionErrorMessage(routeErr))
		}
		return
	}

	// Handle commands
	if strings.HasPrefix(text, "/") {
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
	if routeErr := h.routeToAgent(hCtx, text); routeErr != nil {
		logrus.WithError(routeErr).Error("Failed to route to agent")
		h.SendText(hCtx, executionErrorMessage(routeErr))
	}
}

// handleInboundGate runs the message-shape-specific compatibility gate before
// a non-callback message reaches the handler: pairing for a direct message or
// the legacy whitelist for a group. Target capability access is enforced by
// the host authorization gate; ChatIDLock is deliberately not consulted here
// because explicit DirectChat/Group access is the sole authorization source.
// Returns true when the message is rejected (a
// rejection reply was already sent).
//
// DM and group are two explicit branches rather than one flat gate list
// because their semantics genuinely differ: the pairing gate lets an unpaired
// chat fall through to the /bind handler (so a chat can still pair itself),
// while the whitelist gate does not — collapsing both into one uniform
// func(ctx) bool would either drop the /bind escape hatch or fake it with a
// side channel. See the spec (bot-chat-lifecycle-collapse §3a).
func (h *BotHandler) handleInboundGate(hCtx HandlerContext) bool {
	msg := hCtx.Message
	switch {
	case msg.IsDirectMessage():
		logrus.Infof("Chat ID: %s", hCtx.ChatID)
		return h.handlePairingGate(hCtx)
	case msg.IsGroupMessage():
		logrus.Infof("Group chat ID: %s", hCtx.ChatID)
		return h.handleWhitelistGate(hCtx)
	default:
		logrus.Errorf("Unsupported message from upstream: %v", msg)
		h.SendText(hCtx, fmt.Sprintf("Unsupported message from upstream %s %s.", msg.ChatType, hCtx.ChatID))
		return true
	}
}

// handlePairingGate enforces pairing for a direct message: when
// RequirePairing is on, only paired chats reach the regular handlers — but an
// unpaired chat may still send /bind, which falls through to the bind handler
// that verifies the code. Returns true when the message is rejected (a
// pairing hint was sent).
func (h *BotHandler) handlePairingGate(hCtx HandlerContext) bool {
	if !h.botSetting.IsRequirePairing() {
		return false
	}
	if h.chatStore.IsChatPaired(hCtx.ChatID, hCtx.BotUUID) {
		return false
	}
	if isBindCommand(hCtx.Text()) {
		// Fall through so the /bind handler can verify the code.
		return false
	}
	logrus.WithFields(logrus.Fields{
		"action":   "imbot.pair.unpaired_message",
		"user_id":  hCtx.Message.Sender.ID,
		"bot_uuid": hCtx.BotUUID,
		"chat_id":  hCtx.ChatID,
		"platform": string(hCtx.Platform),
	}).Warn("rejected unpaired direct message")
	h.SendText(hCtx, pairingHintMessage())
	return true
}

// handleWhitelistGate enforces the group whitelist: a group must be
// whitelisted, and (when pairing is required) whitelisted by an operator who
// is themselves paired in DM. Returns true when the message is rejected (a
// hint was sent).
func (h *BotHandler) handleWhitelistGate(hCtx HandlerContext) bool {
	if !h.chatStore.IsWhitelisted(hCtx.ChatID) {
		logrus.Debugf("Group %s is not whitelisted, ignoring message", hCtx.ChatID)
		h.SendText(hCtx, fmt.Sprintf("This group is not enabled. Please DM the bot with `%s %s` to enable.", cmdJoinPrimary, hCtx.ChatID))
		return true
	}
	if h.botSetting.IsRequirePairing() && !h.isWhitelisterPaired(hCtx.ChatID, hCtx.BotUUID) {
		logrus.WithFields(logrus.Fields{
			"action":   "imbot.pair.unpaired_message",
			"user_id":  hCtx.Message.Sender.ID,
			"bot_uuid": hCtx.BotUUID,
			"chat_id":  hCtx.ChatID,
			"platform": string(hCtx.Platform),
		}).Warn("rejected group whitelisted by unpaired operator")
		h.SendText(hCtx, "🔒 This group's operator has not paired with the bot. Ask them to send /bind <code> in a DM first.")
		return true
	}
	return false
}

// handleMediaMessage handles messages with media attachments
func (h *BotHandler) handleMediaMessage(hCtx HandlerContext, media []imbot.MediaAttachment) {
	// Get project path for storage, use default if not bound
	projectPath, ok := h.getProjectPath(hCtx)
	if !ok {
		projectPath = h.defaultProjectPath()
	}

	// Set platform-specific token on FileStore if needed
	if len(media) > 0 && strings.HasPrefix(media[0].URL, "tgfile://") {
		if token := h.botSetting.Auth["token"]; token != "" {
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
	return bot.HandlePromptTextReply(h.imPrompter,
		func(text string) { h.SendText(hCtx, text) },
		hCtx.ChatID, hCtx.SenderID, hCtx.Text(), bot.ReplyToMessageID(hCtx.Message))
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
