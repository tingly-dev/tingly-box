package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// decodeSSEData pulls every `data: {json}` payload from an SSE body and
// returns them parsed. JSON-level escaping (e.g. <image-description> →
// <…) is reversed by the unmarshal, so assertions can match the text
// the client actually accumulates.
func decodeSSEData(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &m))
		out = append(out, m)
	}
	return out
}

// newInjectGinCtx returns a gin context backed by a recorder, with the
// given wrapped descriptions stashed exactly as applyVisionProxy would.
func newInjectGinCtx(descs []string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if len(descs) > 0 {
		c.Set(GinKeyVisionDescriptions, descs)
	}
	return c, rec
}

const wrappedDuck = "\n<image-description>a yellow duck</image-description>\n"

// TestVisionStreamInject_OpenAIChat_PrependsSyntheticChunk proves the hook
// emits a synthetic content chunk before the model's first text chunk.
func TestVisionStreamInject_OpenAIChat_PrependsSyntheticChunk(t *testing.T) {
	c, rec := newInjectGinCtx([]string{wrappedDuck})
	hc := protocol.NewHandleContext(c, "downstream-text-model")

	hook := visionStreamInjectFactory(hc)
	require.NotNil(t, hook, "factory must return a hook when descriptions present")

	// First text chunk.
	chunk := &openai.ChatCompletionChunk{ID: "chunk-1", Created: 123, Model: "deepseek-chat"}
	chunk.Choices = []openai.ChatCompletionChunkChoice{
		{Delta: openai.ChatCompletionChunkChoiceDelta{Content: "It's"}},
	}
	require.NoError(t, hook(chunk))
	// Second chunk must NOT trigger another injection.
	require.NoError(t, hook(chunk))

	events := decodeSSEData(t, rec.Body.String())
	require.Len(t, events, 1, "exactly one synthetic event emitted")
	require.Equal(t, "chat.completion.chunk", events[0]["object"])
	require.Equal(t, "chunk-1", events[0]["id"])
	choices := events[0]["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	require.Contains(t, delta["content"], "<image-description>a yellow duck</image-description>")
}

// TestVisionStreamInject_OpenAIChat_SkipsRolePreamble ensures the prefix is
// not injected on a role-only / empty-content chunk.
func TestVisionStreamInject_OpenAIChat_SkipsRolePreamble(t *testing.T) {
	c, rec := newInjectGinCtx([]string{wrappedDuck})
	hc := protocol.NewHandleContext(c, "m")
	hook := visionStreamInjectFactory(hc)

	role := &openai.ChatCompletionChunk{ID: "c", Model: "m"}
	role.Choices = []openai.ChatCompletionChunkChoice{
		{Delta: openai.ChatCompletionChunkChoiceDelta{Role: "assistant"}},
	}
	require.NoError(t, hook(role))
	require.Empty(t, rec.Body.String(), "role-only chunk must not trigger injection")
}

// TestVisionStreamInject_Anthropic_PrependsTextDelta is the key
// cross-transport proof: Anthropic forwards upstream RawJSON, so the hook
// must PREPEND a synthetic content_block_delta rather than mutate.
func TestVisionStreamInject_Anthropic_PrependsTextDelta(t *testing.T) {
	c, rec := newInjectGinCtx([]string{wrappedDuck})
	hc := protocol.NewHandleContext(c, "m")
	hook := visionStreamInjectFactory(hc)
	require.NotNil(t, hook)

	// A real content_block_delta(text_delta) at index 0.
	var evt anthropic.MessageStreamEventUnion
	err := evt.UnmarshalJSON([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"It's"}}`))
	require.NoError(t, err)

	require.NoError(t, hook(&evt))
	require.NoError(t, hook(&evt)) // second call: no re-injection

	body := rec.Body.String()
	require.Contains(t, body, "event: content_block_delta")
	events := decodeSSEData(t, body)
	require.Len(t, events, 1, "exactly one synthetic event emitted")
	require.Equal(t, "content_block_delta", events[0]["type"])
	delta := events[0]["delta"].(map[string]any)
	require.Equal(t, "text_delta", delta["type"])
	require.Contains(t, delta["text"], "<image-description>a yellow duck</image-description>")
}

// TestVisionStreamInject_Anthropic_SkipsNonText ensures message_start and
// content_block_start (no text yet) do not trigger injection — important
// when the first block is thinking/tool_use.
func TestVisionStreamInject_Anthropic_SkipsNonText(t *testing.T) {
	c, rec := newInjectGinCtx([]string{wrappedDuck})
	hc := protocol.NewHandleContext(c, "m")
	hook := visionStreamInjectFactory(hc)

	var start anthropic.MessageStreamEventUnion
	require.NoError(t, start.UnmarshalJSON([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)))
	require.NoError(t, hook(&start))
	require.Empty(t, rec.Body.String(), "content_block_start must not trigger injection")
}

// TestVisionStreamInject_Responses_PrependsTextDelta covers the
// converter-based OpenAI Responses path: events flow as concrete
// wire.ResponsesEvent value types. Mutation isn't possible, but the
// hook can write a synthetic response.output_text.delta bound to the
// same (item_id, output_index, content_index) as the model's first
// delta. The client accumulates deltas in the same content part →
// description text lands ahead of the model text.
func TestVisionStreamInject_Responses_PrependsTextDelta(t *testing.T) {
	c, rec := newInjectGinCtx([]string{wrappedDuck})
	hc := protocol.NewHandleContext(c, "m")
	hook := visionStreamInjectFactory(hc)
	require.NotNil(t, hook)

	real := wire.ResponsesOutputTextDeltaEvent{
		Type:           "response.output_text.delta",
		SequenceNumber: 7,
		ItemID:         "item-x",
		OutputIndex:    0,
		ContentIndex:   0,
		Delta:          "It's",
	}
	require.NoError(t, hook(real))
	require.NoError(t, hook(real)) // second call: no re-injection

	body := rec.Body.String()
	require.Contains(t, body, "event: response.output_text.delta")
	events := decodeSSEData(t, body)
	require.Len(t, events, 1, "exactly one synthetic event emitted")
	require.Equal(t, "response.output_text.delta", events[0]["type"])
	require.Equal(t, "item-x", events[0]["item_id"])
	require.EqualValues(t, 0, events[0]["output_index"])
	require.EqualValues(t, 0, events[0]["content_index"])
	require.Contains(t, events[0]["delta"], "<image-description>a yellow duck</image-description>")
}

// TestVisionStreamInject_Responses_SkipsNonText ensures the hook ignores
// content_part.added / output_item.added (which arrive BEFORE the first
// delta) — only the text-delta event has the framing needed to land
// the synthetic delta correctly.
func TestVisionStreamInject_Responses_SkipsNonText(t *testing.T) {
	c, rec := newInjectGinCtx([]string{wrappedDuck})
	hc := protocol.NewHandleContext(c, "m")
	hook := visionStreamInjectFactory(hc)

	require.NoError(t, hook(wire.ResponsesContentPartAddedEvent{
		Type: "response.content_part.added", ItemID: "i", ContentIndex: 0,
	}))
	require.Empty(t, rec.Body.String(), "content_part.added must not trigger injection")
}

// TestVisionStreamInject_NoDescriptions_NoHook confirms zero overhead when
// the request carried no images.
func TestVisionStreamInject_NoDescriptions_NoHook(t *testing.T) {
	c, _ := newInjectGinCtx(nil)
	hc := protocol.NewHandleContext(c, "m")
	require.Nil(t, visionStreamInjectFactory(hc), "no descriptions → no hook")
}

// TestVisionStreamInject_AutoAttach proves NewHandleContext wires the
// factory automatically once registered — the zero-handler-change claim.
func TestVisionStreamInject_AutoAttach(t *testing.T) {
	protocol.ResetDefaultStreamEventHookFactories()
	t.Cleanup(protocol.ResetDefaultStreamEventHookFactories)
	protocol.RegisterDefaultStreamEventHookFactory(visionStreamInjectFactory)

	c, rec := newInjectGinCtx([]string{wrappedDuck})
	hc := protocol.NewHandleContext(c, "m")
	require.Len(t, hc.OnStreamEventHooks, 1, "factory auto-attached one hook")

	chunk := &openai.ChatCompletionChunk{ID: "c", Model: "m"}
	chunk.Choices = []openai.ChatCompletionChunkChoice{
		{Delta: openai.ChatCompletionChunkChoiceDelta{Content: "hi"}},
	}
	// Drive the hook the way ProcessStream would.
	for _, h := range hc.OnStreamEventHooks {
		require.NoError(t, h(chunk))
	}
	events := decodeSSEData(t, rec.Body.String())
	require.Len(t, events, 1)
	choices := events[0]["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	require.Contains(t, delta["content"], "a yellow duck")
}
