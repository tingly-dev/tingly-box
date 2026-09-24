package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

func TestSanitizeCodexInputIDsJSON_DropsRequiredEmptyID(t *testing.T) {
	body := `{
		"model": "gpt-5-codex",
		"input": [
			{"type": "message", "role": "user", "content": "hello"},
			{"type": "reasoning", "id": "", "summary": []},
			{"type": "reasoning", "id": "rs_abc123", "summary": []}
		]
	}`

	out := sanitizeCodexInputIDsJSON(t.Context(), body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 2, "reasoning item with empty id must be dropped")
	assert.Equal(t, "message", input[0].Get("type").String())
	assert.Equal(t, "reasoning", input[1].Get("type").String())
	assert.Equal(t, "rs_abc123", input[1].Get("id").String())
}

func TestSanitizeCodexInputIDsJSON_ClearsOptionalEmptyID(t *testing.T) {
	body := `{
		"input": [
			{"type": "function_call", "id": "", "call_id": "c1", "name": "f", "arguments": "{}"},
			{"type": "shell_call", "id": "", "call_id": "c2", "action": {}},
			{"type": "apply_patch_call", "id": "", "call_id": "c3", "operation": {}}
		]
	}`

	out := sanitizeCodexInputIDsJSON(t.Context(), body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 3, "optional-id items must not be dropped")
	for _, item := range input {
		assert.False(t, item.Get("id").Exists(), "empty id should be deleted from item type %s", item.Get("type").String())
	}
}

func TestSanitizeCodexInputIDsJSON_DropsInvalidChars(t *testing.T) {
	body := `{
		"input": [
			{"type": "reasoning", "id": "rs/bad chars!", "summary": []},
			{"type": "function_call", "id": "fc/bad", "call_id": "c1", "name": "f", "arguments": "{}"}
		]
	}`

	out := sanitizeCodexInputIDsJSON(t.Context(), body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 1, "reasoning with invalid chars must be dropped")
	assert.Equal(t, "function_call", input[0].Get("type").String())
	assert.False(t, input[0].Get("id").Exists(), "function_call id with invalid chars must be cleared")
}

func TestSanitizeCodexInputIDsJSON_NoOpWhenAllValid(t *testing.T) {
	body := `{"input":[{"type":"reasoning","id":"rs_abc","summary":[]},{"type":"function_call","id":"fc_123","call_id":"c","name":"f","arguments":"{}"}]}`

	out := sanitizeCodexInputIDsJSON(t.Context(), body)

	assert.Equal(t, body, out, "valid ids must not be modified")
}

func TestSanitizeCodexInputIDsJSON_NoInputArray(t *testing.T) {
	body := `{"model":"gpt-5-codex","input":"hello"}`

	out := sanitizeCodexInputIDsJSON(t.Context(), body)

	assert.Equal(t, body, out, "non-array input must be passed through")
}

func TestSanitizeCodexInputIDsJSON_WhitespaceOnlyID(t *testing.T) {
	body := `{"input":[{"type":"function_call_output","id":"   ","call_id":"c1","output":"ok"}]}`

	out := sanitizeCodexInputIDsJSON(t.Context(), body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 1)
	assert.False(t, input[0].Get("id").Exists(), "whitespace-only id should be cleared")
}

func TestSanitizeCodexInputIDsJSON_HighIndex(t *testing.T) {
	// Mirrors the production failure: input[186].id == ""
	var items []string
	for i := 0; i < 186; i++ {
		items = append(items, `{"type":"message","role":"user","content":"x"}`)
	}
	items = append(items, `{"type":"shell_call","id":"","call_id":"c","action":{}}`)
	body := `{"input":[` + strings.Join(items, ",") + `]}`

	out := sanitizeCodexInputIDsJSON(t.Context(), body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 187)
	assert.False(t, input[186].Get("id").Exists())
}

func TestSanitizeCodexEmptyContentJSON_DropsEmptyStringContent(t *testing.T) {
	body := `{
		"input": [
			{"type": "message", "role": "user", "content": "hello"},
			{"type": "message", "role": "assistant", "content": ""},
			{"type": "message", "role": "user", "content": ""},
			{"type": "function_call_output", "call_id": "c1", "output": ""}
		]
	}`

	out := sanitizeCodexEmptyContentJSON(t.Context(), body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 2, "message items with empty string content must be dropped")
	assert.Equal(t, "hello", input[0].Get("content").String())
	assert.Equal(t, "function_call_output", input[1].Get("type").String(), "non-message items with empty content must be kept")
}

func TestSanitizeCodexEmptyContentJSON_KeepsNonEmptyContent(t *testing.T) {
	body := `{"input":[{"type":"message","role":"user","content":"hi"},{"type":"message","role":"assistant","content":"hello"}]}`

	out := sanitizeCodexEmptyContentJSON(t.Context(), body)

	assert.Equal(t, body, out, "non-empty content must not be modified")
}

func TestSanitizeCodexEmptyContentJSON_NoInputArray(t *testing.T) {
	body := `{"model":"gpt-5-codex","input":"hello"}`

	out := sanitizeCodexEmptyContentJSON(t.Context(), body)

	assert.Equal(t, body, out)
}

func TestSanitizeCodexEmptyContentJSON_HighIndex(t *testing.T) {
	// Mirrors the production failure: input[1012].content == ""
	var items []string
	for i := 0; i < 1012; i++ {
		items = append(items, `{"type":"message","role":"user","content":"x"}`)
	}
	items = append(items, `{"type":"message","role":"assistant","content":""}`)
	body := `{"input":[` + strings.Join(items, ",") + `]}`

	out := sanitizeCodexEmptyContentJSON(t.Context(), body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 1012, "empty-content message at high index must be dropped")
}

func TestNormalizeCodexSystemMessagesJSON(t *testing.T) {
	t.Run("lifts cached system input into instructions", func(t *testing.T) {
		body := `{
			"model":"gpt-5.6-sol",
			"input":[
				{"type":"message","role":"system","content":[{"type":"input_text","text":"You are Claude Code.","prompt_cache_breakpoint":{"type":"ephemeral"}}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}
			]
		}`

		out := normalizeCodexSystemMessagesJSON(body)
		assert.Equal(t, "You are Claude Code.", gjson.Get(out, "instructions").String())
		input := gjson.Get(out, "input").Array()
		require.Len(t, input, 1)
		assert.Equal(t, "user", input[0].Get("role").String())
	})

	t.Run("preserves existing instructions and system order", func(t *testing.T) {
		body := `{
			"instructions":"existing",
			"input":[
				{"type":"message","role":"system","content":"first"},
				{"type":"message","role":"user","content":"hello"},
				{"type":"message","role":"system","content":[{"type":"input_text","text":"second"}]}
			]
		}`

		out := normalizeCodexSystemMessagesJSON(body)
		// System parts concatenate verbatim — the exact inverse of the
		// converters' system→instructions join — so the lifted text is
		// byte-identical whether or not a cache breakpoint forced the system
		// prompt through the input array this turn. Text the client authored as
		// `instructions` is a separate string and stays separated.
		assert.Equal(t, "existing\n\nfirstsecond", gjson.Get(out, "instructions").String())
		assert.Len(t, gjson.Get(out, "input").Array(), 1)
	})

	t.Run("leaves requests without system input untouched", func(t *testing.T) {
		body := `{"instructions":"existing","input":[{"type":"message","role":"user","content":"hello"}]}`
		assert.Equal(t, body, normalizeCodexSystemMessagesJSON(body))
	})
}

func TestCodexFilterFieldLiftsSystemMessages(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"prompt_cache_key":"session-1",
		"prompt_cache_options":{"mode":"explicit"},
		"prompt_cache_retention":"24h",
		"input":[
			{"type":"message","role":"system","content":[{"type":"input_text","text":"system prompt","prompt_cache_breakpoint":{"type":"ephemeral"}}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"type":"ephemeral"}}]},
			{"type":"function_call_output","call_id":"call-1","output":[{"type":"input_text","text":"result","prompt_cache_breakpoint":{"type":"ephemeral"}}]}
		]
	}`)

	out, err := (&codexRoundTripper{}).filterField(t.Context(), body)
	require.NoError(t, err)
	assert.Equal(t, "system prompt", gjson.GetBytes(out, "instructions").String())
	assert.False(t, gjson.GetBytes(out, `input.#(role=="system")`).Exists())
	assert.Equal(t, "user", gjson.GetBytes(out, "input.0.role").String())
	assert.False(t, gjson.GetBytes(out, "prompt_cache_options").Exists())
	assert.False(t, gjson.GetBytes(out, "prompt_cache_retention").Exists())
	assert.Equal(t, "session-1", gjson.GetBytes(out, "prompt_cache_key").String())
	assert.False(t, gjson.GetBytes(out, "input.0.content.0.prompt_cache_breakpoint").Exists())
	assert.False(t, gjson.GetBytes(out, "input.1.output.0.prompt_cache_breakpoint").Exists())
}

func TestCodexInputItemIDRequired(t *testing.T) {
	required := []string{
		"reasoning", "code_interpreter_call", "computer_call", "file_search_call",
		"web_search_call", "image_generation_call", "local_shell_call",
		"local_shell_call_output", "mcp_list_tools", "mcp_approval_request",
		"mcp_call", "item_reference",
	}
	optional := []string{
		"function_call", "function_call_output", "shell_call", "shell_call_output",
		"apply_patch_call", "apply_patch_call_output", "computer_call_output",
		"custom_tool_call", "custom_tool_call_output", "mcp_approval_response",
		"compaction", "message", "output_message",
	}
	for _, typ := range required {
		assert.True(t, codexInputItemIDRequired(typ), "type %q should require id", typ)
	}
	for _, typ := range optional {
		assert.False(t, codexInputItemIDRequired(typ), "type %q should not require id", typ)
	}
}

func TestValidateCodexStreamResponse_AllowsSSE(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body:   io.NopCloser(strings.NewReader("data: {}\n\n")),
	}

	require.NoError(t, validateCodexStreamResponse(resp))
}

func TestValidateCodexStreamResponse_AllowsMissingContentType(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{},
		Body:   io.NopCloser(strings.NewReader("data: {}\n\n")),
	}

	require.NoError(t, validateCodexStreamResponse(resp))
}

func TestValidateCodexStreamResponse_RejectsJSON200(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(`{"error":"maintenance"}`)),
	}

	err := validateCodexStreamResponse(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-SSE 200 response")
	assert.Contains(t, err.Error(), "maintenance")
}

func TestValidateCodexStreamResponse_AllowsAmbiguousNonSSEContentType(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/plain"}},
		Body:   io.NopCloser(strings.NewReader("data: {}\n\n")),
	}

	require.NoError(t, validateCodexStreamResponse(resp))
}

// fakeRoundTripper returns a fixed response for every request, standing in
// for the real transport underneath codexRoundTripper.
type codexFakeRoundTripper struct {
	status int
	body   string
}

func (f codexFakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: f.status,
		Status:     http.StatusText(f.status),
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

// codexTestClient builds a real *openai.Client whose HTTP calls go through
// the real codexRoundTripper to inner, so these tests exercise the whole
// production error path — RoundTrip's body normalization plus the SDK's own
// res.StatusCode >= 400 handling in Execute — not just the RoundTripper in
// isolation. codexRoundTripper rewrites unprefixed paths as
// codexProtocolResponsesSSE (see rewriteCodexPath), which is what a plain
// "images/generations" fake base exercises here; the images-specific
// (codexProtocolPlainJSON) routing is covered separately in
// codex_images_test.go.
func codexTestClient(inner http.RoundTripper) openai.Client {
	return openai.NewClient(
		option.WithBaseURL("http://example.com/"),
		option.WithAPIKey("test"),
		option.WithMaxRetries(0),
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: inner}}),
	)
}

// TestCodexRoundTripper_NonOKStatusClassifiesAsOpenAIError guards against a
// non-200 Codex response (e.g. a 400 content_policy_violation on image
// generation) being turned into a generic Go error. This RoundTripper used
// to return a bare fmt.Errorf, which net/http's Client.Do wraps in
// *url.Error — and *url.Error satisfies net.Error (it has a Timeout()
// method), so protocol.ClassifyUpstreamFailure's transport-failure fallback
// mis-reported a real, actionable 400 as a generic 502 "network_error",
// discarding the real status and message entirely.
func TestCodexRoundTripper_NonOKStatusClassifiesAsOpenAIError(t *testing.T) {
	oc := codexTestClient(codexFakeRoundTripper{
		status: http.StatusBadRequest,
		body:   `{"error":{"message":"Your request was rejected due to content policy.","code":"content_policy_violation"}}`,
	})

	var resp map[string]any
	doErr := oc.Post(context.Background(), "images/generations", map[string]any{}, &resp)
	require.Error(t, doErr)

	var oaiErr *openai.Error
	require.ErrorAs(t, doErr, &oaiErr, "expected the error to unwrap to *openai.Error")
	assert.Equal(t, http.StatusBadRequest, oaiErr.StatusCode)
	assert.Equal(t, "content_policy_violation", oaiErr.Code)
	assert.Equal(t, "Your request was rejected due to content policy.", oaiErr.Message)

	// This is the end-to-end shape the client-facing error goes through:
	// the real 400 and message must survive, not the "502 network_error"
	// catch-all a bare fmt.Errorf here used to produce.
	failure := protocol.ClassifyUpstreamFailure(doErr, http.StatusInternalServerError)
	assert.Equal(t, http.StatusBadRequest, failure.Status)
	assert.Contains(t, failure.Message, "content_policy_violation")
	assert.Contains(t, failure.Message, "Your request was rejected due to content policy.")
	assert.NotContains(t, failure.Message, "network_error")
}

// TestCodexRoundTripper_NonOKStatusWithoutErrorWrapper covers Codex's
// ChatGPT backend not always nesting its error under an "error" key the way
// the public OpenAI API does — the message must still make it through.
// normalizeCodexErrorBody doesn't special-case this: since the whole body
// here already looks like {"message":..., "code":...}, wrapping it under
// "error" (the one shape the SDK's own extraction handles) is enough for the
// SDK to populate Code/Message directly.
func TestCodexRoundTripper_NonOKStatusWithoutErrorWrapper(t *testing.T) {
	oc := codexTestClient(codexFakeRoundTripper{
		status: http.StatusTooManyRequests,
		body:   `{"message":"rate limited","code":"rate_limit_exceeded"}`,
	})

	var resp map[string]any
	doErr := oc.Post(context.Background(), "images/generations", map[string]any{}, &resp)
	require.Error(t, doErr)

	var oaiErr *openai.Error
	require.ErrorAs(t, doErr, &oaiErr)
	assert.Equal(t, http.StatusTooManyRequests, oaiErr.StatusCode)
	assert.Equal(t, "rate_limit_exceeded", oaiErr.Code)
	assert.Equal(t, "rate limited", oaiErr.Message)
}

// TestCodexRoundTripper_NonOKStatusBareStringError covers the shape that
// motivated normalizeCodexErrorBody in the first place: "error" present but
// not an object. Letting the SDK's own gjson.GetBytes(contents,
// "error").Raw extraction see a bare JSON string fails its UnmarshalJSON
// outright (a *json.UnmarshalTypeError, not even an *openai.Error) — see
// that function's doc and TestCodexRoundTripper_ImagesErrorStatusSurfaced in
// codex_images_test.go for the same fixture at the RoundTrip layer.
func TestCodexRoundTripper_NonOKStatusBareStringError(t *testing.T) {
	oc := codexTestClient(codexFakeRoundTripper{
		status: http.StatusBadRequest,
		body:   `{"error":"bad image"}`,
	})

	var resp map[string]any
	doErr := oc.Post(context.Background(), "images/generations", map[string]any{}, &resp)
	require.Error(t, doErr)

	var oaiErr *openai.Error
	require.ErrorAs(t, doErr, &oaiErr, "expected the error to unwrap to *openai.Error, not a bare json error")
	assert.Equal(t, http.StatusBadRequest, oaiErr.StatusCode)
	assert.Equal(t, "bad image", oaiErr.Message)
}

// TestCodexRoundTripper_NonOKStatusNotRetried guards the other half of the
// network_error bug: a real *http.Response (not a RoundTrip error) lets the
// SDK's shouldRetry judge retryability from the status code, so a
// deterministic 400 isn't retried as if the connection had dropped — which
// would silently re-send a policy-blocked prompt. This holds even with the
// SDK's default retry count (2), not just because tingly-box's own client
// construction happens to set option.WithMaxRetries(0).
func TestCodexRoundTripper_NonOKStatusNotRetried(t *testing.T) {
	var hits int32
	handler := &countingRoundTripper{status: http.StatusBadRequest, body: `{"error":{"message":"blocked"}}`, hits: &hits}
	oc := openai.NewClient(
		option.WithBaseURL("http://example.com/"),
		option.WithAPIKey("test"),
		// Deliberately not setting MaxRetries: this must hold on the SDK's
		// own default (2 retries), not rely on tingly-box overriding it.
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: handler}}),
	)

	var resp map[string]any
	doErr := oc.Post(context.Background(), "images/generations", map[string]any{}, &resp)
	require.Error(t, doErr)
	assert.EqualValues(t, 1, hits, "a deterministic 400 must not be retried")
}

type countingRoundTripper struct {
	status int
	body   string
	hits   *int32
}

func (c *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	*c.hits++
	return &http.Response{
		StatusCode: c.status,
		Status:     http.StatusText(c.status),
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Header:     make(http.Header),
	}, nil
}

// TestCodexBodyIsStableAcrossBreakpointRotation is the end-to-end guard for the
// Codex prompt-cache collapse. The ChatGPT backend strips no ambiguity for us:
// it caches on the request prefix, so any byte the gateway changes between two
// turns over the same history is a cache miss the user pays for.
//
// Codex does not accept prompt-cache breakpoints at all, so once the converters
// stopped letting a breakpoint decide an item's shape, moving the client's
// rolling breakpoints must have exactly zero effect on the dispatched body.
func TestCodexBodyIsStableAcrossBreakpointRotation(t *testing.T) {
	rt := &codexRoundTripper{}
	body := func(breakpointAt int) string {
		converted := request.ConvertAnthropicBetaToResponsesRequest(claudeCodeBetaRequest(breakpointAt))
		raw, err := json.Marshal(converted)
		require.NoError(t, err)
		filtered, err := rt.filterField(t.Context(), raw)
		require.NoError(t, err)
		return string(filtered)
	}

	baseline := body(-1)
	for _, at := range []int{0, 1, 2} {
		assert.Equal(t, baseline, body(at),
			"moving the breakpoint to message block %d changed the Codex request body", at)
	}

	// And the body really is the one Codex expects: system text lifted into
	// instructions, no breakpoint fields left anywhere.
	assert.Equal(t, "You are Claude Code.BIG SYSTEM PROMPT", gjson.Get(baseline, "instructions").String())
	assert.NotContains(t, baseline, "prompt_cache_breakpoint")
	assert.NotContains(t, baseline, "prompt_cache_options")
	for _, item := range gjson.Get(baseline, "input").Array() {
		assert.NotEqual(t, "system", item.Get("role").String())
	}
}

// claudeCodeBetaRequest mirrors a Claude Code turn: two-block system prompt, a
// cached tool definition, and a short tool-use history, with the client's
// rolling ephemeral breakpoint parked on message block breakpointAt (-1 =
// none).
func claudeCodeBetaRequest(breakpointAt int) *anthropic.BetaMessageNewParams {
	cache := func(on bool) anthropic.BetaCacheControlEphemeralParam {
		if on {
			return anthropic.NewBetaCacheControlEphemeralParam()
		}
		return anthropic.BetaCacheControlEphemeralParam{}
	}

	return &anthropic.BetaMessageNewParams{
		Model:     "gpt-5.6-sol",
		MaxTokens: 4096,
		System: []anthropic.BetaTextBlockParam{
			{Text: "You are Claude Code."},
			{Text: "BIG SYSTEM PROMPT", CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
		},
		Tools: []anthropic.BetaToolUnionParam{{
			OfTool: &anthropic.BetaToolParam{
				Name:         "Read",
				InputSchema:  anthropic.BetaToolInputSchemaParam{Type: "object"},
				CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
			},
		}},
		Messages: []anthropic.BetaMessageParam{
			{Role: "user", Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "read a file", CacheControl: cache(breakpointAt == 0)}},
			}},
			{Role: "assistant", Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "on it", CacheControl: cache(breakpointAt == 1)}},
				{OfToolUse: &anthropic.BetaToolUseBlockParam{ID: "toolu_1", Name: "Read", Input: map[string]any{"p": "x"}}},
			}},
			{Role: "user", Content: []anthropic.BetaContentBlockParamUnion{
				{OfToolResult: &anthropic.BetaToolResultBlockParam{
					ToolUseID:    "toolu_1",
					CacheControl: cache(breakpointAt == 2),
					Content: []anthropic.BetaToolResultBlockParamContentUnion{
						{OfText: &anthropic.BetaTextBlockParam{Text: "file contents"}},
					},
				}},
			}},
		},
	}
}

func TestApplyCodexSessionAffinityHeader(t *testing.T) {
	sessionID := "16d97292-8713-438b-ad2e-76f495717258"

	t.Run("mirrors prompt_cache_key into session-id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
		applyCodexSessionAffinityHeader(req, []byte(`{"prompt_cache_key":"`+sessionID+`"}`))
		assert.Equal(t, sessionID, req.Header.Get("session-id"))
	})

	t.Run("keeps a session-id the client already sent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
		req.Header.Set("session-id", "client-owned")
		applyCodexSessionAffinityHeader(req, []byte(`{"prompt_cache_key":"`+sessionID+`"}`))
		assert.Equal(t, "client-owned", req.Header.Get("session-id"))
	})

	t.Run("skips non-uuid keys rather than risk a rejected request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
		applyCodexSessionAffinityHeader(req, []byte(`{"prompt_cache_key":"deadbeefdeadbeefdeadbeefdeadbeef"}`))
		assert.Empty(t, req.Header.Get("session-id"))
	})

	t.Run("no key, no header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
		applyCodexSessionAffinityHeader(req, []byte(`{"model":"gpt-5.6-sol"}`))
		assert.Empty(t, req.Header.Get("session-id"))
	})
}
