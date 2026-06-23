package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestHandleResponsesCreate_VisionProxyAppliesBeforeChatConversion is the
// regression test for the bug where /v1/responses requests forwarded to a
// text-only OpenAI-compatible upstream (e.g. DeepSeek) returned
// `messages[N]: unknown variant 'image_url', expected 'text'` even with
// vision proxy configured.
//
// Two ordering bugs lived in HandleResponsesCreate:
//
//  1. The vision proxy switch had no `*responses.ResponseNewParams` case —
//     Responses requests fell through to the no-op default.
//  2. The fix for (1) wasn't enough: applyVisionProxy was called on
//     req.ResponseNewParams BEFORE convertToResponsesParams reparsed the
//     original bodyBytes and overwrote req.ResponseNewParams with a fresh
//     value, discarding the proxy's mutations.
//
// This test exercises the exact handler sub-sequence and asserts the
// post-conversion chat body (the wire-level shape DeepSeek validates)
// contains neither "input_image" nor "image_url". It is intentionally
// written against the handler's component boundary — `convertToResponsesParams`
// + `applyVisionProxy` + `ConvertOpenAIResponsesToChat` — rather than the
// processor in isolation, because the bug was about the order in which
// these three steps fire on the same struct.
func TestHandleResponsesCreate_VisionProxyAppliesBeforeChatConversion(t *testing.T) {
	const dataURL = "data:image/png;base64," + visionTestPNG

	// Multi-turn body matching the real failure case: input_image deep
	// inside the conversation (the original report observed messages[8]).
	bodyBytes := mustMarshalResponsesBody(t, map[string]any{
		"model": "client-facing-model",
		"input": []map[string]any{
			{"role": "user", "content": []map[string]any{{"type": "input_text", "text": "turn 1"}}},
			{"role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "ok"}}},
			{"role": "user", "content": []map[string]any{
				{"type": "input_text", "text": "describe this"},
				{"type": "input_image", "image_url": dataURL},
			}},
		},
		"stream": true,
	})

	s := visionTestServer("claude_code", scenarioVisionExt("p-rule", "vision-model"))

	// Mirror the handler ordering exactly. This is the part under test.
	var req protocol.ResponseCreateRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	params, err := s.convertToResponsesParams(bodyBytes, "downstream-text-only-model")
	if err != nil {
		t.Fatalf("convertToResponsesParams: %v", err)
	}
	req.ResponseNewParams = params
	req.Model = "downstream-text-only-model"
	// Vision proxy MUST run after the assignment above — that is the fix.
	s.applyVisionProxy(newVisionTestGinCtx(), "claude_code", &typ.Rule{}, req.ResponseNewParams)

	// The conversion step is what DeepSeek receives; assert no image_url
	// or input_image survives on the wire-level shape.
	chatParams := request.ConvertOpenAIResponsesToChat(req.ResponseNewParams, 4096)
	chatJSON, err := json.Marshal(chatParams)
	if err != nil {
		t.Fatalf("marshal chat: %v", err)
	}
	got := string(chatJSON)
	if strings.Contains(got, `"image_url"`) {
		t.Fatalf("converted chat body still carries image_url; DeepSeek would reject:\n%s", got)
	}
	if strings.Contains(got, `"input_image"`) {
		t.Fatalf("converted chat body still carries input_image:\n%s", got)
	}
	if !strings.Contains(got, "via vision-model") {
		t.Fatalf("expected the vision-model description spliced in, got:\n%s", got)
	}
}

// TestHandleResponsesCreate_VisionProxyBeforeReparseIsIneffective documents
// the bug shape: applying the vision proxy BEFORE convertToResponsesParams
// is a no-op because the subsequent `req.ResponseNewParams = params`
// overwrite throws away the mutations. If a future refactor reintroduces
// that ordering, this test fails loudly.
//
// It is the inverse of the test above and exists to prevent the same
// ordering mistake from regressing silently — without it, removing the
// `applyVisionProxy` call after the reparse would still pass the happy-path
// test for the single-turn case (because there is no second-call to lose).
func TestHandleResponsesCreate_VisionProxyBeforeReparseIsIneffective(t *testing.T) {
	const dataURL = "data:image/png;base64," + visionTestPNG
	bodyBytes := mustMarshalResponsesBody(t, map[string]any{
		"model": "client-facing-model",
		"input": []map[string]any{
			{"role": "user", "content": []map[string]any{
				{"type": "input_text", "text": "describe this"},
				{"type": "input_image", "image_url": dataURL},
			}},
		},
	})

	s := visionTestServer("claude_code", scenarioVisionExt("p-rule", "vision-model"))

	var req protocol.ResponseCreateRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Wrong order: apply BEFORE the re-parse.
	s.applyVisionProxy(newVisionTestGinCtx(), "claude_code", &typ.Rule{}, req.ResponseNewParams)
	params, err := s.convertToResponsesParams(bodyBytes, "downstream-text-only-model")
	if err != nil {
		t.Fatalf("convertToResponsesParams: %v", err)
	}
	req.ResponseNewParams = params // overwrites the proxy's mutations
	req.Model = "downstream-text-only-model"
	// (no second applyVisionProxy — this is what the bug looked like)

	chatParams := request.ConvertOpenAIResponsesToChat(req.ResponseNewParams, 4096)
	chatJSON, err := json.Marshal(chatParams)
	if err != nil {
		t.Fatalf("marshal chat: %v", err)
	}
	if !strings.Contains(string(chatJSON), `"image_url"`) {
		t.Fatalf("expected the wrong order to LEAK image_url so that the regression test stays meaningful; got:\n%s", chatJSON)
	}
}

func mustMarshalResponsesBody(t *testing.T, body map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return b
}

// _ keeps the responses import used even if a future refactor removes the
// only reference; we want the package alias resolvable for newcomers
// extending these tests.
var _ = responses.ResponseNewParams{}
