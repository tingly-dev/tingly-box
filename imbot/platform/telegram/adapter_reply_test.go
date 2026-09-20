package telegram

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// TestAdaptMessage_CapturesReplyToContext guards inbound reply-to matching
// (.design/imbot-output.md §8): Telegram's native "reply to message" carries
// the replied-to message's ID in msg.ReplyToMessage.ID. This must land on
// the emitted core.Message's ThreadContext.ParentMessageID, the field
// bot.ReplyToMessageID reads regardless of platform.
func TestAdaptMessage_CapturesReplyToContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformTelegram}, nil)

	msg := &models.Message{
		ID:   102,
		Date: 1700000000,
		Chat: models.Chat{ID: 555, Type: models.ChatTypePrivate},
		From: &models.User{ID: 1, FirstName: "Tester"},
		Text: "y",
		ReplyToMessage: &models.Message{
			ID: 101,
		},
	}

	got, err := a.AdaptMessage(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, got.ThreadContext, "a reply-to message must produce a ThreadContext")
	assert.Equal(t, "101", got.ThreadContext.ParentMessageID)
}

// TestAdaptMessage_NoReplyMeansNoThreadContext covers the non-reply case: a
// plain message (no ReplyToMessage) must not synthesize a ThreadContext, so
// bot.ReplyToMessageID correctly reads it as "not a reply".
func TestAdaptMessage_NoReplyMeansNoThreadContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformTelegram}, nil)

	msg := &models.Message{
		ID:   103,
		Date: 1700000000,
		Chat: models.Chat{ID: 555, Type: models.ChatTypePrivate},
		From: &models.User{ID: 1, FirstName: "Tester"},
		Text: "hello",
	}

	got, err := a.AdaptMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, got.ThreadContext, "a plain message must not get a synthesized ThreadContext")
}
