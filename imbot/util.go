package imbot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

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

// BuildTelegramActionKeyboard converts imbot.InlineKeyboardMarkup to tgbotapi.InlineKeyboardMarkup
func BuildTelegramActionKeyboard(kb InlineKeyboardMarkup) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, row := range kb.InlineKeyboard {
		var buttons []tgbotapi.InlineKeyboardButton
		for _, btn := range row {
			tgBtn := tgbotapi.InlineKeyboardButton{
				Text: btn.Text,
			}
			if btn.CallbackData != "" {
				tgBtn.CallbackData = &btn.CallbackData
			}
			if btn.URL != "" {
				tgBtn.URL = &btn.URL
			}
			buttons = append(buttons, tgBtn)
		}
		rows = append(rows, buttons)
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}
