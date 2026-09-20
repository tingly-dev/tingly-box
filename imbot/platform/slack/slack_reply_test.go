package slack

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// newTestBot builds a Bot whose client points at a local stub server
// instead of the real Slack API. handleMessage looks up user/channel info
// via the API purely for cosmetic enrichment (display names); the stub
// always answers with an error response, which handleMessage already
// tolerates (see the `err == nil && ...` guards), so it exercises the real
// message-building path — including ThreadContext — with no network call
// leaving the sandbox.
func newTestBot(t *testing.T) *Bot {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"not_stubbed"}`))
	}))
	t.Cleanup(server.Close)

	return &Bot{
		BaseBot: core.NewBaseBot(&core.Config{Platform: core.PlatformSlack}),
		client:  slack.New("test-token", slack.OptionAPIURL(server.URL+"/")),
	}
}

func waitForMessage(t *testing.T, ch <-chan core.Message) core.Message {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnMessage to fire")
		return core.Message{}
	}
}

// TestHandleMessage_CapturesReplyToContext guards inbound reply-to matching
// (.design/imbot-output.md §8): Slack surfaces a reply via
// event.ThreadTimestamp — the timestamp of the thread's ROOT message, not
// necessarily the exact message being replied to (Slack threads don't have
// per-reply parent pointers). This must land on
// ThreadContext.ParentMessageID, the field bot.ReplyToMessageID reads
// regardless of platform.
func TestHandleMessage_CapturesReplyToContext(t *testing.T) {
	bot := newTestBot(t)

	ch := make(chan core.Message, 1)
	bot.OnMessage(func(msg core.Message) { ch <- msg })

	bot.handleMessage(&slack.MessageEvent{
		Msg: slack.Msg{
			Channel:         "C123",
			User:            "U1",
			Text:            "y",
			Timestamp:       "1700000000.000200",
			ThreadTimestamp: "1700000000.000100",
		},
	})

	got := waitForMessage(t, ch)
	require.NotNil(t, got.ThreadContext, "a threaded reply must produce a ThreadContext")
	assert.Equal(t, "1700000000.000100", got.ThreadContext.ParentMessageID)
}

// TestHandleMessage_NoThreadMeansNoThreadContext covers the non-reply case:
// a plain top-level message (no ThreadTimestamp) must not synthesize a
// ThreadContext.
func TestHandleMessage_NoThreadMeansNoThreadContext(t *testing.T) {
	bot := newTestBot(t)

	ch := make(chan core.Message, 1)
	bot.OnMessage(func(msg core.Message) { ch <- msg })

	bot.handleMessage(&slack.MessageEvent{
		Msg: slack.Msg{
			Channel:   "C123",
			User:      "U1",
			Text:      "hello",
			Timestamp: "1700000000.000200",
		},
	})

	got := waitForMessage(t, ch)
	assert.Nil(t, got.ThreadContext, "a plain message must not get a synthesized ThreadContext")
}
