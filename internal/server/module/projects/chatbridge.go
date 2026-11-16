package projects

import (
	"context"
	"errors"
	"sync"

	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/agent"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/chat"
	"github.com/tingly-dev/tingly-box/remote/outbox"
)

// ChatBridge joins Remote's IM chats to Projects (.design/projects-im.md):
//
//   - inbound: a chat that is bound to a project hands its messages to that
//     project's coordinator instead of @cc / @tb (chat.ProjectRouter);
//   - outbound: the coordinator's replies and thread reports reach the bound
//     chats through Remote's outbox (projectsapp.ChannelSink, which the host
//     forwards to).
//
// The bridge exists before Projects does (the IM handler is built first), so
// it is created empty and bound to the app and the channel registry later.
// Until then, or when the Projects flag is off, it takes no messages.
type ChatBridge struct {
	mu      sync.RWMutex
	app     *projectsapp.App
	enabled func() bool
	start   func()
	// out reaches the chats; it remembers which bot a chat last wrote through.
	out *outbox.Outbox
}

// NewChatBridge returns a bridge that does nothing until Bind.
func NewChatBridge() *ChatBridge {
	return &ChatBridge{out: outbox.New()}
}

// Bind connects the bridge to Projects and to the running bots' channels.
// enabled is the Projects feature flag and start brings the runtime up on
// first use, like the HTTP gate does; disabled may be nil.
func (b *ChatBridge) Bind(app *projectsapp.App, registry *channel.Registry, enabled func() bool, start func(), disabled func(chatID string) bool) {
	b.mu.Lock()
	b.app, b.enabled, b.start = app, enabled, start
	b.mu.Unlock()
	b.out.Bind(registry, disabled)
}

// SetTargetBlocked connects the bot-scoped access control used by outbound
// messages and prompt updates. A lookup failure prevents delivery.
func (b *ChatBridge) SetTargetBlocked(check func(context.Context, string, string, string) (bool, error)) {
	b.out.SetTargetBlocked(check)
}

// live returns the app when Projects is switched on.
func (b *ChatBridge) live() *projectsapp.App {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.app == nil || b.enabled == nil || !b.enabled() {
		return nil
	}
	return b.app
}

// Bound reports whether the chat has a place in Projects.
func (b *ChatBridge) Bound(platform, chatID string) bool {
	app := b.live()
	if app == nil {
		return false
	}
	place, err := app.Place(agent.ChatRef{Platform: platform, ChatID: chatID})
	return err == nil && place.Bound
}

// Enabled implements chat.ProjectRouter: chats follow the Projects
// switch.
func (b *ChatBridge) Enabled() bool { return b.live() != nil }

// errOff is what a chat call gets while Projects is switched off.
var errOff = errors.New("Projects is switched off")

// ToFolder implements agent.Chat.
func (b *ChatBridge) ToFolder(ref agent.ChatRef, folder string, ifNew bool) error {
	app := b.live()
	if app == nil {
		return errOff
	}
	return app.ToFolder(ref, folder, ifNew)
}

// Folders implements agent.Chat: Projects' current, existing working
// folders.
func (b *ChatBridge) Folders() ([]string, error) {
	app := b.live()
	if app == nil {
		return nil, errOff
	}
	return app.Folders()
}

// Place implements agent.Chat: where the chat is, from the actual
// Projects binding.
func (b *ChatBridge) Place(ref agent.ChatRef) (agent.ChatPlace, error) {
	app := b.live()
	if app == nil {
		return agent.ChatPlace{}, errOff
	}
	return app.Place(ref)
}

// Projects implements agent.Chat: project coordinators, separately from
// working folders.
func (b *ChatBridge) Projects() ([]agent.ProjectChoice, error) {
	app := b.live()
	if app == nil {
		return nil, errOff
	}
	return app.Projects()
}

// ToProject implements agent.Chat: it binds the chat to the project's
// coordinator.
func (b *ChatBridge) ToProject(ref agent.ChatRef, projectID string) error {
	app := b.live()
	if app == nil {
		return errOff
	}
	return app.ToProject(ref, projectID)
}

// touch remembers the bot a chat wrote through, so replies go back through it,
// and brings the Projects runtime up on first use.
func (b *ChatBridge) touch(ref agent.ChatRef, startRuntime bool) {
	b.RememberBot(ref.Platform, ref.ChatID, ref.BotUUID)
	b.mu.RLock()
	start := b.start
	b.mu.RUnlock()
	if startRuntime && start != nil {
		start()
	}
}

// RememberBot records the bot a chat wrote through; replies to the chat go
// back through it.
func (b *ChatBridge) RememberBot(platform, chatID, botUUID string) {
	b.out.RememberBot(platform, chatID, botUUID)
}

// Folder is the folder a chat is on in Projects, or "" when it is on a
// project, has no place yet, or Projects is off.
func (b *ChatBridge) Folder(platform, chatID string) string {
	place, err := b.Place(agent.ChatRef{Platform: platform, ChatID: chatID})
	if err != nil {
		return ""
	}
	return place.Folder
}

// BotFor is the bot a chat last wrote through, or "" when none is remembered.
func (b *ChatBridge) BotFor(platform, chatID string) string { return b.out.BotFor(platform, chatID) }

// ForgetBots drops every remembered bot, as a restart does.
func (b *ChatBridge) ForgetBots() { b.out.ForgetBots() }

// App returns the Projects app while it is switched on, else nil.
func (b *ChatBridge) App() *projectsapp.App { return b.live() }

// MayAnswerText identifies candidate text replies without consuming them.
func (b *ChatBridge) MayAnswerText(ref agent.ChatRef, text string) bool {
	app := b.live()
	return app != nil && app.MayAnswerText(ref, text)
}

// Post implements agent.Chat.
func (b *ChatBridge) Post(ref agent.ChatRef, text string, files []agent.File) (bool, error) {
	app := b.live()
	if app == nil || !b.Bound(ref.Platform, ref.ChatID) {
		return false, nil
	}
	b.touch(ref, true)
	return app.Post(ref, text, files)
}

// Command implements agent.Chat.
func (b *ChatBridge) Command(ref agent.ChatRef, name, arg string) (bool, string, error) {
	app := b.live()
	if app == nil || !b.Bound(ref.Platform, ref.ChatID) {
		return false, "", nil
	}
	b.touch(ref, true)
	return app.Command(ref, name, arg)
}

// Answer implements agent.Chat.
func (b *ChatBridge) Answer(ref agent.ChatRef, threadID, requestID, value string) (string, error) {
	app := b.live()
	if app == nil {
		return "Projects is switched off.", nil
	}
	b.touch(ref, false)
	return app.Answer(ref, threadID, requestID, value)
}

var _ chat.ProjectRouter = (*ChatBridge)(nil)

// Outbox is how Projects reaches the bridge's chats: Remote's outbox, which
// remembers the bot each chat wrote through (see Post, Command, Answer).
func (b *ChatBridge) Outbox() agent.Outbox { return b.out }
