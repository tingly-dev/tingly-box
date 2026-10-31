package ops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// responsesRequestWithPromptCache builds a Responses request carrying every
// OpenAI-only prompt-cache field: top-level options and retention, and a
// breakpoint on each input_text / input_image / input_file position a
// converter or client can emit — message content (both message item shapes),
// function_call_output content and custom_tool_call_output content.
func responsesRequestWithPromptCache() *responses.ResponseNewParams {
	text := func() responses.ResponseInputContentUnionParam {
		return responses.ResponseInputContentUnionParam{OfInputText: &responses.ResponseInputTextParam{
			Text: "stable prefix", PromptCacheBreakpoint: responses.NewResponseInputTextPromptCacheBreakpointParam(),
		}}
	}
	image := responses.ResponseInputContentUnionParam{OfInputImage: &responses.ResponseInputImageParam{
		ImageURL: openai.String("https://example.com/a.png"), PromptCacheBreakpoint: responses.NewResponseInputImagePromptCacheBreakpointParam(),
	}}
	file := responses.ResponseInputContentUnionParam{OfInputFile: &responses.ResponseInputFileParam{
		FileID: openai.String("file-1"), PromptCacheBreakpoint: responses.NewResponseInputFilePromptCacheBreakpointParam(),
	}}

	req := &responses.ResponseNewParams{
		Model:                "gpt-5.6",
		PromptCacheKey:       openai.String("stable-affinity-key"),
		PromptCacheOptions:   responses.ResponseNewParamsPromptCacheOptions{Mode: "explicit"},
		PromptCacheRetention: responses.ResponseNewParamsPromptCacheRetention("24h"),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam{
			{OfMessage: &responses.EasyInputMessageParam{
				Role:    responses.EasyInputMessageRoleUser,
				Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: responses.ResponseInputMessageContentListParam{text(), image, file}},
			}},
			{OfInputMessage: &responses.ResponseInputItemMessageParam{
				Role:    "developer",
				Content: responses.ResponseInputMessageContentListParam{text()},
			}},
			{OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
				CallID: openai.String("call_1"),
				Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfResponseFunctionCallOutputItemArray: responses.ResponseFunctionCallOutputItemListParam{
					{OfInputText: &responses.ResponseInputTextContentParam{Text: "tool out", PromptCacheBreakpoint: responses.NewResponseInputTextContentPromptCacheBreakpointParam()}},
					{OfInputImage: &responses.ResponseInputImageContentParam{ImageURL: openai.String("https://example.com/b.png"), PromptCacheBreakpoint: responses.NewResponseInputImageContentPromptCacheBreakpointParam()}},
					{OfInputFile: &responses.ResponseInputFileContentParam{FileID: openai.String("file-2"), PromptCacheBreakpoint: responses.NewResponseInputFileContentPromptCacheBreakpointParam()}},
				}},
			}},
			{OfCustomToolCallOutput: &responses.ResponseCustomToolCallOutputParam{
				CallID: "call_2",
				Output: responses.ResponseCustomToolCallOutputOutputUnionParam{OfOutputContentList: []responses.ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam{
					{OfInputText: &responses.ResponseInputTextParam{Text: "custom out", PromptCacheBreakpoint: responses.NewResponseInputTextPromptCacheBreakpointParam()}},
				}},
			}},
		}},
	}
	req.SetExtraFields(map[string]any{"x_tb_test": "kept"})
	return req
}

// responsesPromptCacheBreakpoints is the number of breakpoints
// responsesRequestWithPromptCache sets.
const responsesPromptCacheBreakpoints = 8

func marshalResponsesParams(t *testing.T, req *responses.ResponseNewParams) (string, map[string]any) {
	t.Helper()
	b, err := json.Marshal(req)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	return string(b), raw
}

func TestResponsesPromptCacheFixtureCarriesEveryField(t *testing.T) {
	body, raw := marshalResponsesParams(t, responsesRequestWithPromptCache())
	assert.Contains(t, raw, "prompt_cache_options")
	assert.Contains(t, raw, "prompt_cache_retention")
	assert.Equal(t, responsesPromptCacheBreakpoints, strings.Count(body, `"prompt_cache_breakpoint"`),
		"the fixture must exercise every breakpoint position, or the strip tests prove nothing")
}

// A strict-schema Responses-compatible vendor gets none of the OpenAI-only
// fields — the Responses-shape counterpart of the #1548 NIM fix (#1561).
func TestApplyResponsesProviderTransforms_StripsPromptCacheForUnlistedVendor(t *testing.T) {
	req := responsesRequestWithPromptCache()

	ApplyResponsesProviderTransforms(req, nvidiaNIMURL)

	body, raw := marshalResponsesParams(t, req)
	assert.NotContains(t, raw, "prompt_cache_options")
	assert.NotContains(t, raw, "prompt_cache_retention")
	assert.NotContains(t, body, "prompt_cache_breakpoint")
	assert.Equal(t, "stable-affinity-key", raw["prompt_cache_key"], "prompt_cache_key is kept, as on the Chat shape")
	assert.Equal(t, "kept", raw["x_tb_test"], "request extra fields must survive the strip")
	assert.Contains(t, body, "stable prefix", "only the cache hints are removed, not the content")
}

func TestApplyResponsesProviderTransforms_AllowlistedProviderKeepsPromptCache(t *testing.T) {
	req := responsesRequestWithPromptCache()

	ApplyResponsesProviderTransforms(req, "https://api.openai.com/v1")

	body, raw := marshalResponsesParams(t, req)
	assert.Contains(t, raw, "prompt_cache_options")
	assert.Contains(t, raw, "prompt_cache_retention")
	assert.Equal(t, responsesPromptCacheBreakpoints, strings.Count(body, `"prompt_cache_breakpoint"`))
}

// The Responses and Chat shapes must agree on which providers keep the fields.
func TestApplyResponsesProviderTransforms_MatchesChatAllowlist(t *testing.T) {
	for _, url := range []string{"https://api.openai.com/v1", nvidiaNIMURL, "https://chatgpt.com/backend-api/codex", "https://openrouter.ai/api/v1", ""} {
		host, _ := SplitProviderHostPath(url)
		req := responsesRequestWithPromptCache()
		ApplyResponsesProviderTransforms(req, url)
		_, raw := marshalResponsesParams(t, req)
		_, kept := raw["prompt_cache_options"]
		assert.Equal(t, supportsExplicitPromptCache(host), kept, url)
	}
}

// nativeResponsesRequestWithEveryBreakpoint is a Responses request as a client
// sends it, with a prompt_cache_breakpoint at each of the 15 positions the
// request shape allows: prompt template variables, both message item shapes,
// function_call_output and custom_tool_call_output content — each with
// input_text, input_image and input_file.
const nativeResponsesRequestWithEveryBreakpoint = `{
  "model": "test",
  "prompt_cache_options": {"mode": "explicit"},
  "prompt_cache_retention": "24h",
  "prompt_cache_key": "stable-affinity-key",
  "prompt": {
    "id": "pmpt_test",
    "variables": {
      "context": {"type": "input_text", "text": "prefix", "prompt_cache_breakpoint": {"mode": "explicit"}},
      "diagram": {"type": "input_image", "image_url": "https://example.com/a.png", "detail": "auto", "prompt_cache_breakpoint": {"mode": "explicit"}},
      "spec": {"type": "input_file", "file_id": "file-1", "prompt_cache_breakpoint": {"mode": "explicit"}},
      "name": "plain string"
    }
  },
  "input": [
    {"type": "message", "role": "user", "content": [
      {"type": "input_text", "text": "a", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_image", "image_url": "https://example.com/b.png", "detail": "auto", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_file", "file_id": "file-2", "prompt_cache_breakpoint": {"mode": "explicit"}}
    ]},
    {"role": "developer", "content": [
      {"type": "input_text", "text": "b", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_image", "image_url": "https://example.com/c.png", "detail": "auto", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_file", "file_id": "file-3", "prompt_cache_breakpoint": {"mode": "explicit"}}
    ]},
    {"type": "function_call_output", "call_id": "call_1", "output": [
      {"type": "input_text", "text": "c", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_image", "image_url": "https://example.com/d.png", "detail": "auto", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_file", "file_id": "file-4", "prompt_cache_breakpoint": {"mode": "explicit"}}
    ]},
    {"type": "custom_tool_call_output", "call_id": "call_2", "output": [
      {"type": "input_text", "text": "d", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_image", "image_url": "https://example.com/e.png", "detail": "auto", "prompt_cache_breakpoint": {"mode": "explicit"}},
      {"type": "input_file", "file_id": "file-5", "prompt_cache_breakpoint": {"mode": "explicit"}}
    ]}
  ]
}`

// nativeResponsesRequest parses the fixture the way the gateway parses an
// incoming Responses request.
func nativeResponsesRequest(t *testing.T) *responses.ResponseNewParams {
	t.Helper()
	var req protocol.ResponseCreateRequest
	require.NoError(t, json.Unmarshal([]byte(nativeResponsesRequestWithEveryBreakpoint), &req))
	return req.ResponseNewParams
}

func TestApplyResponsesProviderTransforms_NativeJSON(t *testing.T) {
	body, _ := marshalResponsesParams(t, nativeResponsesRequest(t))
	require.Equal(t, 15, strings.Count(body, `"prompt_cache_breakpoint"`),
		"every breakpoint must survive unmarshalling, or the assertions below prove nothing")

	t.Run("stripped for an unlisted vendor", func(t *testing.T) {
		req := nativeResponsesRequest(t)
		ApplyResponsesProviderTransforms(req, "https://example-llm-provider.test/v1")
		body, raw := marshalResponsesParams(t, req)
		assert.NotContains(t, body, "prompt_cache_breakpoint")
		assert.NotContains(t, raw, "prompt_cache_options")
		assert.NotContains(t, raw, "prompt_cache_retention")
		assert.Equal(t, "stable-affinity-key", raw["prompt_cache_key"])
		assert.Contains(t, body, `"name":"plain string"`, "prompt variables themselves are kept")
	})

	t.Run("kept for the allowlisted vendor", func(t *testing.T) {
		req := nativeResponsesRequest(t)
		ApplyResponsesProviderTransforms(req, "https://api.openai.com/v1")
		body, _ := marshalResponsesParams(t, req)
		assert.Equal(t, 15, strings.Count(body, `"prompt_cache_breakpoint"`))
	})
}
