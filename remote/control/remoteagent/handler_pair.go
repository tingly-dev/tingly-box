package remoteagent

import (
	"errors"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// isBindCommand reports whether text is a (well-formed-enough) /bind invocation.
func isBindCommand(text string) bool {
	t := strings.TrimSpace(text)
	if t == "/bind" {
		return true
	}
	return strings.HasPrefix(t, "/bind ") || strings.HasPrefix(t, "/bind\t")
}

// isJoinCommand reports whether text is a /join invocation.
func isJoinCommand(text string) bool {
	head, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	return head == cmdJoinPrimary
}

// pairingHintMessage returns the user-facing prompt sent when an unpaired
// chat tries to talk to a bot that requires pairing.
func pairingHintMessage() string {
	return "🔒 This bot requires pairing.\n" +
		"Ask the operator for the current pairing code, then send:\n" +
		"`/bind <code>`"
}

// userFacingPairError translates a bot.PairingManager error into a short, safe
// string that does not leak internal state.
func userFacingPairError(err error) string {
	switch {
	case errors.Is(err, bot.ErrPairLocked):
		return "Too many failed attempts. Try again in a few minutes."
	case errors.Is(err, bot.ErrPairCodeExpired):
		return "Pairing code expired. Ask the operator for a fresh one."
	case errors.Is(err, bot.ErrPairCodeMissing):
		return "No pairing code is currently active. Ask the operator to issue one."
	case errors.Is(err, bot.ErrPairCodeMismatch):
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
		logrus.WithFields(logrus.Fields{
			"action":   "imbot.pair.fail",
			"user_id":  senderID,
			"bot_uuid": botUUID,
			"chat_id":  chatID,
			"platform": platform,
			"reason":   err.Error(),
		}).Warn("verify failed")
		return errors.New(userFacingPairError(err))
	}
	if err := h.chatStore.SetPaired(chatID, platform, botUUID, senderID); err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{
		"action":   "imbot.pair.success",
		"user_id":  senderID,
		"bot_uuid": botUUID,
		"chat_id":  chatID,
		"platform": platform,
	}).Info("chat paired")
	return nil
}
