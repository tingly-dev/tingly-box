package outputinjector

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// fakeInjector returns prefix once, then "" on every subsequent call. Counts
// PrefixText() invocations so tests can assert call counts.
type fakeInjector struct {
	prefix string
	calls  int
}

func (f *fakeInjector) PrefixText() string {
	f.calls++
	if f.calls == 1 {
		return f.prefix
	}
	return ""
}

// ---------------------------------------------------------------------------
// prependToStreamEvent — per-protocol routing
// ---------------------------------------------------------------------------

func TestPrependToStreamEvent_AnthropicTextDelta(t *testing.T) {
	inj := &fakeInjector{prefix: "[V] "}
	raw := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)
	got, did := prependToStreamEvent(inj, "content_block_delta", raw)
	if !did {
		t.Fatal("expected injection on first text_delta")
	}
	if !strings.Contains(string(got), `"text":"[V] hello"`) {
		t.Fatalf("text not prepended: %s", got)
	}
}

func TestPrependToStreamEvent_AnthropicNonTextDelta_NoInjection(t *testing.T) {
	inj := &fakeInjector{prefix: "[V] "}
	// input_json_delta carries tool input, not text — must be untouched.
	raw := []byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"a\":1}"}}`)
	got, did := prependToStreamEvent(inj, "content_block_delta", raw)
	if did {
		t.Fatal("must not inject into non-text deltas")
	}
	if string(got) != string(raw) {
		t.Fatalf("bytes changed unexpectedly: %s", got)
	}
	// And the injector was not consumed — should still have its prefix ready.
	if inj.calls != 0 {
		t.Fatalf("PrefixText should not be called for non-text deltas; calls=%d", inj.calls)
	}
}

func TestPrependToStreamEvent_OpenAIChat_FirstContentDelta(t *testing.T) {
	inj := &fakeInjector{prefix: "[V] "}
	raw := []byte(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}`)
	got, did := prependToStreamEvent(inj, "chat.completion.chunk", raw)
	if !did {
		t.Fatal("expected injection on first chunk with content")
	}
	if !strings.Contains(string(got), `"content":"[V] hi"`) {
		t.Fatalf("content not prepended: %s", got)
	}
}

func TestPrependToStreamEvent_OpenAIChat_NoContentChunk_NoInjection(t *testing.T) {
	inj := &fakeInjector{prefix: "[V] "}
	// Role-only chunk (no content delta).
	raw := []byte(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"}}]}`)
	got, did := prependToStreamEvent(inj, "chat.completion.chunk", raw)
	if did {
		t.Fatal("must not inject on chunks without content")
	}
	if string(got) != string(raw) {
		t.Fatal("bytes must be untouched")
	}
	if inj.calls != 0 {
		t.Fatal("PrefixText must not be consumed when there is nothing to inject")
	}
}

func TestPrependToStreamEvent_OpenAIResponses_TextDelta(t *testing.T) {
	inj := &fakeInjector{prefix: "[V] "}
	raw := []byte(`{"type":"response.output_text.delta","delta":"world"}`)
	got, did := prependToStreamEvent(inj, "response.output_text.delta", raw)
	if !did {
		t.Fatal("expected injection on first output_text.delta")
	}
	if !strings.Contains(string(got), `"delta":"[V] world"`) {
		t.Fatalf("delta not prepended: %s", got)
	}
}

func TestPrependToStreamEvent_UnknownEventType_NoOp(t *testing.T) {
	inj := &fakeInjector{prefix: "[V] "}
	raw := []byte(`{"type":"message_start"}`)
	got, did := prependToStreamEvent(inj, "message_start", raw)
	if did {
		t.Fatal("must not inject on non-text-bearing events")
	}
	if string(got) != string(raw) {
		t.Fatal("bytes must be untouched for unknown event types")
	}
}

func TestPrependToStreamEvent_OnlyInjectsOnce(t *testing.T) {
	inj := &fakeInjector{prefix: "[V] "}
	first := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"a"}}`)
	second := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"b"}}`)

	if g1, did1 := prependToStreamEvent(inj, "content_block_delta", first); !did1 || !strings.Contains(string(g1), `"text":"[V] a"`) {
		t.Fatalf("first call expected to prepend: did=%v g1=%s", did1, g1)
	}
	if g2, did2 := prependToStreamEvent(inj, "content_block_delta", second); did2 || !strings.Contains(string(g2), `"text":"b"`) {
		t.Fatalf("second call must not prepend: did=%v g2=%s", did2, g2)
	}
}

// ---------------------------------------------------------------------------
// AttachToHandleContext — end-to-end hook registration
// ---------------------------------------------------------------------------

func newTestHandleContext() *protocol.HandleContext {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return protocol.NewHandleContext(c, "test-model")
}

func TestAttachToHandleContext_NilInjector_NoHooksAdded(t *testing.T) {
	hc := newTestHandleContext()
	AttachToHandleContext(hc, nil)
	if len(hc.OnStreamRawEventHooks) != 0 || len(hc.OnNonStreamResponseHooks) != 0 {
		t.Fatal("nil injector must not register any hooks")
	}
}

func TestAttachToHandleContext_WiresBothChains(t *testing.T) {
	hc := newTestHandleContext()
	inj := &fakeInjector{prefix: "[V] "}
	AttachToHandleContext(hc, inj)
	if len(hc.OnStreamRawEventHooks) != 1 {
		t.Fatalf("want 1 raw event hook, got %d", len(hc.OnStreamRawEventHooks))
	}
	if len(hc.OnNonStreamResponseHooks) != 1 {
		t.Fatalf("want 1 non-stream hook, got %d", len(hc.OnNonStreamResponseHooks))
	}

	// Drive the raw hook with a real text_delta event.
	raw := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`)
	got, err := hc.RunStreamRawEventHooks("content_block_delta", raw)
	if err != nil {
		t.Fatalf("hook returned error: %v", err)
	}
	if !strings.Contains(string(got), `"text":"[V] hi"`) {
		t.Fatalf("hook did not inject: %s", got)
	}
}

// ---------------------------------------------------------------------------
// PrependToNonStreamResponse — per-type response mutation
// ---------------------------------------------------------------------------

func TestPrependToNonStreamResponse_AnthropicMessage(t *testing.T) {
	resp := &anthropic.Message{Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "hello"}}}
	inj := &fakeInjector{prefix: "[V] "}
	if !PrependToNonStreamResponse(inj, resp) {
		t.Fatal("expected mutation")
	}
	if resp.Content[0].Text != "[V] hello" {
		t.Fatalf("text not prepended: %q", resp.Content[0].Text)
	}
}

func TestPrependToNonStreamResponse_AnthropicBetaMessage(t *testing.T) {
	resp := &anthropic.BetaMessage{Content: []anthropic.BetaContentBlockUnion{{Type: "text", Text: "hello"}}}
	inj := &fakeInjector{prefix: "[V] "}
	if !PrependToNonStreamResponse(inj, resp) {
		t.Fatal("expected mutation")
	}
	if resp.Content[0].Text != "[V] hello" {
		t.Fatalf("text not prepended: %q", resp.Content[0].Text)
	}
}

func TestPrependToNonStreamResponse_OpenAIChat_Map(t *testing.T) {
	m := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{"content": "hello"},
			},
		},
	}
	inj := &fakeInjector{prefix: "[V] "}
	if !PrependToNonStreamResponse(inj, m) {
		t.Fatal("expected mutation")
	}
	got := m["choices"].([]interface{})[0].(map[string]interface{})["message"].(map[string]interface{})["content"]
	if got != "[V] hello" {
		t.Fatalf("content not prepended: %v", got)
	}
}

func TestPrependToNonStreamResponse_OpenAIChat_Typed(t *testing.T) {
	resp := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{Content: "hello"},
		}},
	}
	inj := &fakeInjector{prefix: "[V] "}
	if !PrependToNonStreamResponse(inj, resp) {
		t.Fatal("expected mutation")
	}
	if resp.Choices[0].Message.Content != "[V] hello" {
		t.Fatalf("content not prepended: %q", resp.Choices[0].Message.Content)
	}
}

func TestPrependToNonStreamResponse_AnthropicToolUseOnly_NoInjection(t *testing.T) {
	// First content block is tool_use, not text — injector should NOT fire.
	resp := &anthropic.Message{Content: []anthropic.ContentBlockUnion{{Type: "tool_use"}}}
	inj := &fakeInjector{prefix: "[V] "}
	if PrependToNonStreamResponse(inj, resp) {
		t.Fatal("must not inject when no text content present")
	}
	if inj.calls != 0 {
		t.Fatalf("PrefixText must not be consumed; calls=%d", inj.calls)
	}
}

func TestPrependToNonStreamResponse_NilInjector_NoOp(t *testing.T) {
	resp := &anthropic.Message{Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "hello"}}}
	if PrependToNonStreamResponse(nil, resp) {
		t.Fatal("nil injector must return false")
	}
	if resp.Content[0].Text != "hello" {
		t.Fatalf("text must be unchanged: %q", resp.Content[0].Text)
	}
}

func TestPrependToNonStreamResponse_OpenAIResponses(t *testing.T) {
	// Build via raw JSON to avoid SDK private constructor quirks.
	rawJSON := []byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`)
	var resp responses.Response
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		t.Fatalf("seed unmarshal failed: %v", err)
	}
	inj := &fakeInjector{prefix: "[V] "}
	if !PrependToNonStreamResponse(inj, &resp) {
		t.Fatal("expected mutation")
	}
	if resp.Output[0].Content[0].Text != "[V] hello" {
		t.Fatalf("text not prepended: %q", resp.Output[0].Content[0].Text)
	}
}
