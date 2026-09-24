package weixin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/weixin/types"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

func newTestAdapter() *Adapter {
	return NewAdapter(&core.Config{Platform: core.PlatformWeixin}, nil)
}

// TestAdaptMessage_CapturesReplyToContext guards inbound reply-to matching
// (.design/imbot-output.md §8): Weixin's native quote/reply carries the
// replied-to message's ID in msg.ReplyToID. This must land on
// ThreadContext.ParentMessageID, the field bot.ReplyToMessageID reads
// regardless of platform.
func TestAdaptMessage_CapturesReplyToContext(t *testing.T) {
	a := newTestAdapter()

	msg := &types.Message{
		MessageID: "msg-reply-1",
		ChatType:  types.ChatTypeDirect,
		Timestamp: time.Now(),
		Text:      "y",
		From:      "bot-1",
		To:        "user-1",
		ReplyToID: "msg-original-prompt",
		Metadata:  map[string]interface{}{"session_id": "sess-1"},
	}

	got, err := a.AdaptMessage(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, got.ThreadContext, "a reply-to message must produce a ThreadContext")
	assert.Equal(t, "msg-original-prompt", got.ThreadContext.ParentMessageID)
}

// TestAdaptMessage_ReplyToIDSurvivesWithoutSessionID guards against a real
// regression: ThreadContext used to be gated on Metadata["session_id"]
// alone, so a message with a genuine ReplyToID but an empty WeChat
// customer-service session ID (confirmed possible — the upstream SDK's own
// message/monitor.go checks msg.SessionID != "" before using it) silently
// lost its reply-to signal. The two concerns are independent: session_id
// tracks the WeChat CS session, ReplyToID is the actual reply target.
func TestAdaptMessage_ReplyToIDSurvivesWithoutSessionID(t *testing.T) {
	a := newTestAdapter()

	msg := &types.Message{
		MessageID: "msg-reply-2",
		ChatType:  types.ChatTypeDirect,
		Timestamp: time.Now(),
		Text:      "y",
		From:      "bot-1",
		To:        "user-1",
		ReplyToID: "msg-original-prompt-2",
		Metadata:  map[string]interface{}{"session_id": ""},
	}

	got, err := a.AdaptMessage(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, got.ThreadContext, "ReplyToID alone must be enough to produce a ThreadContext")
	assert.Equal(t, "msg-original-prompt-2", got.ThreadContext.ParentMessageID)
}

// TestAdaptMessage_NoReplyMeansNoThreadContext covers the non-reply case: a
// plain message (no ReplyToID, no session_id) must not synthesize a
// ThreadContext.
func TestAdaptMessage_NoReplyMeansNoThreadContext(t *testing.T) {
	a := newTestAdapter()

	msg := &types.Message{
		MessageID: "msg-fresh-1",
		ChatType:  types.ChatTypeDirect,
		Timestamp: time.Now(),
		Text:      "hello",
		From:      "bot-1",
		To:        "user-1",
		Metadata:  map[string]interface{}{"session_id": ""},
	}

	got, err := a.AdaptMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, got.ThreadContext, "a plain message must not get a synthesized ThreadContext")
}
