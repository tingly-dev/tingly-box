package bot

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	imbot "github.com/tingly-dev/tingly-box/imbot/core"
)

// captureBot records every SendMessage call. The other Bot methods are
// inherited from stubBot (no-ops) and are sufficient for the streaming
// handler tests, which only exercise SendMessage.
type captureBot struct {
	stubBot
	mu   sync.Mutex
	sent []string
}

func (b *captureBot) SendMessage(ctx context.Context, target string, opts *imbot.SendMessageOptions) (*imbot.SendResult, error) {
	b.mu.Lock()
	b.sent = append(b.sent, opts.Text)
	b.mu.Unlock()
	return &imbot.SendResult{MessageID: "msg"}, nil
}

func (b *captureBot) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.sent))
	copy(out, b.sent)
	return out
}

func assistantText(text string) *claude.AssistantMessage {
	return &claude.AssistantMessage{
		Type: claude.SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: claude.ContentBlockTypeText, Text: text},
			},
		},
	}
}

func toolUseMsg(name, id string, input map[string]interface{}) *claude.ToolUseMessage {
	return &claude.ToolUseMessage{
		Type:      claude.SDKToolUseMessage,
		Name:      name,
		ToolUseID: id,
		Input:     input,
	}
}

func TestStreamingHandler_BuffersToolsUntilTextFlush_Verbose(t *testing.T) {
	bot := &captureBot{}
	meta := &ResponseMeta{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", true, meta)

	require.NoError(t, h.OnMessage(toolUseMsg("Read", "id-1", map[string]interface{}{"file_path": "/a/b.go"})))
	require.NoError(t, h.OnMessage(toolUseMsg("Bash", "id-2", map[string]interface{}{"command": "ls"})))

	assert.Empty(t, bot.snapshot(), "tool-only messages should be buffered, not sent immediately")

	require.NoError(t, h.OnMessage(assistantText("All done reading.")))

	sent := bot.snapshot()
	require.Len(t, sent, 2, "expected one aggregated tool message followed by the text message")
	// Both tool renders must end up in the same aggregated message.
	assert.Contains(t, sent[0], "b.go", "Read render should appear in the aggregated message")
	assert.Contains(t, sent[0], "ls", "Bash render should appear in the aggregated message")
	assert.Contains(t, sent[1], "All done reading.")
}

func TestStreamingHandler_QuietFlushRendersSummary(t *testing.T) {
	bot := &captureBot{}
	meta := &ResponseMeta{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", false, meta)

	for i := 0; i < 5; i++ {
		require.NoError(t, h.OnMessage(toolUseMsg("Read", "id", map[string]interface{}{
			"file_path": "/x/y.go",
		})))
	}
	assert.Empty(t, bot.snapshot(), "tool buffer should hold messages until a text-bearing message arrives")

	require.NoError(t, h.OnMessage(assistantText("Summary text.")))

	sent := bot.snapshot()
	require.Len(t, sent, 2)
	assert.Contains(t, sent[0], "5 tool call(s)", "quiet mode should aggregate to a count")
	assert.Contains(t, sent[0], "(+2 more)", "quiet mode should fold extras beyond the preview")
	assert.Contains(t, sent[1], "Summary text.")
}

func TestStreamingHandler_FlushOnThreshold(t *testing.T) {
	bot := &captureBot{}
	meta := &ResponseMeta{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", true, meta)

	for i := 0; i < toolBufferFlushThreshold; i++ {
		require.NoError(t, h.OnMessage(toolUseMsg("Bash", "id", map[string]interface{}{"command": "echo"})))
	}
	sent := bot.snapshot()
	require.Len(t, sent, 1, "buffer should auto-flush at the threshold")
	assert.NotEmpty(t, sent[0])
}

func TestStreamingHandler_AssistantWithTextDoesNotBuffer(t *testing.T) {
	bot := &captureBot{}
	meta := &ResponseMeta{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", true, meta)

	// AssistantMessage with both text and tool_use blocks is text-bearing:
	// the formatter groups text + tools into a single render, so it must
	// not be diverted into the buffer.
	mixed := &claude.AssistantMessage{
		Type: claude.SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: claude.ContentBlockTypeText, Text: "Looking up file..."},
				{Type: claude.ContentBlockTypeToolUse, ID: "id-1", Name: "Read"},
			},
		},
	}
	require.NoError(t, h.OnMessage(mixed))

	sent := bot.snapshot()
	require.Len(t, sent, 1, "text-bearing assistant message should send immediately")
	assert.Contains(t, sent[0], "Looking up file...")
}

func TestStreamingHandler_OnErrorFlushesBuffer(t *testing.T) {
	bot := &captureBot{}
	meta := &ResponseMeta{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", true, meta)

	require.NoError(t, h.OnMessage(toolUseMsg("Read", "id-1", map[string]interface{}{"file_path": "/a.go"})))
	h.OnError(errors.New("boom"))

	sent := bot.snapshot()
	require.Len(t, sent, 2, "OnError should flush buffered tools, then send the error message")
	assert.Contains(t, sent[0], "a.go", "buffered tool render must surface before the error")
	assert.Contains(t, sent[1], "boom")
}

// TestStreamingHandler_QuietSuppressedMessageStillFlushesBuffer guards the
// "messages are the splitting boundary" invariant: even when a text-bearing
// claude message would itself be suppressed by the quiet filter (e.g. a
// UserMessage echo), the act of receiving it must still flush the buffered
// tool renders so they don't pile up across boundaries.
func TestStreamingHandler_QuietSuppressedMessageStillFlushesBuffer(t *testing.T) {
	bot := &captureBot{}
	meta := &ResponseMeta{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", false, meta)

	require.NoError(t, h.OnMessage(toolUseMsg("Read", "id-1", map[string]interface{}{"file_path": "/a.go"})))
	require.NoError(t, h.OnMessage(toolUseMsg("Read", "id-2", map[string]interface{}{"file_path": "/b.go"})))
	assert.Empty(t, bot.snapshot(), "tools should be buffered before any text-bearing message")

	// UserMessage is text-bearing but quiet mode drops it; the flush must
	// still happen so the boundary is honored.
	require.NoError(t, h.OnMessage(&claude.UserMessage{
		Type:    claude.SDKUserMessage,
		Message: "ignored in quiet mode",
	}))

	sent := bot.snapshot()
	require.Len(t, sent, 1, "buffer should flush even though the user message itself is suppressed")
	assert.Contains(t, sent[0], "2 tool call(s)")
}

// --- Map-path handler tests (handleMapMessage). This is the production path
// for the smart-guide agent, which streams status/assistant frames as plain
// maps; tool aggregation must work here too, not only on the claude stream.

func TestHandleMapMessage_AggregatesToolEvents(t *testing.T) {
	bot := &captureBot{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", true, &ResponseMeta{})

	// Buffer two tool events (mix top-level fields and nested data) ...
	require.NoError(t, h.OnMessage(map[string]interface{}{
		"type":      agentboot.EventTypeToolUse,
		"tool_name": "Read",
		"input":     map[string]interface{}{"file_path": "/a.go"},
	}))
	require.NoError(t, h.OnMessage(map[string]interface{}{
		"type": agentboot.EventTypeToolResult,
		"data": map[string]interface{}{
			"tool_name": "Read",
			"is_error":  true,
		},
	}))
	assert.Empty(t, bot.snapshot(), "tool events on the map path should buffer")

	// ... and confirm an assistant map flushes them.
	require.NoError(t, h.OnMessage(map[string]interface{}{
		"type":    agentboot.EventTypeAssistant,
		"message": "ok",
	}))

	sent := bot.snapshot()
	require.Len(t, sent, 2)
	assert.Contains(t, sent[0], "Read")
	assert.Contains(t, sent[0], "error", "tool_result with is_error=true should surface as error")
	assert.Contains(t, sent[1], "ok")
}

// TestBufferToolEvent_Dispatcher asserts the dispatcher buffers tool events
// and ignores non-tool types, and that sendText flushes the buffer.
func TestBufferToolEvent_Dispatcher(t *testing.T) {
	bot := &captureBot{}
	h := newStreamingMessageHandler(bot, "chat-1", "reply-1", true, &ResponseMeta{})

	nestedFields := toolFieldsFromNestedMap(map[string]interface{}{
		"data": map[string]interface{}{
			"tool_name": "Bash",
			"input":     map[string]interface{}{"command": "echo hi"},
		},
	})

	h.mu.Lock()
	require.True(t, h.bufferToolEvent(agentboot.EventTypeToolUse, nestedFields))
	require.True(t, h.bufferToolEvent(agentboot.EventTypeToolUse, nestedFields))
	// Non-tool types are not handled by the dispatcher.
	require.False(t, h.bufferToolEvent(agentboot.EventTypeAssistant, nestedFields))
	h.mu.Unlock()
	assert.Empty(t, bot.snapshot(), "buffered tool events must not send yet")

	// sendText flushes the buffer, then sends the text. Empty text is a no-op.
	h.mu.Lock()
	h.sendText("   ")
	h.mu.Unlock()
	assert.Empty(t, bot.snapshot(), "sendText(empty) must be a no-op")

	h.mu.Lock()
	h.sendText("done")
	h.mu.Unlock()
	sent := bot.snapshot()
	require.Len(t, sent, 2)
	// Aggregated tool message contains both Bash entries.
	assert.Contains(t, sent[0], "Bash")
	assert.Equal(t, "done", sent[1])
}

func TestBriefInputHint_PicksKnownKeys(t *testing.T) {
	assert.Equal(t, "ls -la", briefInputHint(map[string]interface{}{"command": "ls -la"}))
	assert.Equal(t, "/a.go", briefInputHint(map[string]interface{}{"file_path": "/a.go"}))
	assert.Equal(t, "", briefInputHint(nil))
	assert.Equal(t, "", briefInputHint(map[string]interface{}{"unrelated": 1}))

	// Long ASCII values truncate to <=80 runes with an ellipsis.
	got := briefInputHint(map[string]interface{}{"command": strings.Repeat("x", 100)})
	assert.LessOrEqual(t, utf8.RuneCountInString(got), maxToolHintLen)
	assert.Contains(t, got, "…")

	// Multibyte values truncate on a rune boundary — the result must stay
	// valid UTF-8, never sliced mid-rune.
	gotCJK := briefInputHint(map[string]interface{}{"command": strings.Repeat("查", 100)})
	assert.LessOrEqual(t, utf8.RuneCountInString(gotCJK), maxToolHintLen)
	assert.True(t, utf8.ValidString(gotCJK), "truncated hint must be valid UTF-8")
}

func TestIsToolOnlyClaudeMessage(t *testing.T) {
	cases := []struct {
		name string
		msg  claude.Message
		want bool
	}{
		{"tool_use", &claude.ToolUseMessage{Type: claude.SDKToolUseMessage}, true},
		{"tool_result", &claude.ToolResultMessage{Type: claude.SDKToolResultMessage}, true},
		{"assistant_text", assistantText("hi"), false},
		{"assistant_tool_only", &claude.AssistantMessage{
			Type: claude.SDKAssistantMessage,
			Message: anthropic.Message{Content: []anthropic.ContentBlockUnion{
				{Type: claude.ContentBlockTypeToolUse, ID: "x"},
			}},
		}, true},
		{"assistant_whitespace_text_then_tool", &claude.AssistantMessage{
			Type: claude.SDKAssistantMessage,
			Message: anthropic.Message{Content: []anthropic.ContentBlockUnion{
				{Type: claude.ContentBlockTypeText, Text: "   "},
				{Type: claude.ContentBlockTypeToolUse, ID: "x"},
			}},
		}, true},
		{"user", &claude.UserMessage{Type: claude.SDKUserMessage, Message: "hi"}, false},
		{"result", &claude.ResultMessage{Type: claude.SDKResultMessage}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isToolOnlyClaudeMessage(tc.msg))
		})
	}
}
