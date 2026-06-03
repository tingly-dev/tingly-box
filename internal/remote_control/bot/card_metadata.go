package bot

import (
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
)

const trackActionMenuIDKey = "_trackActionMenuID"

// buildReplyMetadata builds keyboard metadata in the form the target platform's send
// path understands (neutral keyboard for Feishu/Lark, Telegram struct otherwise).
func buildReplyMetadata(platform imbot.Platform, kb imbot.InlineKeyboardMarkup) map[string]interface{} {
	return imbot.BuildKeyboardMetadata(platform, kb)
}

func buildTrackedReplyMetadata(platform imbot.Platform, kb imbot.InlineKeyboardMarkup) map[string]interface{} {
	metadata := buildReplyMetadata(platform, kb)
	metadata[trackActionMenuIDKey] = true
	return metadata
}

// buildActionCardMetadata builds metadata for an action card (keyboard + card payload).
func buildActionCardMetadata(platform imbot.Platform, kb imbot.InlineKeyboardMarkup, card imbot.Card) map[string]interface{} {
	metadata := buildReplyMetadata(platform, kb)
	metadata["card"] = card
	return metadata
}

// buildActionMenuMetadata builds metadata for action menu with platform-specific card rendering
func buildActionMenuMetadata(hCtx HandlerContext, kb imbot.InlineKeyboardMarkup, card imbot.Card) map[string]interface{} {
	metadata := buildActionCardMetadata(hCtx.Platform, kb, card)

	// For Feishu/Lark, add card_json so the richer card (title/sections/fields) renders.
	if hCtx.Platform == imbot.PlatformFeishu || hCtx.Platform == imbot.PlatformLark {
		renderer := feature.NewFeishuCardRenderer()
		if cardJSON, err := renderer.Render(card); err == nil {
			metadata["card_json"] = cardJSON
		}
	}

	return metadata
}

func (h *BotHandler) buildTrackedActionMenuMetadata(hCtx HandlerContext, kb imbot.InlineKeyboardMarkup, card imbot.Card) map[string]interface{} {
	return buildTrackedActionMenuMetadata(hCtx, kb, card)
}

func buildTrackedActionMenuMetadata(hCtx HandlerContext, kb imbot.InlineKeyboardMarkup, card imbot.Card) map[string]interface{} {
	metadata := buildActionMenuMetadata(hCtx, kb, card)
	metadata[trackActionMenuIDKey] = true
	return metadata
}
