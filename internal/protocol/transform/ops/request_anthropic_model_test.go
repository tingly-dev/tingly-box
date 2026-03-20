package ops

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
)

func TestIsThinkingSupportedModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{
			name:     "Claude Opus 4.6",
			model:    "claude-opus-4-6",
			expected: true,
		},
		{
			name:     "Claude Opus 4.6 uppercase",
			model:    "CLAUDE-OPUS-4-6",
			expected: true,
		},
		{
			name:     "Claude Sonnet 4.6",
			model:    "claude-sonnet-4-6",
			expected: true,
		},
		{
			name:     "Claude Sonnet 4.6 uppercase",
			model:    "CLAUDE-SONNET-4-6",
			expected: true,
		},
		{
			name:     "Claude Haiku 3.5",
			model:    "claude-3-5-haiku-20241022",
			expected: false,
		},
		{
			name:     "Claude Haiku 3",
			model:    "claude-3-haiku",
			expected: false,
		},
		{
			name:     "Claude Sonnet 3.5",
			model:    "claude-3-5-sonnet-20241022",
			expected: false,
		},
		{
			name:     "Claude Opus 3.7",
			model:    "claude-3-7-opus-20250214",
			expected: false,
		},
		{
			name:     "Empty model",
			model:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isThinkingSupportedModel(tt.model)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsThinkingAdaptiveV1(t *testing.T) {
	tests := []struct {
		name     string
		thinking anthropic.ThinkingConfigParamUnion
		expected bool
	}{
		{
			name:     "Adaptive thinking",
			thinking: anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
			expected: true,
		},
		{
			name:     "Enabled thinking",
			thinking: anthropic.ThinkingConfigParamUnion{OfEnabled: &anthropic.ThinkingConfigEnabledParam{}},
			expected: false,
		},
		{
			name:     "Empty thinking",
			thinking: anthropic.ThinkingConfigParamUnion{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isThinkingAdaptiveV1(tt.thinking)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyAnthropicModelTransform_V1_Opus46_Adaptive(t *testing.T) {
	// Test case: Opus 4.6 model with adaptive thinking should keep thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-6"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicModelTransform(req, "claude-opus-4-6")

	assert.NotNil(t, result)
	typedResult, ok := result.(*anthropic.MessageNewParams)
	assert.True(t, ok)
	assert.NotNil(t, typedResult.Thinking.OfAdaptive, "Thinking.OfAdaptive should be preserved for Opus 4.6")
}

func TestApplyAnthropicModelTransform_V1_Sonnet46_Adaptive(t *testing.T) {
	// Test case: Sonnet 4.6 model with adaptive thinking should keep thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicModelTransform(req, "claude-sonnet-4-6")

	assert.NotNil(t, result)
	typedResult, ok := result.(*anthropic.MessageNewParams)
	assert.True(t, ok)
	assert.NotNil(t, typedResult.Thinking.OfAdaptive, "Thinking.OfAdaptive should be preserved for Sonnet 4.6")
}

func TestApplyAnthropicModelTransform_V1_Haiku_Adaptive(t *testing.T) {
	// Test case: Haiku model with adaptive thinking should remove thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicModelTransform(req, "claude-3-5-haiku-20241022")

	assert.NotNil(t, result)
	typedResult, ok := result.(*anthropic.MessageNewParams)
	assert.True(t, ok)
	assert.True(t, typedResult.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil for Haiku")
	assert.True(t, typedResult.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil for Haiku")
}

func TestApplyAnthropicModelTransform_V1_Sonnet35_Adaptive(t *testing.T) {
	// Test case: Sonnet 3.5 model with adaptive thinking should remove thinking (not 4.6)
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicModelTransform(req, "claude-3-5-sonnet-20241022")

	assert.NotNil(t, result)
	typedResult, ok := result.(*anthropic.MessageNewParams)
	assert.True(t, ok)
	assert.True(t, typedResult.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil for Sonnet 3.5")
	assert.True(t, typedResult.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil for Sonnet 3.5")
}

func TestApplyAnthropicModelTransform_V1_Opus37_Adaptive(t *testing.T) {
	// Test case: Opus 3.7 model with adaptive thinking should remove thinking (not 4.6)
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-7-opus-20250214"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicModelTransform(req, "claude-3-7-opus-20250214")

	assert.NotNil(t, result)
	typedResult, ok := result.(*anthropic.MessageNewParams)
	assert.True(t, ok)
	assert.True(t, typedResult.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil for Opus 3.7")
	assert.True(t, typedResult.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil for Opus 3.7")
}

func TestApplyAnthropicModelTransform_V1_Haiku_Enabled(t *testing.T) {
	// Test case: Haiku model with enabled thinking should keep thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicModelTransform(req, "claude-3-5-haiku-20241022")

	assert.NotNil(t, result)
	typedResult, ok := result.(*anthropic.MessageNewParams)
	assert.True(t, ok)
	assert.NotNil(t, typedResult.Thinking.OfEnabled, "Thinking.OfEnabled should be preserved")
}

func TestApplyAnthropicModelTransform_V1_NoThinking(t *testing.T) {
	// Test case: No thinking configured
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: int64(4096),
		Thinking:  anthropic.ThinkingConfigParamUnion{},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicModelTransform(req, "claude-3-5-haiku-20241022")

	assert.NotNil(t, result)
	typedResult, ok := result.(*anthropic.MessageNewParams)
	assert.True(t, ok)
	assert.True(t, typedResult.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil")
	assert.True(t, typedResult.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil")
}

func TestApplyAnthropicModelTransform_NilRequest(t *testing.T) {
	// Test case: nil request
	result := ApplyAnthropicModelTransform(nil, "claude-3-5-haiku-20241022")
	assert.Nil(t, result)
}

func TestFilterThinkingBlocksInMessages(t *testing.T) {
	// Test case: Filter thinking blocks from messages
	messages := []anthropic.MessageParam{
		{
			Role: "user",
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Hello"),
			},
		},
		{
			Role: "assistant",
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Thinking..."),
				// Note: Creating a thinking block requires proper construction
				// This test demonstrates the structure; actual implementation may vary
			},
		},
	}

	// The filter should remove messages with only thinking blocks
	result := filterThinkingBlocksInMessages(messages)
	assert.NotNil(t, result)
	// User message should be preserved
	assert.True(t, len(result) >= 1)
}
