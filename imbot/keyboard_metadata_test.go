package imbot

import (
	"testing"

	tgmodels "github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

func sampleKeyboard() InlineKeyboardMarkup {
	return interaction.NewKeyboardBuilder().
		AddRow(interaction.CallbackButton("OK", "action:ok")).
		Build()
}

func TestBuildKeyboardMetadata_Feishu(t *testing.T) {
	for _, p := range []Platform{PlatformFeishu, PlatformLark} {
		md := BuildKeyboardMetadata(p, sampleKeyboard())
		rm, ok := md["replyMarkup"]
		if !ok {
			t.Fatalf("%s: expected replyMarkup key", p)
		}
		kb, ok := rm.(InlineKeyboardMarkup)
		if !ok {
			t.Fatalf("%s: expected neutral InlineKeyboardMarkup, got %T", p, rm)
		}
		if len(kb.InlineKeyboard) != 1 || kb.InlineKeyboard[0][0].CallbackData != "action:ok" {
			t.Errorf("%s: keyboard not preserved: %+v", p, kb)
		}
	}
}

func TestBuildKeyboardMetadata_TelegramAndDefault(t *testing.T) {
	for _, p := range []Platform{PlatformTelegram, Platform("unknown")} {
		md := BuildKeyboardMetadata(p, sampleKeyboard())
		rm, ok := md["replyMarkup"]
		if !ok {
			t.Fatalf("%s: expected replyMarkup key", p)
		}
		if _, ok := rm.(tgmodels.InlineKeyboardMarkup); !ok {
			t.Fatalf("%s: expected telegram InlineKeyboardMarkup, got %T", p, rm)
		}
	}
}
