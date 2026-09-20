package discord

import (
	"context"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// TestAdaptMessage_CapturesReplyToContext guards inbound reply-to matching
// (.design/imbot-output.md §8): Discord's native "reply" carries the
// replied-to message's ID in msg.MessageReference.MessageID (populated by
// the gateway only for reply/thread-starter messages). This must land on
// ThreadContext.ParentMessageID, the field bot.ReplyToMessageID reads
// regardless of platform.
//
// discordgo.Message also exposes a Reference() method, but per its own
// doc comment that method builds a reference TO this message (for use when
// someone else replies to it) — it is NOT the inbound "this message is
// replying to X" signal, and always echoes the message's own ID. Using it
// here would make every message appear to reply to itself.
func TestAdaptMessage_CapturesReplyToContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformDiscord}, nil)

	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-reply-1",
			ChannelID: "chan-1",
			Content:   "y",
			Timestamp: time.Now(),
			Author:    &discordgo.User{ID: "user-1", Username: "tester"},
			Type:      discordgo.MessageTypeReply,
			MessageReference: &discordgo.MessageReference{
				MessageID: "msg-original-prompt",
				ChannelID: "chan-1",
			},
		},
	}

	got, err := a.AdaptMessage(context.Background(), m)
	require.NoError(t, err)
	require.NotNil(t, got.ThreadContext, "a Discord reply must produce a ThreadContext")
	assert.Equal(t, "msg-original-prompt", got.ThreadContext.ParentMessageID,
		"ParentMessageID must be the message being replied to, not this message's own ID")
}

// TestAdaptMessage_NoReferenceMeansNoThreadContext covers the non-reply
// case: a plain message (no MessageReference) must not synthesize a
// ThreadContext.
func TestAdaptMessage_NoReferenceMeansNoThreadContext(t *testing.T) {
	a := NewAdapter(&core.Config{Platform: core.PlatformDiscord}, nil)

	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-fresh-1",
			ChannelID: "chan-1",
			Content:   "hello",
			Timestamp: time.Now(),
			Author:    &discordgo.User{ID: "user-1", Username: "tester"},
		},
	}

	got, err := a.AdaptMessage(context.Background(), m)
	require.NoError(t, err)
	assert.Nil(t, got.ThreadContext, "a plain message must not get a synthesized ThreadContext")
}
