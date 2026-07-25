package feishu

import (
	"context"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Card button presses arrive as their own callback event, separate from the
// message stream. Until this file existed the bot subscribed only to message
// receipt, so every card button it rendered was inert: the click produced a
// spinner on the user's side and nothing at all on ours.
//
// That is worse than having no buttons. Phase 2a fixed Feishu cards rendering
// without buttons; a button that renders and then does nothing looks to the
// user like a broken bot rather than a missing feature.

// handleCardActionTrigger converts a card button press into a core.Message and
// emits it on the same channel as ordinary messages, so consumers dispatch
// callbacks identically on every platform.
//
// The returned response is what Feishu shows the user immediately. Returning
// nil leaves the card as it is, which is right here: the handler downstream
// decides whether to restate the card, and guessing a toast would pre-empt it.
func (b *Bot) handleCardActionTrigger(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if event == nil || event.Event == nil || event.Event.Action == nil {
		return nil, nil
	}

	payload := payloadFromButtonValue(event.Event.Action.Value)
	if payload.IsEmpty() {
		b.Logger().Debug("Card action carried no recognisable payload: %v", event.Event.Action.Value)
		return nil, nil
	}

	var chatID, messageID string
	if event.Event.Context != nil {
		chatID = event.Event.Context.OpenChatID
		messageID = event.Event.Context.OpenMessageID
	}

	senderID := ""
	if op := event.Event.Operator; op != nil {
		// Prefer the tenant-global user_id when the app is scoped to see it,
		// matching what convertLarkMessageToCore does for ordinary messages so
		// a chat's sender identity does not change between the two paths.
		if op.UserID != nil && *op.UserID != "" {
			senderID = *op.UserID
		} else {
			senderID = op.OpenID
		}
	}

	msg := core.NewMessageBuilder(core.Platform(b.domain)).
		WithID(messageID).
		WithTimestamp(time.Now().Unix()).
		WithSender(senderID, "", "").
		WithRecipient(chatID, string(core.ChatTypeDirect), "").
		WithTextContent("", nil).
		WithPayload(payload).
		WithMetadata("callback_query_id", messageID).
		WithMetadata("message_id", messageID).
		WithMetadata("original_chat_id", chatID).
		Build()

	b.EmitMessage(*msg)
	b.UpdateLastActivity()
	return nil, nil
}
