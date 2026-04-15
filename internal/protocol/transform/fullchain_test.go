package transform

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newFullChainContext creates a TransformContext with common fields for full-chain tests.
func newFullChainContext(request interface{}, providerURL string, extra map[string]interface{}) *TransformContext {
	return &TransformContext{
		Request:         request,
		OriginalRequest: request,
		ProviderURL:     providerURL,
		IsStreaming:     true,
		ScenarioFlags:   &typ.ScenarioFlags{},
		TransformSteps:  []string{},
		Extra:           extra,
	}
}

// anthropicExtra returns the minimum extra map required by Anthropic vendor transforms.
func anthropicExtra() map[string]interface{} {
	return map[string]interface{}{
		"device":  "integration-test-device",
		"user_id": "integration-test-account-uuid",
	}
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (passthrough, Opus 4.6)
// =============================================

func TestFullChain_AnthropicV1_To_AnthropicV1_Passthrough(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("api.anthropic.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-6-20250514"),
		MaxTokens: 16384,
		System: []anthropic.TextBlockParam{
			{Text: "You are a helpful assistant."},
		},
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"base_convert", "consistency_normalize", "vendor_adjust"}, result.TransformSteps)

	// Same type in, same type out
	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Adaptive thinking preserved for Opus 4.6
	assert.NotNil(t, out.Thinking.OfAdaptive, "adaptive thinking should be preserved for Opus 4.6")

	// Billing header injected
	require.NotEmpty(t, out.System)
	assert.True(t, strings.HasPrefix(out.System[0].Text, "x-anthropic-billing-header:"), "first system block should be billing header")
	assert.Equal(t, "You are a helpful assistant.", out.System[1].Text, "original system prompt preserved")

	// Metadata user_id injected
	assert.True(t, out.Metadata.UserID.Valid(), "user_id metadata should be set")
	uid := out.Metadata.UserID.String()
	assert.Contains(t, uid, "integration-test-device")
	assert.Contains(t, uid, "integration-test-account-uuid")
	assert.Contains(t, uid, "session_id")

	// Original request preserved
	assert.Equal(t, req, result.OriginalRequest)
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (Haiku, unsupported model)
// =============================================

func TestFullChain_AnthropicV1_Haiku_AdaptiveStripped(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("api.anthropic.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Adaptive thinking stripped for Haiku
	assert.Nil(t, out.Thinking.OfAdaptive, "adaptive thinking should be stripped for Haiku")
	assert.Nil(t, out.Thinking.OfEnabled, "enabled thinking should also be nil")

	// But billing header and metadata still injected
	require.NotEmpty(t, out.System)
	assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
	assert.True(t, out.Metadata.UserID.Valid())
}

// =============================================
// Full Chain: Anthropic Beta → Anthropic Beta (passthrough, Sonnet 4.6)
// =============================================

func TestFullChain_AnthropicBeta_To_AnthropicBeta_Passthrough(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicBeta),
		NewConsistencyTransform(protocol.TypeAnthropicBeta),
		NewVendorTransform("api.anthropic.com"),
	})

	req := newBetaRequest("claude-sonnet-4-6-20250514", anthropic.BetaThinkingConfigParamUnion{
		OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
	})

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)

	// Adaptive thinking preserved for Sonnet 4.6
	assert.NotNil(t, out.Thinking.OfAdaptive)

	// Billing header injected
	require.NotEmpty(t, out.System)
	assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")

	// Metadata user_id injected
	assert.True(t, out.Metadata.UserID.Valid())
	uid := out.Metadata.UserID.String()
	assert.Contains(t, uid, "integration-test-device")
}

// =============================================
// Full Chain: Anthropic Beta → Codex (Responses)
// Most critical path: full thinking→reasoning, tool transforms
// =============================================

func TestFullChain_AnthropicBeta_To_Codex_AdaptiveThinking(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeOpenAIResponses),
		NewConsistencyTransform(protocol.TypeOpenAIResponses),
		NewVendorTransform(protocol.CodexAPIBase),
	})

	req := newBetaRequest("claude-sonnet-4-6-20250514", anthropic.BetaThinkingConfigParamUnion{
		OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
	})

	ctx := newFullChainContext(req, protocol.CodexAPIBase, anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"base_convert", "consistency_normalize", "vendor_adjust"}, result.TransformSteps)

	// Output must be Responses API type
	resp, ok := result.Request.(*responses.ResponseNewParams)
	require.True(t, ok, "expected *responses.ResponseNewParams, got %T", result.Request)

	// Codex sets store=false and includes reasoning.encrypted_content
	require.Len(t, resp.Include, 1)
	assert.Equal(t, responses.ResponseIncludable("reasoning.encrypted_content"), resp.Include[0],
		"Codex must include reasoning.encrypted_content")
}

func TestFullChain_AnthropicBeta_To_Codex_EnabledThinking(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeOpenAIResponses),
		NewConsistencyTransform(protocol.TypeOpenAIResponses),
		NewVendorTransform(protocol.CodexAPIBase),
	})

	req := newBetaRequest("claude-sonnet-4-6-20250514", anthropic.BetaThinkingConfigParamUnion{
		OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{
			BudgetTokens: 10000,
		},
	})

	ctx := newFullChainContext(req, protocol.CodexAPIBase, anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	resp, ok := result.Request.(*responses.ResponseNewParams)
	require.True(t, ok)

	// Budget 10000 → effort "medium" per convertBudgetToEffort
	// Reasoning config should be set
	require.Len(t, resp.Include, 1)
	assert.Equal(t, responses.ResponseIncludable("reasoning.encrypted_content"), resp.Include[0])

	// Model preserved through the chain
	assert.Contains(t, string(resp.Model), "claude-sonnet-4-6")
}

// =============================================
// Full Chain: Anthropic v1 → Responses (Codex, no Codex-specific transforms)
// Codex vendor only activates when OriginalRequest is BetaMessageNewParams.
// V1 → Responses goes through base conversion only (no Codex reasoning transform).
// =============================================

func TestFullChain_AnthropicV1_To_Responses_ProtocolConversion(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeOpenAIResponses),
		NewConsistencyTransform(protocol.TypeOpenAIResponses),
		NewVendorTransform(protocol.CodexAPIBase),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6-20250514"),
		MaxTokens: 16384,
		System: []anthropic.TextBlockParam{
			{Text: "You are a helpful assistant."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("What is 2+2?")),
		},
	}

	ctx := newFullChainContext(req, protocol.CodexAPIBase, anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	// Output must be Responses API type
	resp, ok := result.Request.(*responses.ResponseNewParams)
	require.True(t, ok, "expected *responses.ResponseNewParams, got %T", result.Request)

	// Model preserved
	assert.Contains(t, string(resp.Model), "claude-sonnet-4-6")

	// System prompt converted to instructions
	assert.Contains(t, string(resp.Instructions.Value), "You are a helpful assistant.")
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (non-Anthropic provider, no vendor transform)
// =============================================

func TestFullChain_AnthropicV1_NonAnthropicProvider_NoVendorTransform(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("api.openai.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := newFullChainContext(req, "api.openai.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Non-Anthropic provider: adaptive thinking preserved (no vendor transform applied)
	assert.NotNil(t, out.Thinking.OfAdaptive, "adaptive should be preserved for non-Anthropic provider")

	// No billing header or metadata injection
	assert.Empty(t, out.System, "no billing header for non-Anthropic provider")
	assert.False(t, out.Metadata.UserID.Valid(), "no metadata injection for non-Anthropic provider")
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (multi-turn with thinking blocks)
// =============================================

func TestFullChain_AnthropicV1_MultiTurn_ThinkingBlocksFiltered(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("api.anthropic.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("What is 2+2?")),
			{
				Role: "assistant",
				Content: []anthropic.ContentBlockParamUnion{
					{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "Let me calculate...", Signature: "sig_abc"}},
					{OfText: &anthropic.TextBlockParam{Text: "The answer is 4."}},
				},
			},
			anthropic.NewUserMessage(anthropic.NewTextBlock("What about 3+3?")),
			{
				Role: "assistant",
				Content: []anthropic.ContentBlockParamUnion{
					{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "Another calculation...", Signature: "sig_def"}},
					{OfText: &anthropic.TextBlockParam{Text: "The answer is 6."}},
				},
			},
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// All 4 messages preserved
	require.Len(t, out.Messages, 4)

	// Thinking blocks removed from both assistant messages, text blocks preserved
	for _, msgIdx := range []int{1, 3} {
		for _, block := range out.Messages[msgIdx].Content {
			assert.Nil(t, block.OfThinking, "thinking block should be filtered in message %d", msgIdx)
		}
		foundText := false
		for _, block := range out.Messages[msgIdx].Content {
			if block.OfText != nil {
				foundText = true
				break
			}
		}
		assert.True(t, foundText, "text block should be preserved in message %d", msgIdx)
	}
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (Claude AI provider URL)
// =============================================

func TestFullChain_AnthropicV1_ClaudeAI_ProviderURL(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("https://claude.ai/api/v1/messages"),
	})

	req := newAnthropicV1Request("claude-opus-4-6-20250514", 16384)

	ctx := newFullChainContext(req, "https://claude.ai/api/v1/messages", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// claude.ai URL should match Anthropic provider
	require.NotEmpty(t, out.System)
	assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
	assert.True(t, out.Metadata.UserID.Valid())
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (empty model fails consistency validation)
// =============================================

func TestFullChain_AnthropicV1_EmptyModel_ValidationFails(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("api.anthropic.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     "",
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	_, err := chain.Execute(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consistency_normalize")
	assert.Contains(t, err.Error(), "model is required")
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (existing billing header replaced)
// =============================================

func TestFullChain_AnthropicV1_ExistingBillingHeader_Replaced(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("api.anthropic.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-6-20250514"),
		MaxTokens: 16384,
		System: []anthropic.TextBlockParam{
			{Text: "x-anthropic-billing-header: cc_version=old_version; cc_entrypoint=gui; cch=old_hash;"},
			{Text: "You are a coding assistant."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Help me code")),
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Still 2 system blocks: replaced billing header + original prompt
	require.Len(t, out.System, 2)
	assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
	assert.NotContains(t, out.System[0].Text, "old_version", "old billing header should be replaced")
	assert.NotContains(t, out.System[0].Text, "old_hash")
	assert.Contains(t, out.System[0].Text, "cc_entrypoint=cli", "new billing header should have cli entrypoint")
	assert.Equal(t, "You are a coding assistant.", out.System[1].Text)
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (enabled thinking preserved on Haiku)
// =============================================

func TestFullChain_AnthropicV1_Haiku_ExplicitThinkingPreserved(t *testing.T) {
	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
		NewConsistencyTransform(protocol.TypeAnthropicV1),
		NewVendorTransform("api.anthropic.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 8192,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: 5000,
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Explicitly enabled thinking should be preserved even on unsupported models
	assert.NotNil(t, out.Thinking.OfEnabled, "explicitly enabled thinking should be preserved on Haiku")
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (multiple supported models table-driven)
// =============================================

func TestFullChain_AnthropicV1_SupportedModels(t *testing.T) {
	models := []string{
		"claude-opus-4-6",
		"claude-opus-4-6-20250514",
		"claude-sonnet-4-6",
		"claude-sonnet-4-6-20250514",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			chain := NewTransformChain([]Transform{
				NewBaseTransform(protocol.TypeAnthropicV1),
				NewConsistencyTransform(protocol.TypeAnthropicV1),
				NewVendorTransform("api.anthropic.com"),
			})

			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(model),
				MaxTokens: 16384,
				Thinking: anthropic.ThinkingConfigParamUnion{
					OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
				},
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
				},
			}

			ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			out, ok := result.Request.(*anthropic.MessageNewParams)
			require.True(t, ok)

			assert.NotNil(t, out.Thinking.OfAdaptive,
				"adaptive thinking should be preserved for %s", model)

			// Billing header and metadata should always be present for Anthropic provider
			require.NotEmpty(t, out.System)
			assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
			assert.True(t, out.Metadata.UserID.Valid())
		})
	}
}

// =============================================
// Full Chain: Anthropic v1 → Anthropic v1 (multiple unsupported models table-driven)
// =============================================

func TestFullChain_AnthropicV1_UnsupportedModels(t *testing.T) {
	models := []string{
		"claude-3-5-haiku-20241022",
		"claude-3-5-sonnet-20241022",
		"claude-3-7-opus-20250214",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			chain := NewTransformChain([]Transform{
				NewBaseTransform(protocol.TypeAnthropicV1),
				NewConsistencyTransform(protocol.TypeAnthropicV1),
				NewVendorTransform("api.anthropic.com"),
			})

			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(model),
				MaxTokens: 4096,
				Thinking: anthropic.ThinkingConfigParamUnion{
					OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
				},
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
				},
			}

			ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			out, ok := result.Request.(*anthropic.MessageNewParams)
			require.True(t, ok)

			assert.Nil(t, out.Thinking.OfAdaptive,
				"adaptive thinking should be stripped for %s", model)
			assert.Nil(t, out.Thinking.OfEnabled)

			// But billing header and metadata still injected
			require.NotEmpty(t, out.System)
			assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
			assert.True(t, out.Metadata.UserID.Valid())
		})
	}
}

// =============================================
// Full Chain: Transform steps invariant
// =============================================

func TestFullChain_Steps_Order(t *testing.T) {
	tests := []struct {
		name       string
		targetType protocol.APIType
		provider   string
		wantSteps  []string
	}{
		{
			name:       "Anthropic v1 passthrough",
			targetType: protocol.TypeAnthropicV1,
			provider:   "api.anthropic.com",
			wantSteps:  []string{"base_convert", "consistency_normalize", "vendor_adjust"},
		},
		{
			name:       "Anthropic beta passthrough",
			targetType: protocol.TypeAnthropicBeta,
			provider:   "api.anthropic.com",
			wantSteps:  []string{"base_convert", "consistency_normalize", "vendor_adjust"},
		},
		{
			name:       "Beta to Codex Responses",
			targetType: protocol.TypeOpenAIResponses,
			provider:   protocol.CodexAPIBase,
			wantSteps:  []string{"base_convert", "consistency_normalize", "vendor_adjust"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewTransformChain([]Transform{
				NewBaseTransform(tt.targetType),
				NewConsistencyTransform(tt.targetType),
				NewVendorTransform(tt.provider),
			})

			var req interface{}
			switch tt.targetType {
			case protocol.TypeAnthropicV1:
				req = newAnthropicV1Request("claude-opus-4-6-20250514", 16384)
			case protocol.TypeAnthropicBeta:
				req = newBetaRequest("claude-opus-4-6-20250514", anthropic.BetaThinkingConfigParamUnion{
					OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
				})
			case protocol.TypeOpenAIResponses:
				req = newBetaRequest("claude-sonnet-4-6-20250514", anthropic.BetaThinkingConfigParamUnion{})
			}

			ctx := newFullChainContext(req, tt.provider, anthropicExtra())

			result, err := chain.Execute(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSteps, result.TransformSteps)
		})
	}
}
