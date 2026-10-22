package remoteagent

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot"
)

// botHandlerAdapter implements command.BotHandlerAdapter by delegating to BotHandler.
type botHandlerAdapter struct {
	handler *BotHandler
}

// NewBotHandlerAdapter creates a new adapter for the given handler.
func NewBotHandlerAdapter(handler *BotHandler) BotHandlerAdapter {
	return &botHandlerAdapter{handler: handler}
}

// ctx builds a HandlerContext for a command-registry invocation, carrying the
// bot's OWN platform. The registry dispatches without a message, so there is
// no inbound HandlerContext to inherit — but the platform must still be the
// real one: it is persisted onto the chat record (BindProject), and a wrong
// value both mis-attributes the chat and hides it from its own bot's
// GET /bots/:bot/chats listing.
func (a *botHandlerAdapter) ctx(chatID string) HandlerContext {
	return HandlerContext{
		ChatID:   chatID,
		Platform: imbot.Platform(a.handler.botSetting.Platform),
	}
}

// SendText sends a text message to a chat.
func (a *botHandlerAdapter) SendText(chatID, text string) error {
	a.handler.SendText(a.ctx(chatID), text)
	return nil
}

// GetProjectPath gets the folder of a chat.
func (a *botHandlerAdapter) GetProjectPath(chatID string) (string, error) {
	projectPath, _, err := a.handler.chatStore.GetProjectPath(chatID)
	return projectPath, err
}

// SetProjectPath moves a chat to a folder.
func (a *botHandlerAdapter) SetProjectPath(chatID, path string) error {
	expandedPath, err := ExpandPath(path)
	if err != nil {
		return err
	}
	a.handler.completeBind(a.ctx(chatID), expandedPath)
	return nil
}

// IsWhitelisted checks if a group is whitelisted.
func (a *botHandlerAdapter) IsWhitelisted(groupID string) bool {
	return a.handler.chatStore.IsWhitelisted(groupID)
}

// AddToWhitelist adds a group to whitelist.
func (a *botHandlerAdapter) AddToWhitelist(groupID, platform, userID string) error {
	return a.handler.chatStore.AddToWhitelist(groupID, platform, userID)
}

// ListChatProjectPaths returns the folders a chat used, most recent first.
func (a *botHandlerAdapter) ListChatProjectPaths(chatID string) ([]string, error) {
	return a.handler.chatStore.ListChatProjectPaths(chatID)
}

// VerifyAndPair runs pairing-code verification and persists the binding.
func (a *botHandlerAdapter) VerifyAndPair(botUUID, chatID, senderID, platform, code string) error {
	return a.handler.VerifyAndPair(botUUID, chatID, senderID, platform, code)
}

// RunSessionCommand runs a session command in Projects.
func (a *botHandlerAdapter) RunSessionCommand(ctx *imbot.HandlerContext, name, arg string) {
	a.handler.runProjectCommand(HandlerContext{
		Bot:      ctx.Bot,
		BotUUID:  a.handler.botSetting.UUID,
		ChatID:   ctx.ChatID,
		SenderID: ctx.SenderID,
		Platform: ctx.Platform,
	}, name, arg)
}

// InitCommandRegistry initializes the command registry with built-in commands.
func (h *BotHandler) InitCommandRegistry() error {
	registry := imbot.NewCommandRegistry()
	if err := RegisterBuiltinCommands(registry, NewBotHandlerAdapter(h)); err != nil {
		return fmt.Errorf("failed to register built-in commands: %w", err)
	}
	h.commandRegistry = registry
	return nil
}

// GetCommandRegistry returns the command registry.
func (h *BotHandler) GetCommandRegistry() *imbot.CommandRegistry {
	return h.commandRegistry
}
