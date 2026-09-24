package wecom

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/weixin/types"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// TestAdaptMessage_CapturesReplyToContext guards inbound reply-to matching
// (.design/imbot-output.md §8): WeCom's native quote/reply carries the
// replied-to message's ID in msg.ReplyToID (shared types.Message with
// Weixin, translated upstream by the SDK). This must land on
// ThreadContext.ParentMessageID, the field bot.ReplyToMessageID reads
// regardless of platform. Unlike Weixin's adapter, WeCom's gates
// ThreadContext on ReplyToID alone, with no session_id coupling.
func TestAdaptMessage_CapturesReplyToContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformWecom})

	msg := &types.Message{
		MessageID: "msg-reply-1",
		ChatType:  types.ChatTypeDirect,
		Timestamp: time.Now(),
		Text:      "y",
		SenderID:  "user-1",
		To:        "bot-1",
		ReplyToID: "msg-original-prompt",
	}

	got, err := a.AdaptMessage(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, got.ThreadContext, "a reply-to message must produce a ThreadContext")
	assert.Equal(t, "msg-original-prompt", got.ThreadContext.ParentMessageID)
}

// TestAdaptMessage_NoReplyMeansNoThreadContext covers the non-reply case: a
// plain message (no ReplyToID) must not synthesize a ThreadContext.
func TestAdaptMessage_NoReplyMeansNoThreadContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformWecom})

	msg := &types.Message{
		MessageID: "msg-fresh-1",
		ChatType:  types.ChatTypeDirect,
		Timestamp: time.Now(),
		Text:      "hello",
		SenderID:  "user-1",
		To:        "bot-1",
	}

	got, err := a.AdaptMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, got.ThreadContext, "a plain message must not get a synthesized ThreadContext")
}
