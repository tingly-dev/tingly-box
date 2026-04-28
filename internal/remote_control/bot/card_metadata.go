package bot

import (
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
)

// buildActionMenuMetadata builds metadata for action menu with platform-specific card rendering
func buildActionMenuMetadata(hCtx HandlerContext, tgKeyboard interface{}, card imbot.Card) map[string]interface{} {
	metadata := map[string]interface{}{
		"replyMarkup": tgKeyboard,
		"card":        card,
	}

	// For Feishu/Lark, add card_json
	if hCtx.Platform == imbot.PlatformFeishu || hCtx.Platform == imbot.PlatformLark {
		renderer := feature.NewFeishuCardRenderer()
		if cardJSON, err := renderer.Render(card); err == nil {
			metadata["card_json"] = cardJSON
		}
	}

	return metadata
}
