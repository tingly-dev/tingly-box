// Package lark provides Lark platform support as an alias to Feishu.
//
// Lark and Feishu are identical platforms except for the base URL:
//   - Feishu: https://open.feishu.cn
//   - Lark:   https://open.larksuite.com
//
// This package simply reuses the Feishu implementation with the Lark domain preset.
package lark

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/feishu"
)

// Platform constants
const (
	PlatformLark core.Platform = "lark"
)

// Bot is an alias to feishu.Bot with Lark domain preset
type Bot struct {
	*feishu.Bot
}

// NewBot creates a new Lark bot using the Feishu implementation
// with the Lark domain preset.
func NewBot(config *core.Config) (*Bot, error) {
	// Use Feishu implementation with Lark domain
	feishuBot, err := feishu.NewBot(config, feishu.DomainLark)
	if err != nil {
		return nil, err
	}

	return &Bot{Bot: feishuBot}, nil
}

// PlatformInfo returns Lark platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(PlatformLark, "Lark")
}

// Connect connects to Lark
func (b *Bot) Connect(ctx context.Context) error {
	return b.Bot.Connect(ctx)
}

// Disconnect disconnects from Lark
func (b *Bot) Disconnect(ctx context.Context) error {
	return b.Bot.Disconnect(ctx)
}

// SendMessage sends a message via Lark
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	return b.Bot.SendMessage(ctx, target, opts)
}

// SendText sends a text message via Lark
func (b *Bot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return b.Bot.SendText(ctx, target, text)
}

// SendMedia sends media via Lark
func (b *Bot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return b.Bot.SendMedia(ctx, target, media)
}

// React adds a reaction to a message
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	return b.Bot.React(ctx, messageID, emoji)
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	return b.Bot.EditMessage(ctx, messageID, text)
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	return b.Bot.DeleteMessage(ctx, messageID)
}

// HandleWebhook handles an incoming webhook event
func (b *Bot) HandleWebhook(body []byte) error {
	return b.Bot.HandleWebhook(body)
}

// GetWebhookURL returns the webhook URL for Lark
func (b *Bot) GetWebhookURL(webhookPath string) string {
	return "/webhook/lark/" + webhookPath
}

// VerifyWebhook verifies webhook signature
func (b *Bot) VerifyWebhook(signature, timestamp, body string) bool {
	return b.Bot.VerifyWebhook(signature, timestamp, body)
}

// StartReceiving starts receiving events
func (b *Bot) StartReceiving(ctx context.Context) error {
	return b.Bot.StartReceiving(ctx)
}

// StopReceiving stops receiving events
func (b *Bot) StopReceiving(ctx context.Context) error {
	return b.Bot.StopReceiving(ctx)
}
