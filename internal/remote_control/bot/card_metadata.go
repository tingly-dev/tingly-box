package bot

import (
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
)

const trackActionMenuIDKey = "_trackActionMenuID"

func buildReplyMetadata(tgKeyboard interface{}) map[string]interface{} {
	return map[string]interface{}{
		"replyMarkup": tgKeyboard,
	}
}

func buildTrackedReplyMetadata(tgKeyboard interface{}) map[string]interface{} {
	metadata := buildReplyMetadata(tgKeyboard)
	metadata[trackActionMenuIDKey] = true
	return metadata
}

// buildActionMenuMetadata builds metadata for action menu with platform-specific card rendering
func buildActionCardMetadata(tgKeyboard interface{}, card imbot.Card) map[string]interface{} {
	metadata := buildReplyMetadata(tgKeyboard)
	metadata["card"] = card
	return metadata
}

func buildActionMenuMetadata(hCtx HandlerContext, tgKeyboard interface{}, card imbot.Card) map[string]interface{} {
	metadata := buildActionCardMetadata(tgKeyboard, card)

	// For Feishu/Lark, add card_json
	if hCtx.Platform == imbot.PlatformFeishu || hCtx.Platform == imbot.PlatformLark {
		renderer := feature.NewFeishuCardRenderer()
		if cardJSON, err := renderer.Render(card); err == nil {
			metadata["card_json"] = cardJSON
		}
	}

	return metadata
}

func (h *BotHandler) buildTrackedActionMenuMetadata(hCtx HandlerContext, tgKeyboard interface{}, card imbot.Card) map[string]interface{} {
	return buildTrackedActionMenuMetadata(hCtx, tgKeyboard, card)
}

func buildTrackedActionMenuMetadata(hCtx HandlerContext, tgKeyboard interface{}, card imbot.Card) map[string]interface{} {
	metadata := buildActionMenuMetadata(hCtx, tgKeyboard, card)
	metadata[trackActionMenuIDKey] = true
	return metadata
}
