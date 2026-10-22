package remoteagent

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
	bot2 "github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/feature"

	"github.com/tingly-dev/tingly-box/imbot"
)

func NewBotHandler(
	ctx context.Context,
	botSetting bot2.BotSetting,
	chatStore bot2.ChatStoreInterface,
	directoryBrowser *feature.DirectoryBrowser,
	manager *imbot.Manager,
	pairing *bot2.PairingManager,
	store bot2.SettingsStore,
) *BotHandler {
	// Create file store with proxy support
	fileStore, err := NewFileStoreWithProxy(botSetting.ProxyURL)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create file store with proxy, using default")
		fileStore = NewFileStore()
	}

	// Set telegram token for file URL resolution
	if token, ok := botSetting.Auth["token"]; ok {
		fileStore.SetTelegramToken(token)
	}

	handler := &BotHandler{
		ctx:              ctx,
		botSetting:       botSetting,
		chatStore:        chatStore,
		directoryBrowser: directoryBrowser,
		manager:          manager,
		fileStore:        fileStore,
		store:            store,
		pairing:          pairing,
	}
	handler.InitCommandRegistry()
	return handler
}

// currentSetting reads the bot's settings as they are now, so an operator's
// change (DefaultCwd) applies without a bot restart.
func (h *BotHandler) currentSetting() bot2.BotSetting {
	if h.store != nil {
		if s, err := h.store.GetSettingsByUUID(h.botSetting.UUID); err == nil {
			return s
		}
	}
	return h.botSetting
}

// getProjectPath returns the folder the chat picked (/cd, /dir); direct and
// group chats share the same chat-id key in the store.
func (h *BotHandler) getProjectPath(hCtx HandlerContext) (string, bool) {
	projectPath, hasBound, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	if hasBound && projectPath != "" {
		return projectPath, true
	}
	return "", false
}

// defaultProjectPath is the folder of a chat that never picked one: the
// bot's DefaultCwd, else the working directory, home or "/".
func (h *BotHandler) defaultProjectPath() string {
	if cwd := h.currentSetting().DefaultCwd; cwd != "" {
		if expanded, err := ExpandPath(cwd); err == nil {
			return expanded
		}
		logrus.WithField("path", cwd).Warn("Failed to expand DefaultCwd")
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}
