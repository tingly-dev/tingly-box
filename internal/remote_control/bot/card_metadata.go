package bot

import (
	"github.com/tingly-dev/tingly-box/imbot"
	imbotfeishu "github.com/tingly-dev/tingly-box/imbot/platform/feishu"
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

	// For Feishu/Lark, add card_json.
	//
	// TODO(phase-2a): this whole branch goes away. The caller should not be
	// rendering a platform's wire format at all — it should hand over a
	// neutral Card and let the platform render it. Note that today nothing on
	// the Feishu send path reads metadata["card_json"] (only feishu/menu.go
	// does, and remote_control never goes through the menu adapter), so this
	// is already inert; see .sdlc/research/arch-remote-control-platform-seams §3.1.
	if hCtx.Platform == imbot.PlatformFeishu || hCtx.Platform == imbot.PlatformLark {
		if cardJSON, err := imbotfeishu.RenderCard(card); err == nil {
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
