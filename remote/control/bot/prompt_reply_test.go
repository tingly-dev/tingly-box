package bot_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/control/ask"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// TestSelectPendingRequest_ReplyToWinsOverTieBreak guards the core promise
// of reply-to matching: a native platform reply that targets an OLDER,
// lower-priority (notify) request must still win over a newer,
// higher-priority (remote_agent) one that GetPendingRequestsForChat's
// Source/recency tie-break would otherwise pick — because the reply-to is a
// real match, not a guess. See .design/imbot-output.md §8.
func TestSelectPendingRequest_ReplyToWinsOverTieBreak(t *testing.T) {
	// Already in GetPendingRequestsForChat's tie-break order: remote_agent
	// (preferred by the heuristic) first, notify second.
	pendingReqs := []ask.Request{
		{ID: "req-agent", Source: ask.SourceRemoteAgent, MessageID: "msg-agent"},
		{ID: "req-notify", Source: ask.SourceNotify, MessageID: "msg-notify"},
	}

	got := bot.SelectPendingRequest(pendingReqs, "msg-notify")
	assert.Equal(t, "req-notify", got.ID, "a reply-to match must win even against a higher-priority candidate")
}

// TestSelectPendingRequest_FallsBackWithoutReplyTo covers the two cases
// where there's nothing to match against: no reply-to signal at all, and a
// reply-to that doesn't match any currently pending request (e.g. a reply
// to an already-resolved prompt). Both must fall back to the tie-break's
// own pick, pendingReqs[0].
func TestSelectPendingRequest_FallsBackWithoutReplyTo(t *testing.T) {
	pendingReqs := []ask.Request{
		{ID: "req-first", MessageID: "msg-first"},
		{ID: "req-second", MessageID: "msg-second"},
	}

	t.Run("no reply-to at all", func(t *testing.T) {
		got := bot.SelectPendingRequest(pendingReqs, "")
		assert.Equal(t, "req-first", got.ID)
	})

	t.Run("reply-to matches nothing pending", func(t *testing.T) {
		got := bot.SelectPendingRequest(pendingReqs, "msg-some-other-resolved-prompt")
		assert.Equal(t, "req-first", got.ID)
	})
}

// TestReplyToMessageID mirrors the three shapes callers hand it: no thread
// context at all (platforms with no native reply-to, or a plain message),
// a thread context with no parent (Slack's ThreadTimestamp populates its
// own ID/ParentMessageID pair even for a fresh, non-reply message on some
// platforms, so an empty ParentMessageID must still read as "not a reply"),
// and a populated one.
func TestReplyToMessageID(t *testing.T) {
	t.Run("no thread context", func(t *testing.T) {
		msg := imbot.Message{}
		assert.Equal(t, "", bot.ReplyToMessageID(msg))
	})

	t.Run("thread context with no parent", func(t *testing.T) {
		msg := imbot.Message{ThreadContext: &imbot.ThreadContext{ID: "thread-1"}}
		assert.Equal(t, "", bot.ReplyToMessageID(msg))
	})

	t.Run("populated parent", func(t *testing.T) {
		msg := imbot.Message{ThreadContext: &imbot.ThreadContext{ID: "thread-1", ParentMessageID: "msg-42"}}
		assert.Equal(t, "msg-42", bot.ReplyToMessageID(msg))
	})
}
