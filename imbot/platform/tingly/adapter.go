package tingly

import (
	"time"

	tgmodels "github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/core"
	itx "github.com/tingly-dev/tingly-box/imbot/interaction"
)

// decodeReplyMarkup extracts an inline keyboard from outbound message
// metadata. Production callers stuff one of two shapes into
// metadata["replyMarkup"]:
//
//   - imbot.InlineKeyboardMarkup (from the generic interaction package), or
//   - models.InlineKeyboardMarkup (Telegram-specific, returned by
//     imbot.BuildTelegramActionKeyboard, used widely by remote_control/bot).
//
// We accept both shapes so tingly faithfully captures keyboards regardless
// of which API the caller used.
func decodeReplyMarkup(meta map[string]interface{}) *Keyboard {
	if meta == nil {
		return nil
	}
	raw, ok := meta["replyMarkup"]
	if !ok {
		return nil
	}
	switch m := raw.(type) {
	case itx.InlineKeyboardMarkup:
		return convertGenericKeyboard(m)
	case *itx.InlineKeyboardMarkup:
		if m == nil {
			return nil
		}
		return convertGenericKeyboard(*m)
	case tgmodels.InlineKeyboardMarkup:
		return convertTelegramKeyboard(m)
	case *tgmodels.InlineKeyboardMarkup:
		if m == nil {
			return nil
		}
		return convertTelegramKeyboard(*m)
	}
	return nil
}

func convertGenericKeyboard(m itx.InlineKeyboardMarkup) *Keyboard {
	out := &Keyboard{Rows: make([][]Button, 0, len(m.InlineKeyboard))}
	for _, row := range m.InlineKeyboard {
		buttons := make([]Button, 0, len(row))
		for _, b := range row {
			buttons = append(buttons, Button{
				Label:        b.Text,
				CallbackData: b.CallbackData,
				URL:          b.URL,
			})
		}
		out.Rows = append(out.Rows, buttons)
	}
	return out
}

func convertTelegramKeyboard(m tgmodels.InlineKeyboardMarkup) *Keyboard {
	out := &Keyboard{Rows: make([][]Button, 0, len(m.InlineKeyboard))}
	for _, row := range m.InlineKeyboard {
		buttons := make([]Button, 0, len(row))
		for _, b := range row {
			buttons = append(buttons, Button{
				Label:        b.Text,
				CallbackData: b.CallbackData,
				URL:          b.URL,
			})
		}
		out.Rows = append(out.Rows, buttons)
	}
	return out
}

// NewIncomingTextMessage constructs an inbound text message. messageID is
// optional — tests usually pass "" and let the harness mint one.
func NewIncomingTextMessage(messageID, chatID string, sender core.Sender, text string, chatType core.ChatType) core.Message {
	return core.Message{
		ID:        messageID,
		Platform:  core.PlatformTingly,
		Timestamp: time.Now().Unix(),
		Sender:    sender,
		Recipient: core.Recipient{
			ID:   chatID,
			Type: recipientTypeFromChat(chatType),
		},
		Content:  core.NewTextContent(text),
		ChatType: chatType,
	}
}

// NewIncomingCallback constructs an inbound callback-query message. The
// shape mirrors imbot/platform/telegram/adapter.go: metadata carries
// is_callback / callback_data / callback_query_id, and Content is empty
// text. Production handlers (handler_message.go:38, telegram_callback.go,
// the generic interaction handler) all key off Metadata["is_callback"].
func NewIncomingCallback(messageID, chatID string, sender core.Sender, callbackData string, chatType core.ChatType) core.Message {
	return core.Message{
		ID:        messageID,
		Platform:  core.PlatformTingly,
		Timestamp: time.Now().Unix(),
		Sender:    sender,
		Recipient: core.Recipient{
			ID:   chatID,
			Type: recipientTypeFromChat(chatType),
		},
		Content:  core.NewTextContent(""),
		ChatType: chatType,
		Metadata: map[string]interface{}{
			"is_callback":       true,
			"callback_data":     callbackData,
			"callback_query_id": messageID,
		},
	}
}

func recipientTypeFromChat(c core.ChatType) string {
	switch c {
	case core.ChatTypeGroup:
		return "group"
	case core.ChatTypeChannel:
		return "channel"
	case core.ChatTypeThread:
		return "thread"
	default:
		return "user"
	}
}
