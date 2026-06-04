package stream

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// chunkSSE marshals each chunk as an OpenAI Chat Completions SSE frame and
// terminates the stream with the conventional [DONE] sentinel.
func chunkSSE(t *testing.T, chunks []map[string]any) string {
	t.Helper()
	var b strings.Builder
	for _, ch := range chunks {
		raw, err := json.Marshal(ch)
		require.NoError(t, err)
		b.WriteString("data: ")
		b.Write(raw)
		b.WriteString("\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// newChatChunkStream builds a real *ssestream.Stream[openai.ChatCompletionChunk]
// from synthetic SSE bytes, so the converter can be driven end-to-end offline
// (no OPENAI_API_KEY, no network) through its actual Next() path.
func newChatChunkStream(sse string) *ssestream.Stream[openai.ChatCompletionChunk] {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   io.NopCloser(strings.NewReader(sse)),
	}
	return ssestream.NewStream[openai.ChatCompletionChunk](ssestream.NewDecoder(resp), nil)
}

// TestChatToResponsesConverter_GoldenSequence is a hermetic, deterministic
// regression oracle for the ChatToResponses converter. It feeds a realistic
// multi-chunk conversation (interleaved text + a tool call assembled across
// two chunks, then a terminal finish_reason+usage chunk) through the full
// Next() iterator and asserts the exact ordered sequence of emitted Responses
// API events plus key payload fields and final usage.
//
// Unlike TestHandleOpenAIChatToResponsesStream_* (which require a live
// OPENAI_API_KEY and are skipped in CI), this test runs entirely offline and
// therefore actually executes on every run.
func TestChatToResponsesConverter_GoldenSequence(t *testing.T) {
	chunks := []map[string]any{
		{ // 1: role + first text delta -> created, item.added(text), text.delta
			"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 1, "model": "gpt-4o-mini",
			"choices": []any{map[string]any{
				"index": 0,
				"delta": map[string]any{"role": "assistant", "content": "Hello"},
			}},
		},
		{ // 2: second text delta -> text.delta
			"choices": []any{map[string]any{
				"index": 0,
				"delta": map[string]any{"content": ", World!"},
			}},
		},
		{ // 3: tool call opens + first args fragment -> item.added(fn), args.delta
			"choices": []any{map[string]any{
				"index": 0,
				"delta": map[string]any{"tool_calls": []any{map[string]any{
					"index": 0, "id": "call_1", "type": "function",
					"function": map[string]any{"name": "get_weather", "arguments": `{"city":`},
				}}},
			}},
		},
		{ // 4: more args -> args.delta
			"choices": []any{map[string]any{
				"index": 0,
				"delta": map[string]any{"tool_calls": []any{map[string]any{
					"index":    0,
					"function": map[string]any{"arguments": `"Paris"}`},
				}}},
			}},
		},
		{ // 5: finish + usage -> terminal completion events
			"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		},
	}

	conv := NewChatToResponsesConverter(newChatChunkStream(chunkSSE(t, chunks)), "gpt-4o-mini")

	var got []wire.ResponsesEvent
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		re, ok := evt.(wire.ResponsesEvent)
		require.Truef(t, ok, "emitted event %T does not implement wire.ResponsesEvent", evt)
		got = append(got, re)
	}

	// 1. Exact ordered event sequence — the heart of the oracle.
	want := []string{
		"response.created",
		"response.output_item.added",             // text item (index 0)
		"response.output_text.delta",             // "Hello"
		"response.output_text.delta",             // ", World!"
		"response.output_item.added",             // function call (index 1)
		"response.function_call_arguments.delta", // {"city":
		"response.function_call_arguments.delta", // "Paris"}
		"response.output_text.done",              // text done before tool done
		"response.output_item.done",              // text message item
		"response.function_call_arguments.done",  // full arguments
		"response.output_item.done",              // function call item
		"response.completed",
	}
	gotTypes := make([]string, len(got))
	for i, e := range got {
		gotTypes[i] = e.EventType()
	}
	require.Equal(t, want, gotTypes, "ordered Responses event sequence")

	// 2. Monotonically increasing sequence numbers 1..N (no gaps/dupes).
	for i, e := range got {
		assert.Equal(t, int64(i+1), seqOf(t, e), "sequence number at position %d", i)
	}

	// 3. Spot-check key payloads.
	assert.Equal(t, "Hello", got[2].(wire.ResponsesOutputTextDeltaEvent).Delta)
	assert.Equal(t, ", World!", got[3].(wire.ResponsesOutputTextDeltaEvent).Delta)

	textDone := got[7].(wire.ResponsesOutputTextDoneEvent)
	assert.Equal(t, "Hello, World!", textDone.Text)

	argsDone := got[9].(wire.ResponsesFunctionCallArgumentsDoneEvent)
	assert.Equal(t, "get_weather", argsDone.Name)
	assert.Equal(t, `{"city":"Paris"}`, argsDone.Arguments)

	completed := got[11].(wire.ResponsesCompletedEvent)
	assert.Equal(t, "completed", completed.Response.Status)
	require.Len(t, completed.Response.Output, 2, "final output carries text + tool-call items")

	// 4. Usage reflects the upstream usage chunk (input = prompt - cached).
	usage := conv.Usage()
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
}

// seqOf extracts the SequenceNumber from any emitted Responses event via its
// concrete type. Centralised here to keep the assertions above readable.
func seqOf(t *testing.T, e wire.ResponsesEvent) int64 {
	t.Helper()
	switch v := e.(type) {
	case wire.ResponsesCreatedEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputItemAddedEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputTextDeltaEvent:
		return v.SequenceNumber
	case wire.ResponsesFunctionCallArgumentsDeltaEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputTextDoneEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputItemDoneEvent:
		return v.SequenceNumber
	case wire.ResponsesFunctionCallArgumentsDoneEvent:
		return v.SequenceNumber
	case wire.ResponsesCompletedEvent:
		return v.SequenceNumber
	default:
		t.Fatalf("unexpected event type %T", e)
		return 0
	}
}
