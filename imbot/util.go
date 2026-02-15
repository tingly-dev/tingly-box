package imbot

// Platform message limits
const (
	TelegramMessageLimit = 4096
	DiscordMessageLimit  = 2000
	DingTalkMessageLimit = 20480 // 20KB
	FeishuMessageLimit   = 30000
	DefaultMessageLimit  = 4000
)

// GetMessageLimit returns the message length limit for each platform.
func GetMessageLimit(platform Platform) int {
	switch platform {
	case PlatformTelegram:
		return TelegramMessageLimit
	case PlatformDiscord:
		return DiscordMessageLimit
	case PlatformDingTalk:
		return DingTalkMessageLimit
	case PlatformFeishu:
		return FeishuMessageLimit
	default:
		return DefaultMessageLimit
	}
}
