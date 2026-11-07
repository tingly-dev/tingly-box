package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/agent"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/chat"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// ChatBridge joins Remote's IM chats to Projects (.design/projects-im.md):
//
//   - inbound: a chat that is bound to a project hands its messages to that
//     project's coordinator instead of @cc / @tb (chat.ProjectRouter);
//   - outbound: the coordinator's replies and thread reports reach the bound
//     chats through the bot's channel, the same send path notifications use
//     (projectsapp.ChannelSink, which the host forwards to).
//
// The bridge exists before Projects does (the IM handler is built first), so
// it is created empty and bound to the app and the channel registry later.
// Until then, or when the Projects flag is off, it takes no messages.
type ChatBridge struct {
	mu       sync.RWMutex
	app      *projectsapp.App
	registry *channel.Registry
	enabled  func() bool
	start    func()
	// disabled reports a blocklisted chat; nothing is sent to one.
	disabled      func(chatID string) bool
	targetBlocked func(context.Context, string, string, string) (bool, error)
	// bots remembers which bot a bound chat last wrote through, so replies
	// go back through the same one when several bots share a platform.
	bots map[string]string
}

// NewChatBridge returns a bridge that does nothing until Bind.
func NewChatBridge() *ChatBridge {
	return &ChatBridge{bots: map[string]string{}}
}

// Bind connects the bridge to Projects and to the running bots' channels.
// enabled is the Projects feature flag and start brings the runtime up on
// first use, like the HTTP gate does; disabled may be nil.
func (b *ChatBridge) Bind(app *projectsapp.App, registry *channel.Registry, enabled func() bool, start func(), disabled func(chatID string) bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.app, b.registry, b.enabled, b.start, b.disabled = app, registry, enabled, start, disabled
}

// SetTargetBlocked connects the bot-scoped access control used by outbound
// messages and prompt updates. A lookup failure prevents delivery.
func (b *ChatBridge) SetTargetBlocked(check func(context.Context, string, string, string) (bool, error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.targetBlocked = check
}

func (b *ChatBridge) checkTarget(ctx context.Context, botUUID, platform, chatID string) error {
	b.mu.RLock()
	disabled, check := b.disabled, b.targetBlocked
	b.mu.RUnlock()
	if disabled != nil && disabled(chatID) {
		return errChatDisabled
	}
	if check != nil {
		blocked, err := check(ctx, botUUID, platform, chatID)
		if err != nil {
			return fmt.Errorf("check chat access: %w", err)
		}
		if blocked {
			return errChatDisabled
		}
	}
	return nil
}

func chatKey(platform, chatID string) string { return platform + "\x00" + chatID }

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
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bots[chatKey(platform, chatID)] = botUUID
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
func (b *ChatBridge) BotFor(platform, chatID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bots[chatKey(platform, chatID)]
}

// ForgetBots drops every remembered bot, as a restart does.
func (b *ChatBridge) ForgetBots() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bots = map[string]string{}
}

// App returns the Projects app while it is switched on, else nil.
func (b *ChatBridge) App() *projectsapp.App { return b.live() }

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

var (
	_ chat.ProjectRouter          = (*ChatBridge)(nil)
	_ projectsapp.ChannelSink     = (*ChatBridge)(nil)
	_ projectsapp.ChannelPrompter = (*ChatBridge)(nil)
)

// ErrNoChannel means no running bot can reach the chat.
var ErrNoChannel = errors.New("no running bot for this chat")

// SendChannelText implements projectsapp.ChannelSink: it sends text to a chat
// through the channel of the bot the chat last wrote through, else through the
// bots of the chat's platform.
func (b *ChatBridge) SendChannelText(ctx context.Context, platform, chatID, text string) error {
	_, err := b.through(ctx, platform, chatID, projectsapp.ChannelBot(ctx), func(ch channel.Channel) error {
		return ch.Send(ctx, channel.Target{ChatID: chatID}, interaction.Notification{Body: text})
	})
	if errors.Is(err, errChatDisabled) {
		// A blocklisted chat gets nothing, bound or not.
		return nil
	}
	return err
}

// SendChannelFile delivers a published file through the bound bot.
func (b *ChatBridge) SendChannelFile(ctx context.Context, platform, chatID, path, name, mediaType, caption string) error {
	_, err := b.through(ctx, platform, chatID, projectsapp.ChannelBot(ctx), func(ch channel.Channel) error {
		sender, ok := ch.(interface {
			SendFile(context.Context, string, string, string, string, string) error
		})
		if !ok {
			return fmt.Errorf("file delivery is not supported by %s", platform)
		}
		return sender.SendFile(ctx, chatID, path, name, mediaType, caption)
	})
	return err
}

// actionChannel is a channel that can send buttons and restate them
// (imchannel.Channel).
type actionChannel interface {
	SendActions(ctx context.Context, chatID, text string, buttons []imchannel.Button) (string, error)
	Restate(ctx context.Context, chatID, messageID, text string) error
}

// SendChannelPrompt implements projectsapp.ChannelPrompter. The returned ref
// names the bot that sent it, so the prompt is restated through the same one.
func (b *ChatBridge) SendChannelPrompt(ctx context.Context, platform, chatID string, p projectsapp.ChannelPrompt) (string, error) {
	buttons := make([]imchannel.Button, len(p.Choices))
	for i, c := range p.Choices {
		buttons[i] = imchannel.Button{Label: c.Label, Payload: []string{chat.ProjectAnswerPayload, p.ThreadID, p.RequestID, c.Value}}
	}
	var messageID string
	botUUID, err := b.through(ctx, platform, chatID, projectsapp.ChannelBot(ctx), func(ch channel.Channel) error {
		ac, ok := ch.(actionChannel)
		if !ok {
			return projectsapp.ErrNoButtons
		}
		id, err := ac.SendActions(ctx, chatID, p.Text, buttons)
		if errors.Is(err, imchannel.ErrNoButtons) {
			return projectsapp.ErrNoButtons
		}
		messageID = id
		return err
	})
	if errors.Is(err, errChatDisabled) {
		// Nothing reaches a blocklisted chat, the text line included.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return botUUID + "\x00" + messageID, nil
}

// RestateChannelPrompt implements projectsapp.ChannelPrompter.
func (b *ChatBridge) RestateChannelPrompt(ctx context.Context, platform, chatID, ref, text string) error {
	botUUID, messageID, ok := strings.Cut(ref, "\x00")
	if !ok {
		return nil
	}
	b.mu.RLock()
	reg := b.registry
	b.mu.RUnlock()
	if reg == nil {
		return ErrNoChannel
	}
	if err := b.checkTarget(ctx, botUUID, platform, chatID); err != nil {
		if errors.Is(err, errChatDisabled) {
			return nil
		}
		return err
	}
	ch, found := reg.Get(botUUID)
	if !found {
		return fmt.Errorf("%w: bot %s stopped", ErrNoChannel, botUUID)
	}
	ac, isAction := ch.(actionChannel)
	if !isAction {
		return nil
	}
	return ac.Restate(ctx, chatID, messageID, text)
}

var errChatDisabled = errors.New("the chat is disabled")

// through runs send with the channel of the bot the chat last wrote through,
// else with each bot of the chat's platform until one succeeds, and returns
// the bot that did.
func (b *ChatBridge) through(ctx context.Context, platform, chatID, chosenBot string, send func(channel.Channel) error) (string, error) {
	b.mu.RLock()
	reg, botUUID := b.registry, b.bots[chatKey(platform, chatID)]
	b.mu.RUnlock()
	if reg == nil {
		return "", ErrNoChannel
	}

	if chosenBot != "" {
		ch, ok := reg.Get(chosenBot)
		if !ok || ch.Platform() != platform {
			return "", fmt.Errorf("%w: bot %s", ErrNoChannel, chosenBot)
		}
		if err := b.checkTarget(ctx, chosenBot, platform, chatID); err != nil {
			return "", err
		}
		return chosenBot, send(ch)
	}
	if botUUID != "" {
		if ch, ok := reg.Get(botUUID); ok {
			if err := b.checkTarget(ctx, botUUID, platform, chatID); err != nil {
				return "", err
			}
			return botUUID, send(ch)
		}
	}
	// Nothing written yet since the server started (or that bot stopped): try
	// the platform's bots. A bot that does not know the chat fails, and the
	// next one is tried.
	var errs []error
	for _, ch := range reg.ByPlatform(platform) {
		if err := b.checkTarget(ctx, ch.ID(), platform, chatID); err != nil {
			return "", err
		}
		err := send(ch)
		if err == nil {
			b.mu.Lock()
			b.bots[chatKey(platform, chatID)] = ch.ID()
			b.mu.Unlock()
			return ch.ID(), nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return "", fmt.Errorf("%w: %s", ErrNoChannel, platform)
	}
	logrus.WithField("platform", platform).Debug("projects: chat unreachable through every bot of its platform")
	return "", errors.Join(errs...)
}
