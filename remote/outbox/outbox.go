// Package outbox implements agent.Outbox over Remote's bots: it picks the bot
// that reaches a chat, remembers it, and drops what the chat's access policy
// refuses.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/remote/agent"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// Outbox sends to chats through the running bots of a channel.Registry.
//
// It exists before its registry does (the host builds it first and binds the
// registry once the bots are up), so a new Outbox reaches nothing and every
// send fails with agent.ErrNoChannel until Bind.
type Outbox struct {
	mu       sync.RWMutex
	registry *channel.Registry
	// disabled reports a blocklisted chat; nothing is sent to one.
	disabled      func(chatID string) bool
	targetBlocked func(ctx context.Context, botUUID, platform, chatID string) (bool, error)
	// bots remembers which bot a chat last wrote through, so replies go back
	// through the same one when several bots share a platform.
	bots map[string]string
}

var _ agent.Outbox = (*Outbox)(nil)

// New returns an Outbox that reaches nothing until Bind.
func New() *Outbox { return &Outbox{bots: map[string]string{}} }

// Bind connects the outbox to the running bots' channels. disabled may be nil.
func (o *Outbox) Bind(registry *channel.Registry, disabled func(chatID string) bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.registry, o.disabled = registry, disabled
}

// SetTargetBlocked connects the bot-scoped access control used by every send.
// A lookup failure prevents delivery.
func (o *Outbox) SetTargetBlocked(check func(ctx context.Context, botUUID, platform, chatID string) (bool, error)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.targetBlocked = check
}

func chatKey(platform, chatID string) string { return platform + "\x00" + chatID }

// RememberBot records the bot a chat wrote through; sends to the chat go back
// through it.
func (o *Outbox) RememberBot(platform, chatID, botUUID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.bots[chatKey(platform, chatID)] = botUUID
}

// BotFor is the bot a chat last wrote through, or "" when none is remembered.
func (o *Outbox) BotFor(platform, chatID string) string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.bots[chatKey(platform, chatID)]
}

// ForgetBots drops every remembered bot, as a restart does.
func (o *Outbox) ForgetBots() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.bots = map[string]string{}
}

var errChatDisabled = errors.New("the chat is disabled")

func (o *Outbox) checkTarget(ctx context.Context, botUUID, platform, chatID string) error {
	o.mu.RLock()
	disabled, check := o.disabled, o.targetBlocked
	o.mu.RUnlock()
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

// Text implements agent.Outbox.
func (o *Outbox) Text(ctx context.Context, to agent.ChatRef, text string) error {
	_, err := o.through(ctx, to, func(ch channel.Channel) error {
		return ch.Send(ctx, channel.Target{ChatID: to.ChatID}, interaction.Notification{Body: text})
	})
	if errors.Is(err, errChatDisabled) {
		// A blocklisted chat gets nothing, bound or not.
		return nil
	}
	return err
}

// File implements agent.Outbox.
func (o *Outbox) File(ctx context.Context, to agent.ChatRef, f agent.OutFile) error {
	_, err := o.through(ctx, to, func(ch channel.Channel) error {
		sender, ok := ch.(channel.FileSender)
		if !ok {
			return fmt.Errorf("file delivery is not supported by %s", to.Platform)
		}
		return sender.SendFile(ctx, to.ChatID, f.Path, f.Name, f.MediaType, f.Caption)
	})
	return err
}

// Prompt implements agent.Outbox. The returned reference names the bot that
// sent the prompt, so it is restated through the same one.
func (o *Outbox) Prompt(ctx context.Context, to agent.ChatRef, p agent.Prompt) (string, error) {
	buttons := make([]channel.Button, len(p.Choices))
	for i, c := range p.Choices {
		buttons[i] = channel.Button{Label: c.Label, Payload: []string{agent.AnswerPayload, p.ThreadID, p.RequestID, c.Value}}
	}
	var messageID string
	botUUID, err := o.through(ctx, to, func(ch channel.Channel) error {
		ac, ok := ch.(channel.ActionSender)
		if !ok {
			return agent.ErrNoButtons
		}
		id, err := ac.SendActions(ctx, to.ChatID, p.Text, buttons)
		if errors.Is(err, channel.ErrNoButtons) {
			return agent.ErrNoButtons
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

// Restate implements agent.Outbox.
func (o *Outbox) Restate(ctx context.Context, to agent.ChatRef, ref, text string) error {
	botUUID, messageID, ok := strings.Cut(ref, "\x00")
	if !ok {
		return nil
	}
	o.mu.RLock()
	reg := o.registry
	o.mu.RUnlock()
	if reg == nil {
		return agent.ErrNoChannel
	}
	if err := o.checkTarget(ctx, botUUID, to.Platform, to.ChatID); err != nil {
		if errors.Is(err, errChatDisabled) {
			return nil
		}
		return err
	}
	ch, found := reg.Get(botUUID)
	if !found {
		return fmt.Errorf("%w: bot %s stopped", agent.ErrNoChannel, botUUID)
	}
	ac, isAction := ch.(channel.ActionSender)
	if !isAction {
		return nil
	}
	return ac.Restate(ctx, to.ChatID, messageID, text)
}

// through runs send with the channel of the bot to names, else of the bot the
// chat last wrote through, else with each bot of the chat's platform until one
// succeeds, and returns the bot that did.
func (o *Outbox) through(ctx context.Context, to agent.ChatRef, send func(channel.Channel) error) (string, error) {
	platform, chatID, chosenBot := to.Platform, to.ChatID, to.BotUUID
	o.mu.RLock()
	reg, botUUID := o.registry, o.bots[chatKey(platform, chatID)]
	o.mu.RUnlock()
	if reg == nil {
		return "", agent.ErrNoChannel
	}

	if chosenBot != "" {
		ch, ok := reg.Get(chosenBot)
		if !ok || ch.Platform() != platform {
			return "", fmt.Errorf("%w: bot %s", agent.ErrNoChannel, chosenBot)
		}
		if err := o.checkTarget(ctx, chosenBot, platform, chatID); err != nil {
			return "", err
		}
		return chosenBot, send(ch)
	}
	if botUUID != "" {
		if ch, ok := reg.Get(botUUID); ok {
			if err := o.checkTarget(ctx, botUUID, platform, chatID); err != nil {
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
		if err := o.checkTarget(ctx, ch.ID(), platform, chatID); err != nil {
			return "", err
		}
		err := send(ch)
		if err == nil {
			o.RememberBot(platform, chatID, ch.ID())
			return ch.ID(), nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return "", fmt.Errorf("%w: %s", agent.ErrNoChannel, platform)
	}
	logrus.WithField("platform", platform).Debug("outbox: chat unreachable through every bot of its platform")
	return "", errors.Join(errs...)
}
