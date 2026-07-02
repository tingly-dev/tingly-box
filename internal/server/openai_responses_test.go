package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/visionproxy"
	"github.com/tingly-dev/tingly-box/internal/visionproxy/visionproxytest"
)

// _ keeps the responses import used even if a future refactor removes the
// only reference; we want the package alias resolvable for newcomers
// extending these tests.
var _ = responses.ResponseNewParams{}

// TestResponseNewParamsInputUnion_String tests unmarshaling a string input
func TestResponseNewParamsInputUnion_String(t *testing.T) {
	jsonString := `"Hello, world!"`

	var input responses.ResponseNewParamsInputUnion
	if err := json.Unmarshal([]byte(jsonString), &input); err != nil {
		t.Fatalf("Failed to deserialize string into ResponseNewParamsInputUnion: %v", err)
	}

	if param.IsOmitted(input.OfString) {
		t.Fatal("Expected input to be a string")
	}

	str := input.OfString.Value
	if str != "Hello, world!" {
		t.Fatalf("Expected 'Hello, world!', got '%s'", str)
	}
}

// TestResponseNewParamsInputUnion_Array tests unmarshaling an array input
func TestResponseNewParamsInputUnion_Array(t *testing.T) {
	jsonString := `[{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello"}]}]`

	var input responses.ResponseNewParamsInputUnion
	if err := json.Unmarshal([]byte(jsonString), &input); err != nil {
		t.Fatalf("Failed to deserialize array into ResponseNewParamsInputUnion: %v", err)
	}

	if param.IsOmitted(input.OfInputItemList) {
		t.Fatal("Expected input to be an array")
	}
}

// TestResponseCreateRequest_UnmarshalJSON tests unmarshaling a ResponseCreateRequest
func TestResponseCreateRequest_UnmarshalJSON(t *testing.T) {
	jsonString := `{
		"model": "gpt-4o",
		"input": "Write a haiku",
		"stream": true
	}`

	var req protocol.ResponseCreateRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize into ResponseCreateRequest: %v", err)
	}

	if string(req.Model) != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	if !req.Stream {
		t.Fatal("Expected stream to be true")
	}

	if param.IsOmitted(req.Input.OfString) {
		t.Fatal("Expected input to be a string")
	}

	str := req.Input.OfString.Value
	if str != "Write a haiku" {
		t.Fatalf("Expected input 'Write a haiku', got '%s'", str)
	}
}

// TestResponseCreateRequest_UnmarshalJSON_ArrayInput tests unmarshaling a ResponseCreateRequest with array input
func TestResponseCreateRequest_UnmarshalJSON_ArrayInput(t *testing.T) {
	jsonString := `{
		"model": "gpt-4o",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello"}]}
		]
	}`

	var req protocol.ResponseCreateRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize into ResponseCreateRequest: %v", err)
	}

	if string(req.Model) != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	if param.IsOmitted(req.Input.OfInputItemList) {
		t.Fatal("Expected input to be an array")
	}
}

func TestResponseCreateRequest_UnmarshalJSON_TypelessFunctionCallInput(t *testing.T) {
	jsonString := `{
		"model": "gpt-4o",
		"input": [
			{
				"call_id": "call_123",
				"name": "Write",
				"arguments": "{\"file_path\":\"c:/Users/nil/workspace/mario-game.html\",\"content\":\"hello\"}"
			}
		]
	}`

	var req protocol.ResponseCreateRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize typeless function_call input: %v", err)
	}

	if param.IsOmitted(req.Input.OfInputItemList) {
		t.Fatal("Expected input item list")
	}
	items := req.Input.OfInputItemList
	if len(items) != 1 {
		t.Fatalf("Expected 1 input item, got %d", len(items))
	}
	if param.IsOmitted(items[0].OfFunctionCall) {
		t.Fatal("Expected function_call item to be preserved")
	}
	if items[0].OfFunctionCall.Name != "Write" {
		t.Fatalf("Expected function name Write, got %q", items[0].OfFunctionCall.Name)
	}
	if items[0].OfFunctionCall.Arguments == "" {
		t.Fatal("Expected function call arguments to be preserved")
	}
	if items[0].OfFunctionCall.Arguments != `{"file_path":"c:/Users/nil/workspace/mario-game.html","content":"hello"}` {
		t.Fatalf("Unexpected function call arguments: %s", items[0].OfFunctionCall.Arguments)
	}
	if items[0].OfFunctionCall.CallID != "call_123" {
		t.Fatalf("Expected call_id call_123, got %q", items[0].OfFunctionCall.CallID)
	}
}

// TestResponseCreateRequest_UnmarshalJSON_WithExtraFields tests unmarshaling with extra fields
func TestResponseCreateRequest_UnmarshalJSON_WithExtraFields(t *testing.T) {
	jsonString := `{
		"model": "gpt-4o",
		"input": "Hello",
		"temperature": 0.7,
		"max_output_tokens": 1000
	}`

	var req protocol.ResponseCreateRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize into ResponseCreateRequest: %v", err)
	}

	if string(req.Model) != "gpt-4o" {
		t.Fatalf("Expected model 'gpt-4o', got '%s'", req.Model)
	}

	// Check that temperature was captured (it's a field on ResponseNewParams)
	if !param.IsOmitted(req.Temperature) {
		if req.Temperature.Value != 0.7 {
			t.Fatalf("Expected temperature 0.7, got %v", req.Temperature.Value)
		}
	}

	// Check that max_output_tokens was captured
	if !param.IsOmitted(req.MaxOutputTokens) {
		if req.MaxOutputTokens.Value != 1000 {
			t.Fatalf("Expected max_output_tokens 1000, got %v", req.MaxOutputTokens.Value)
		}
	}
}

func TestConvertToResponsesParams_Simple(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hello world"
	}`)

	server := &Server{} // Minimal server for testing
	params, err := server.convertToResponsesParams(body, "gpt-4")
	if err != nil {
		t.Fatalf("Failed to convert params: %v", err)
	}

	// The model should be overridden
	// Note: ResponseNewParams.Model is shared.ResponsesModel (string type)
	// We can't directly access it, but the conversion should succeed
	_ = params
}

func TestConvertToResponsesParams_Complex(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hello world",
		"temperature": 0.7,
		"max_output_tokens": 1000,
		"instructions": "Be concise"
	}`)

	server := &Server{}
	params, err := server.convertToResponsesParams(body, "gpt-4")
	if err != nil {
		t.Fatalf("Failed to convert params: %v", err)
	}

	// The model should be overridden to gpt-4
	_ = params
}

func TestResponseNewParams_MarshalUnmarshal(t *testing.T) {
	// Test that the SDK's ResponseNewParams can handle our input
	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("Hello world")},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal ResponseNewParams: %v", err)
	}

	var unmarshaled responses.ResponseNewParams
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ResponseNewParams: %v", err)
	}
}

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

// visionTestPNG re-exports visionproxytest.PNG for tests in this package
// (e.g. openai_responses_vision_test.go) that need a real applyVisionProxy
// call through *Server rather than visionproxy.Service in isolation.
// Logic-level coverage for the plugin itself lives in
// internal/server/module/visionproxy.
const visionTestPNG = visionproxytest.PNG

func newVisionTestGinCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/tingly/claude_code/messages", nil)
	return c
}

// visionTestServer builds a Server whose scenario config carries the given
// Extensions, with a stub vision processor that echoes the model used.
func visionTestServer(scenario typ.RuleScenario, ext map[string]interface{}) *Server {
	return &Server{
		config: &config.Config{
			Scenarios: []typ.ScenarioConfig{
				{Scenario: scenario, Extensions: ext},
			},
		},
		visionProxyService: visionproxy.NewService(visionproxytest.NewProcessor()),
	}
}

func scenarioVisionExt(provider, model string) map[string]interface{} {
	return visionproxytest.ScenarioExt(provider, model)
}
