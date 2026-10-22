package remoteagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

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
	// copy is the host-less path (the test harness — see
	// remoteagent.BootForTest), which never enters manager.go's dispatch
	// chain. Two paths into the handler → two gates; see the spec
	// (bot-chat-lifecycle-collapse §3b).
	if h.chatStore.IsChatDisabled(chatID) {
		logrus.Debugf("chat %s is disabled, dropping message", chatID)
		return
	}

	// An action firing (a button press) dispatches on its payload, not on
	// message text.
	if msg.IsCallback() {
		h.handleCallbackQuery(b, botUUID, chatID, msg)
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

	// Without Projects a chat has access only: pairing and /join (and the
	// notifications the host sends).
	text := hCtx.Text()
	if !h.projectsOn() {
		if isBindCommand(text) || isJoinCommand(text) {
			h.handleSlashCommand(hCtx, text)
			return
		}
		h.SendText(hCtx, chatControlOff)
		return
	}
	// Every chat has a place in Projects: its folder, or the bot's default.
	h.adoptChat(hCtx)

	// Files go to the chat's session as attachments, with their caption.
	if msg.IsMediaContent() {
		mc, _ := msg.Content.(*imbot.MediaContent)
		h.routeMediaToProject(hCtx, mc)
		return
	}
	if !msg.IsTextContent() {
		h.SendText(hCtx, "Only text and media messages are supported.")
		return
	}
	if text == "" {
		return
	}

	// A chat typing a path for the directory browser.
	if h.directoryBrowser.IsWaitingInput(hCtx.ChatID) && !strings.HasPrefix(text, "/") {
		h.handleCustomPathInput(hCtx)
		return
	}

	// Commands: Projects runs the session ones; /cd, /dir, /help and access
	// are the registry's; nothing else is a command. The old agent words
	// ("@cc fix it", "/tb hi") are not commands: they go on as messages.
	if strings.HasPrefix(text, "/") && stripAgentPrefix(text) == text {
		if name, arg, ok := projectCommand(text); ok {
			h.runProjectCommand(hCtx, name, arg)
			return
		}
		if !h.handleSlashCommand(hCtx, text) {
			head, _, _ := strings.Cut(text, " ")
			h.SendText(hCtx, "Unknown command "+head+". /help lists what works here.")
		}
		return
	}

	h.reactReceived(hCtx)
	h.routeToProject(hCtx, text)
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

// reactReceived marks the user's message as taken.
func (h *BotHandler) reactReceived(hCtx HandlerContext) {
	if hCtx.MessageID == "" {
		return
	}
	emoji := imbot.ResolveReaction(hCtx.Platform, imbot.ReactionToken(imbot.ReactionReceived))
	if err := hCtx.Bot.React(context.Background(), hCtx.MessageID, emoji); err != nil {
		logrus.WithError(err).WithField("messageID", hCtx.MessageID).Warn("React received failed")
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
