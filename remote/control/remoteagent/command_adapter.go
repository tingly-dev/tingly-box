package remoteagent

import (
	"context"
	"fmt"
	"path/filepath"

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
		BotUUID:  a.handler.botSetting.UUID,
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
	if source, ok := a.handler.projectRouter.(interface {
		CurrentFolder(context.Context, string, string) (string, error)
	}); ok && a.handler.projectsOn() {
		path, err := source.CurrentFolder(a.handler.ctx, a.handler.botSetting.Platform, chatID)
		if err != nil {
			return "", err
		}
		return path, nil
	}
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
	history, err := a.handler.chatStore.ListChatProjectPaths(chatID)
	if err != nil {
		return nil, err
	}
	source, ok := a.handler.projectRouter.(interface {
		RegisteredFolders(context.Context) ([]string, error)
	})
	if !ok || !a.handler.projectsOn() {
		return history, nil
	}
	registered, err := source.RegisteredFolders(a.handler.ctx)
	if err != nil {
		return nil, err
	}
	key := func(path string) string {
		if p, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Clean(p)
		}
		return filepath.Clean(path)
	}
	allowed := make(map[string]string, len(registered))
	for _, p := range registered {
		allowed[key(p)] = p
	}
	result := make([]string, 0, len(registered))
	seen := map[string]bool{}
	for _, list := range [][]string{history, registered} {
		for _, path := range list {
			k := key(path)
			if p, ok := allowed[k]; ok && !seen[k] {
				result = append(result, p)
				seen[k] = true
			}
		}
	}
	return result, nil
}

// RememberFolderList captures the numbers shown to this chat, so a later
// selection cannot change meaning when the registered list is reordered.
func (a *botHandlerAdapter) RememberFolderList(chatID string, paths []string) {
	a.handler.projectListsMu.Lock()
	defer a.handler.projectListsMu.Unlock()
	if a.handler.folderLists == nil {
		a.handler.folderLists = map[string][]string{}
	}
	a.handler.folderLists[chatID] = append([]string(nil), paths...)
}
func (a *botHandlerAdapter) FolderList(chatID string) []string {
	a.handler.projectListsMu.Lock()
	defer a.handler.projectListsMu.Unlock()
	return append([]string(nil), a.handler.folderLists[chatID]...)
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

// RunProjectDirectory handles the separate project-coordinator picker.
func (a *botHandlerAdapter) RunProjectDirectory(ctx *imbot.HandlerContext, arg string) {
	a.handler.projectDirectory(HandlerContext{Bot: ctx.Bot, BotUUID: a.handler.botSetting.UUID, ChatID: ctx.ChatID, SenderID: ctx.SenderID, Platform: ctx.Platform}, arg)
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
