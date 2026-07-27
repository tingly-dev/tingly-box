package remoteagent

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
)

func (h *BotHandler) GetVerbose(chatID string) bool {
	// Some platforms cannot carry a running commentary of intermediate
	// messages — Weixin ties each outbound message to one inbound reply
	// context, so follow-ups either fail or arrive detached. The check used to
	// be commented out here with no table to consult; it now reads imbot's
	// platform descriptor and applies regardless of the operator's setting,
	// because it is a platform limit rather than a preference.
	if imbot.GetPlatformBehavior(imbot.Platform(h.botSetting.Platform)).SuppressVerbose {
		return false
	}

	// Try to get verbose from chat store
	if h.chatStore != nil {
		chat, err := h.chatStore.GetChat(chatID)
		if err == nil && chat != nil && chat.Verbose != nil {
			return *chat.Verbose
		}
	}

	// Fallback to bot setting default (nil = verbose on).
	return h.botSetting.Verbose == nil || *h.botSetting.Verbose
}

// SetVerbose sets the verbose mode for a chat
func (h *BotHandler) SetVerbose(chatID string, verbose bool) {
	if h.chatStore == nil {
		return
	}
	if err := h.chatStore.UpdateChat(chatID, func(c *bot.Chat) {
		c.Verbose = &verbose
	}); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Warn("Failed to update verbose in chat store")
	}
}
