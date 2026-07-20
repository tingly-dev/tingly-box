// Package lark provides Lark platform support as an alias to Feishu.
//
// Lark and Feishu are identical platforms except for the base URL:
//   - Feishu: https://open.feishu.cn
//   - Lark:   https://open.larksuite.com
//
// This package simply reuses the Feishu implementation with the Lark domain preset.
package lark

import (
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/feishu"
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

// PlatformInfo returns Lark platform information.
//
// All other core.Bot methods (Connect, SendMessage, React, StartReceiving, …)
// are promoted from the embedded *feishu.Bot and need no explicit override.
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(PlatformLark, "Lark")
}

// GetWebhookURL returns the webhook URL for Lark. This differs from the
// embedded Feishu implementation, which uses a "/webhook/feishu/" prefix.
func (b *Bot) GetWebhookURL(webhookPath string) string {
	return "/webhook/lark/" + webhookPath
}
