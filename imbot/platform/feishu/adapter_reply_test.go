package feishu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// TestAdaptMessage_CapturesReplyToContext guards inbound reply-to matching
// (.design/imbot-output.md §8): Feishu/Lark's native reply carries the
// replied-to message's ID in event.ParentID. This must land on
// ThreadContext.ParentMessageID, the field bot.ReplyToMessageID reads
// regardless of platform. lark.Bot embeds *feishu.Bot and inherits this
// adapter unchanged, so this test covers both platforms.
func TestAdaptMessage_CapturesReplyToContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformFeishu})

	event := MessageEventDetail{
		MessageID:  "om_reply_1",
		CreateTime: "1700000000000",
		ChatType:   "p2p",
		MsgType:    "text",
		Content:    map[string]interface{}{"text": "y"},
		Sender:     SenderDetail{SenderID: "ou_sender"},
		ChatID:     "oc_chat_1",
		ParentID:   "om_original_prompt",
	}

	got, err := a.AdaptMessage(context.Background(), event)
	require.NoError(t, err)
	require.NotNil(t, got.ThreadContext, "a reply-to message must produce a ThreadContext")
	assert.Equal(t, "om_original_prompt", got.ThreadContext.ParentMessageID)
}

// TestAdaptMessage_NoParentIDMeansNoThreadContext covers the non-reply
// case: a plain message (nil ParentID) must not synthesize a ThreadContext.
func TestAdaptMessage_NoParentIDMeansNoThreadContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformFeishu})

	event := MessageEventDetail{
		MessageID:  "om_fresh_1",
		CreateTime: "1700000000000",
		ChatType:   "p2p",
		MsgType:    "text",
		Content:    map[string]interface{}{"text": "hello"},
		Sender:     SenderDetail{SenderID: "ou_sender"},
		ChatID:     "oc_chat_1",
	}

	got, err := a.AdaptMessage(context.Background(), event)
	require.NoError(t, err)
	assert.Nil(t, got.ThreadContext, "a plain message must not get a synthesized ThreadContext")
}
