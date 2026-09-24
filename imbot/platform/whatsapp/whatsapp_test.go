package whatsapp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/core/coretest"
)

func newTestBot(t *testing.T) *Bot {
	t.Helper()
	bot, err := NewWhatsAppBot(&core.Config{
		Platform: core.PlatformWhatsApp,
		Auth:     core.AuthConfig{Type: "token", Token: "test-token"},
	})
	require.NoError(t, err)
	return bot
}

// TestHandleWebhook_CapturesReplyToContext guards inbound reply-to matching
// (.design/imbot-output.md §8): a WhatsApp quoted-reply webhook carries the
// replied-to message's ID in `messages[].context.id`. This must land on the
// emitted core.Message's ThreadContext.ParentMessageID, the same field every
// other platform's adapter populates, so bot.ReplyToMessageID reads it
// uniformly regardless of platform.
func TestHandleWebhook_CapturesReplyToContext(t *testing.T) {
	bot := newTestBot(t)

	ch := make(chan core.Message, 1)
	bot.OnMessage(func(msg core.Message) { ch <- msg })

	payload := []byte(`{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "entry-1",
			"changes": [{
				"field": "messages",
				"value": {
					"messaging_product": "whatsapp",
					"messages": [{
						"from": "15551234567",
						"id": "wamid.reply-1",
						"timestamp": "1700000000",
						"type": "text",
						"text": {"body": "y"},
						"context": {"id": "wamid.original-prompt"}
					}]
				}
			}]
		}]
	}`)

	require.NoError(t, bot.HandleWebhook(payload))
	got := coretest.WaitForMessage(t, ch)
	require.NotNil(t, got.ThreadContext, "reply-to context must produce a ThreadContext")
	assert.Equal(t, "wamid.original-prompt", got.ThreadContext.ParentMessageID)
}

// TestHandleWebhook_NoContextMeansNotAReply covers the non-reply case: a
// plain message (no "context" object) must not synthesize a ThreadContext,
// so bot.ReplyToMessageID correctly reads it as "not a reply".
func TestHandleWebhook_NoContextMeansNotAReply(t *testing.T) {
	bot := newTestBot(t)

	ch := make(chan core.Message, 1)
	bot.OnMessage(func(msg core.Message) { ch <- msg })

	payload := []byte(`{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "entry-1",
			"changes": [{
				"field": "messages",
				"value": {
					"messaging_product": "whatsapp",
					"messages": [{
						"from": "15551234567",
						"id": "wamid.fresh-1",
						"timestamp": "1700000000",
						"type": "text",
						"text": {"body": "hello"}
					}]
				}
			}]
		}]
	}`)

	require.NoError(t, bot.HandleWebhook(payload))
	got := coretest.WaitForMessage(t, ch)
	assert.Nil(t, got.ThreadContext, "a plain message must not get a synthesized ThreadContext")
}
