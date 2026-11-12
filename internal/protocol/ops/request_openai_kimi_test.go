package ops

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestDeepSeekAndKimiEffortTiersAreIndependentMaps proves deepSeekEffortTiers
// and kimiEffortTiers are distinct map values, not two variables aliasing
// the same underlying Go map (lowHighMaxEffortTiers() returns a fresh map on
// each call precisely to guarantee this) — mutating one must never affect
// the other, since the whole point of splitting them was to let either
// vendor's tier scheme diverge independently.
func TestDeepSeekAndKimiEffortTiersAreIndependentMaps(t *testing.T) {
	original := kimiEffortTiers["max"]
	kimiEffortTiers["max"] = "high" // simulate Kimi-only divergence
	defer func() { kimiEffortTiers["max"] = original }()

	assert.Equal(t, "high", kimiEffortTiers["max"])
	assert.NotEqual(t, "high", deepSeekEffortTiers["max"],
		"mutating kimiEffortTiers must not affect deepSeekEffortTiers")
}

// TestKimiTransformReasoningContentConversion proves that Moonshot/Kimi gets
// the same x_thinking -> reasoning_content message-shape conversion as
// DeepSeek.
func TestKimiTransformReasoningContentConversion(t *testing.T) {
	msg := assistantToolCallMessage(t)
	msg.OfAssistant.SetExtraFields(map[string]any{"x_thinking": "reasoning for kimi"})

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("kimi-k3"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://api.moonshot.cn/v1", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	assert.Equal(t, "reasoning for kimi", raw["reasoning_content"],
		"x_thinking should be converted to reasoning_content")
	assert.NotContains(t, raw, "x_thinking",
		"x_thinking should be removed after conversion")
}

// TestKimiTransformMatchesHostRegardlessOfPath proves api.kimi.com dispatch
// is host-only, not scoped to /coding/v1: it's Moonshot's own dedicated
// host, and the wire protocol is a property of the vendor/model, not the
// product path on that host (unlike opencode.ai, a multi-vendor relay where
// the path is load-bearing — see the opencode.ai test in
// request_openai_deepseek_test.go).
func TestKimiTransformMatchesHostRegardlessOfPath(t *testing.T) {
	msg := assistantToolCallMessage(t)
	msg.OfAssistant.SetExtraFields(map[string]any{"x_thinking": "reasoning for kimi"})

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("kimi-k3"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://api.kimi.com/some-other-product", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	assert.Equal(t, "reasoning for kimi", raw["reasoning_content"])
}

// TestKimiReasoningEffortCollapsesOntoThreeTiers proves that Kimi K3 gets
// the same low/high/max reasoning_effort collapse as DeepSeek, since both
// vendors document the identical three-tier scheme.
func TestKimiReasoningEffortCollapsesOntoThreeTiers(t *testing.T) {
	tests := []struct {
		ladderEffort string
		want         string
	}{
		{"minimal", "low"},
		{"low", "low"},
		{"medium", "high"},
		{"high", "high"},
		{"xhigh", "max"},
		{"max", "max"},
	}

	urls := []string{
		"https://api.moonshot.cn/v1",
		"https://api.moonshot.ai/v1",
		"https://api.kimi.com/coding/v1",
	}

	for _, url := range urls {
		for _, tt := range tests {
			t.Run(url+"/"+tt.ladderEffort, func(t *testing.T) {
				req := &openai.ChatCompletionNewParams{
					Model:           openai.ChatModel("kimi-k3"),
					ReasoningEffort: openai.ReasoningEffort(tt.ladderEffort),
				}

				ApplyProviderTransforms(req, url, string(req.Model), &protocol.OpenAIConfig{})

				assert.Equal(t, tt.want, string(req.ReasoningEffort))
			})
		}
	}
}

// TestKimiReasoningEffortFromConfigDefault proves that the effort derived
// during Anthropic→OpenAI conversion (config.ReasoningEffort) reaches Kimi
// the same way it reaches DeepSeek.
func TestKimiReasoningEffortFromConfigDefault(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("kimi-k3"),
	}

	ApplyProviderTransforms(req, "https://api.moonshot.cn/v1", string(req.Model), &protocol.OpenAIConfig{
		HasThinking:     true,
		ReasoningEffort: "medium",
	})

	assert.Equal(t, "high", string(req.ReasoningEffort))
}

// TestKimiReasoningEffortNoSignalLeavesUnset proves that Kimi is not forced
// into thinking mode when neither the request nor the config carries an
// actionable thinking signal.
func TestKimiReasoningEffortNoSignalLeavesUnset(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("kimi-k3"),
	}

	ApplyProviderTransforms(req, "https://api.moonshot.cn/v1", string(req.Model), &protocol.OpenAIConfig{})

	assert.Equal(t, "", string(req.ReasoningEffort))
}
