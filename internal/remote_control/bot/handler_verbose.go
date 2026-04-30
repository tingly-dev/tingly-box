package bot

import "github.com/sirupsen/logrus"

func (h *BotHandler) GetVerbose(chatID string) bool {
	// Check if platform supports verbose mode
	//if !SupportsVerboseMode(h.botSetting.Platform) {
	//	return false
	//}

	// Try to get verbose from chat store
	if h.chatStore != nil {
		chat, err := h.chatStore.GetChat(chatID)
		if err == nil && chat != nil && chat.Verbose != nil {
			return *chat.Verbose
		}
	}

	// Fallback to bot setting default
	botSetting := h.botSetting.GetOutputBehavior()
	return botSetting.Verbose
}

// SetVerbose sets the verbose mode for a chat
func (h *BotHandler) SetVerbose(chatID string, verbose bool) {
	// Update in chat store
	if h.chatStore != nil {
		err := h.chatStore.UpdateChat(chatID, func(c *Chat) {
			c.Verbose = &verbose
		})
		if err != nil {
			logrus.WithError(err).WithField("chatID", chatID).Warn("Failed to update verbose in chat store")
		}
	}

	// Also update in-memory default (fallback)
	h.verboseMu.Lock()
	h.verbose = verbose
	h.verboseMu.Unlock()
}
