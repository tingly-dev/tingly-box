package ops

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const nvidiaNIMURL = "https://integrate.api.nvidia.com/v1"

// TestNVIDIAStripPromptCacheTopLevel proves that the top-level
// prompt_cache_options / prompt_cache_retention fields Claude Code sends are
// removed before the request is forwarded to NVIDIA NIM.
func TestNVIDIAStripPromptCacheTopLevel(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model:                openai.ChatModel("thinkingmachines/inkling"),
		PromptCacheOptions:   openai.ChatCompletionNewParamsPromptCacheOptions{Mode: "implicit"},
		PromptCacheRetention: openai.ChatCompletionNewParamsPromptCacheRetention("1400h"),
		Messages: []openai.ChatCompletionMessageParamUnion{{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Name:    openai.String("u"),
				Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String("hi")},
			},
		}},
	}

	ApplyProviderTransforms(req, nvidiaNIMURL, string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalParams(t, req)
	assert.NotContains(t, raw, "prompt_cache_options",
		"prompt_cache_options must be stripped for NVIDIA NIM")
	assert.NotContains(t, raw, "prompt_cache_retention",
		"prompt_cache_retention must be stripped for NVIDIA NIM")
}

// TestNVIDIAStripOnlyPromptCacheOptions proves stripping also works when only
// prompt_cache_options is set (retention absent).
func TestNVIDIAStripOnlyPromptCacheOptions(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model:              openai.ChatModel("thinkingmachines/inkling"),
		PromptCacheOptions: openai.ChatCompletionNewParamsPromptCacheOptions{Mode: "implicit"},
		Messages: []openai.ChatCompletionMessageParamUnion{{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Name:    openai.String("u"),
				Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String("hi")},
			},
		}},
	}

	ApplyProviderTransforms(req, nvidiaNIMURL, string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalParams(t, req)
	assert.NotContains(t, raw, "prompt_cache_options",
		"prompt_cache_options must be stripped for NVIDIA NIM")
	assert.NotContains(t, raw, "prompt_cache_retention",
		"prompt_cache_retention must not be injected when absent")
}

// TestNVIDIAStripPreservesMessageExtras proves that per-message extra fields
// (x_thinking / reasoning_content) survive the transform for NVIDIA NIM.
func TestNVIDIAStripPreservesMessageExtras(t *testing.T) {
	msg := assistantToolCallMessage(t)
	msg.OfAssistant.SetExtraFields(map[string]any{"x_thinking": "I need to call a tool"})

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("thinkingmachines/inkling"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, nvidiaNIMURL, string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	assert.Equal(t, "I need to call a tool", raw["x_thinking"],
		"x_thinking must survive the NVIDIA transform untouched")
	assert.NotContains(t, raw, "reasoning_content",
		"NVIDIA models must not get a DeepSeek-style reasoning_content injection")
}

// TestNVIDIAStripPreservesRequestExtras proves that request-level extra fields
// (e.g. thinking passthrough) survive the transform for NVIDIA NIM.
func TestNVIDIAStripPreservesRequestExtras(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("thinkingmachines/inkling"),
		Messages: []openai.ChatCompletionMessageParamUnion{{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Name:    openai.String("u"),
				Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String("hi")},
			},
		}},
	}
	req.SetExtraFields(map[string]any{"x_tb_test": "kept"})

	ApplyProviderTransforms(req, nvidiaNIMURL, string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalParams(t, req)
	assert.Equal(t, "kept", raw["x_tb_test"],
		"request-level extra fields must survive the NVIDIA transform")
}

// TestNVIDIANonNVIDIAProvidersUnaffected proves the transform only fires for
// NVIDIA NIM URLs.
func TestNVIDIANonNVIDIAProvidersUnaffected(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model:              openai.ChatModel("gpt-oss-20b"),
		PromptCacheOptions: openai.ChatCompletionNewParamsPromptCacheOptions{Mode: "implicit"},
	}
	req.SetExtraFields(map[string]any{"x_tb_test": "kept"})

	ApplyProviderTransforms(req, "https://api.openai.com/v1", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalParams(t, req)
	assert.Contains(t, raw, "prompt_cache_options",
		"non-NVIDIA providers must keep prompt_cache_options")
}

func marshalParams(t *testing.T, req *openai.ChatCompletionNewParams) map[string]any {
	t.Helper()

	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(reqBytes, &raw))
	return raw
}
