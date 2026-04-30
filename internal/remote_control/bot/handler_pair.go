package bot

import (
	"errors"
	"strings"
)

// isBindCommand reports whether text is a (well-formed-enough) /bind invocation.
func isBindCommand(text string) bool {
	t := strings.TrimSpace(text)
	if t == "/bind" {
		return true
	}
	return strings.HasPrefix(t, "/bind ") || strings.HasPrefix(t, "/bind\t")
}

// pairingHintMessage returns the user-facing prompt sent when an unpaired
// chat tries to talk to a bot that requires pairing.
func pairingHintMessage() string {
	return "🔒 This bot requires pairing.\n" +
		"Ask the operator for the current pairing code, then send:\n" +
		"`/bind <code>`"
}

// userFacingPairError translates a PairingManager error into a short, safe
// string that does not leak internal state.
func userFacingPairError(err error) string {
	switch {
	case errors.Is(err, ErrPairLocked):
		return "Too many failed attempts. Try again in a few minutes."
	case errors.Is(err, ErrPairCodeExpired):
		return "Pairing code expired. Ask the operator for a fresh one."
	case errors.Is(err, ErrPairCodeMissing):
		return "No pairing code is currently active. Ask the operator to issue one."
	case errors.Is(err, ErrPairCodeMismatch):
		return "Incorrect pairing code."
	default:
		return "Pairing failed."
	}
}

// isWhitelisterPaired reports whether the operator who originally whitelisted
// the group chat is themselves paired with this bot. Required so that group
// access cannot be re-enabled by an attacker who steals the bot token.
func (h *BotHandler) isWhitelisterPaired(groupChatID, botUUID string) bool {
	if h == nil || h.chatStore == nil {
		return false
	}
	chat, err := h.chatStore.GetChat(groupChatID)
	if err != nil || chat == nil || chat.WhitelistedBy == "" {
		return false
	}
	owners, err := h.chatStore.ListChatsByOwner(chat.WhitelistedBy, chat.Platform)
	if err != nil {
		return false
	}
	for _, c := range owners {
		if c == nil {
			continue
		}
		if c.IsPaired && c.PairedBotUUID == botUUID {
			return true
		}
	}
	return false
}

// VerifyAndPair runs the pairing-code check and, on success, persists the
// binding in the chat store. It is invoked by the /bind command handler.
func (h *BotHandler) VerifyAndPair(botUUID, chatID, senderID, platform, code string) error {
	if h == nil {
		return errors.New("internal error")
	}
	if h.pairing == nil {
		return errors.New("pairing is not enabled on this bot")
	}
	if err := h.pairing.Verify(botUUID, code); err != nil {
		h.auditWarn("imbot.pair.fail", senderID, "verify failed", map[string]interface{}{
			"bot_uuid": botUUID,
			"chat_id":  chatID,
			"platform": platform,
			"reason":   err.Error(),
		})
		return errors.New(userFacingPairError(err))
	}
	if err := h.chatStore.SetPaired(chatID, platform, botUUID, senderID); err != nil {
		return err
	}
	h.auditInfo("imbot.pair.success", senderID, "chat paired", map[string]interface{}{
		"bot_uuid": botUUID,
		"chat_id":  chatID,
		"platform": platform,
	})
	return nil
}

// auditInfo records an info-level audit event if a logger is configured.
func (h *BotHandler) auditInfo(action, userID, message string, details map[string]interface{}) {
	if h == nil || h.audit == nil {
		return
	}
	h.audit.Info(action, userID, "", message, details)
}

// auditWarn records a warn-level audit event if a logger is configured.
func (h *BotHandler) auditWarn(action, userID, message string, details map[string]interface{}) {
	if h == nil || h.audit == nil {
		return
	}
	h.audit.Warn(action, userID, "", message, details)
}
