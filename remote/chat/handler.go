package chat

import (
	"context"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/agent"
	bot2 "github.com/tingly-dev/tingly-box/remote/bot"
	"github.com/tingly-dev/tingly-box/remote/chat/feature"
)

// BotHandler handles what a chat sends a bot: access (pairing, group
// whitelist), which folder it works in (/cd, /dir, the directory browser),
// and everything else, which goes to Claude Code through Projects
// (project_route.go).
type BotHandler struct {
	ctx              context.Context
	botSetting       bot2.BotSetting
	chatStore        bot2.ChatStoreInterface // Use interface for flexibility
	directoryBrowser *feature.DirectoryBrowser
	manager          *imbot.Manager
	fileStore        *FileStore
	// store reads the bot's current settings (nil: botSetting as built).
	store bot2.SettingsStore

	// commandRegistry holds the strongly-typed command registry
	commandRegistry *imbot.CommandRegistry

	// pairing handles TOFU pairing-code verification for direct messages.
	pairing *bot2.PairingManager

	// projectRouter hands chats to Projects; nil, or switched off, leaves
	// chats with access and notifications only. See project_route.go.
	projectRouter ProjectRouter
	// privileged tells /help whether a chat may use privileged actions; nil
	// when the host has no access policy.
	privileged     bot2.PrivilegeChecker
	answerAllowed  bot2.PrivilegeChecker
	startAllowed   bot2.PrivilegeChecker
	projectListsMu sync.Mutex
	projectLists   map[string][]agent.ProjectChoice
	folderLists    map[string][]string
}

// HandlerContext contains per-message context data
type HandlerContext struct {
	Bot       imbot.Bot
	BotUUID   string
	ChatID    string
	SenderID  string
	MessageID string
	Platform  imbot.Platform
	Message   imbot.Message
}

func (c *HandlerContext) IsDirect() bool {
	return c.Message.IsDirectMessage()
}

func (c *HandlerContext) Text() string {
	return strings.TrimSpace(c.Message.GetText())
}
