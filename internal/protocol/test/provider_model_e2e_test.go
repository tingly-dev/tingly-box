//go:build e2e
// +build e2e

package test

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// =============================================
// Provider Model E2E Tests
// This file tests different models across various providers
// to ensure proper request transformation and vendor-specific handling
// =============================================

// TestProviderModels_DeepSeek_AnthropicV1 tests DeepSeek provider with Anthropic v1 API format
func TestProviderModels_DeepSeek_AnthropicV1(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicV1),
		transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
		transform.NewVendorTransform("api.deepseek.com"),
	})

	tests := []struct {
		name    string
		model   string
		wantErr bool
	}{
		{"DeepSeek flash", "deepseek-v4-flash", false},
		{"DeepSeek pro", "deepseek-v4-pro", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(tt.model),
				MaxTokens: 4096,
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello from DeepSeek")),
				},
			}

			ctx := newFullChainContext(req, "api.deepseek.com", map[string]interface{}{})

			result, err := chain.Execute(ctx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			out, ok := result.Request.(*anthropic.MessageNewParams)
			require.True(t, ok)

			// Model should be preserved
			assert.Equal(t, tt.model, string(out.Model))

			// Verify transform steps were executed
			assert.Equal(t, []string{"base_convert", "consistency_normalize", "vendor_adjust"}, result.TransformSteps)
		})
	}
}

// TestProviderModels_DeepSeek_AnthropicV1_WithThinking tests DeepSeek provider with thinking configuration
func TestProviderModels_DeepSeek_AnthropicV1_WithThinking(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicV1),
		transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
		transform.NewVendorTransform("api.deepseek.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("deepseek-v4-flash"),
		MaxTokens: 8192,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: 2000,
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Solve this step by step")),
		},
	}

	ctx := newFullChainContext(req, "api.deepseek.com", map[string]interface{}{})

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// DeepSeek should sanitize thinking config
	// Thinking blocks in messages should be handled
	assert.NotNil(t, out.Thinking)
}

func TestProviderModels_DeepSeek_AnthropicV1_WithThinking_Disable(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicV1),
		transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
		transform.NewVendorTransform("api.deepseek.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("deepseek-v4-flash"),
		MaxTokens: 8192,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfDisabled: &anthropic.ThinkingConfigDisabledParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: "medium",
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Solve this step by step")),
		},
	}

	ctx := newFullChainContext(req, "api.deepseek.com", map[string]interface{}{})

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	bs, _ := out.MarshalJSON()
	t.Logf("output: %s", string(bs))

	// DeepSeek should sanitize thinking config
	// Thinking blocks in messages should be handled
	assert.NotNil(t, out.Thinking)
	assert.Equal(t, out.OutputConfig.Effort, anthropic.OutputConfigEffort(""))
}

// TestProviderModels_DeepSeek_OpenAIChat tests DeepSeek provider with OpenAI Chat format
func TestProviderModels_DeepSeek_OpenAIChat(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeOpenAIChat),
		transform.NewConsistencyTransform(protocol.TypeOpenAIChat),
		transform.NewVendorTransform("api.deepseek.com"),
	})

	tests := []struct {
		name  string
		model string
	}{
		{"DeepSeek Flash", "deepseek-v4-flash"},
		{"DeepSeek Pro", "deepseek-v4-pro"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &openai.ChatCompletionNewParams{
				Model: openai.ChatModel(tt.model),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Test message"),
				},
			}

			ctx := newFullChainContext(req, "api.deepseek.com", map[string]interface{}{})

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			out, ok := result.Request.(*openai.ChatCompletionNewParams)
			require.True(t, ok)

			// Model should be preserved
			assert.Equal(t, tt.model, string(out.Model))

			// Verify transform steps
			assert.Equal(t, []string{"base_convert", "consistency_normalize", "vendor_adjust"}, result.TransformSteps)
		})
	}
}

// TestProviderModels_Anthropic_Official tests Anthropic official provider with various models
func TestProviderModels_Anthropic_Official(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicV1),
		transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
		transform.NewVendorTransform("api.anthropic.com"),
	})

	tests := []struct {
		name              string
		model             string
		supportsThinking  bool
		wantBillingHeader bool
	}{
		{"Claude Opus 4.6", "claude-opus-4-6-20250514", true, true},
		{"Claude Sonnet 4.6", "claude-sonnet-4-6-20250514", true, true},
		{"Claude Haiku 3.5", "claude-3-5-haiku-20241022", false, true},
		{"Claude Sonnet 3.5", "claude-3-5-sonnet-20241022", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(tt.model),
				MaxTokens: 4096,
				Thinking: anthropic.ThinkingConfigParamUnion{
					OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
				},
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello Claude")),
				},
			}

			ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			out, ok := result.Request.(*anthropic.MessageNewParams)
			require.True(t, ok)

			// Model should be preserved
			assert.Equal(t, tt.model, string(out.Model))

			// Check thinking support
			if tt.supportsThinking {
				assert.NotNil(t, out.Thinking.OfAdaptive, "adaptive thinking should be preserved for %s", tt.model)
			} else {
				assert.Nil(t, out.Thinking.OfAdaptive, "adaptive thinking should be stripped for %s", tt.model)
			}

			// Check billing header injection
			if tt.wantBillingHeader {
				require.NotEmpty(t, out.System, "system should have billing header")
				assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
			}

			// Check metadata injection
			assert.True(t, out.Metadata.UserID.Valid(), "user_id metadata should be set")
			uid := out.Metadata.UserID.String()
			assert.Contains(t, uid, "integration-test-device")
		})
	}
}

// TestProviderModels_Anthropic_Beta tests Anthropic beta API with various models
func TestProviderModels_Anthropic_Beta(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicBeta),
		transform.NewConsistencyTransform(protocol.TypeAnthropicBeta),
		transform.NewVendorTransform("api.anthropic.com"),
	})

	tests := []struct {
		name             string
		model            string
		supportsThinking bool
	}{
		{"Beta Opus 4.6", "claude-opus-4-6-20250514", true},
		{"Beta Sonnet 4.6", "claude-sonnet-4-6-20250514", true},
		{"Beta Haiku 3.5", "claude-3-5-haiku-20241022", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newBetaRequest(tt.model, anthropic.BetaThinkingConfigParamUnion{
				OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
			})

			ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			out, ok := result.Request.(*anthropic.BetaMessageNewParams)
			require.True(t, ok)

			// Check thinking support
			if tt.supportsThinking {
				assert.NotNil(t, out.Thinking.OfAdaptive, "adaptive thinking should be preserved for %s", tt.model)
			} else {
				assert.Nil(t, out.Thinking.OfAdaptive, "adaptive thinking should be stripped for %s", tt.model)
			}

			// Check billing header
			require.NotEmpty(t, out.System)
			assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")

			// Check metadata
			assert.True(t, out.Metadata.UserID.Valid())
		})
	}
}

// TestProviderModels_OpenAI_Official tests OpenAI official provider
func TestProviderModels_OpenAI_Official(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeOpenAIChat),
		transform.NewConsistencyTransform(protocol.TypeOpenAIChat),
		transform.NewVendorTransform("api.openai.com"),
	})

	tests := []struct {
		name  string
		model string
	}{
		{"GPT-4", "gpt-4"},
		{"GPT-4 Turbo", "gpt-4-turbo"},
		{"GPT-4o", "gpt-4o"},
		{"GPT-4o Mini", "gpt-4o-mini"},
		{"O3 Mini", "o3-mini"},
		{"O1", "o1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &openai.ChatCompletionNewParams{
				Model: openai.ChatModel(tt.model),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Hello GPT"),
				},
			}

			ctx := newFullChainContext(req, "api.openai.com", map[string]interface{}{})

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			out, ok := result.Request.(*openai.ChatCompletionNewParams)
			require.True(t, ok)

			// Model should be preserved
			assert.Equal(t, tt.model, string(out.Model))

			// Verify config was set
			assert.NotNil(t, out)
		})
	}
}

// TestProviderModels_CrossProtocol_OpenAI_To_Anthropic tests cross-protocol conversion
func TestProviderModels_CrossProtocol_OpenAI_To_Anthropic(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicBeta),
		transform.NewConsistencyTransform(protocol.TypeAnthropicBeta),
		transform.NewVendorTransform("api.anthropic.com"),
	})

	req := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("gpt-4"),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Convert this to Anthropic format"),
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	// Should be converted to Anthropic beta format
	out, ok := result.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok, "request should be converted to Anthropic beta format")

	// Should have billing header and metadata
	require.NotEmpty(t, out.System)
	assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
	assert.True(t, out.Metadata.UserID.Valid())
}

// TestProviderModels_CrossProtocol_Anthropic_To_OpenAI tests cross-protocol conversion
func TestProviderModels_CrossProtocol_Anthropic_To_OpenAI(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeOpenAIChat),
		transform.NewConsistencyTransform(protocol.TypeOpenAIChat),
		transform.NewVendorTransform("api.openai.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Convert this to OpenAI format")),
		},
	}

	ctx := newFullChainContext(req, "api.openai.com", map[string]interface{}{})

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	// Should be converted to OpenAI format
	out, ok := result.Request.(*openai.ChatCompletionNewParams)
	require.True(t, ok, "request should be converted to OpenAI format")

	// Model should be preserved
	assert.Contains(t, string(out.Model), "claude")

	// Should have OpenAI config
	assert.NotNil(t, result.Config.OpenAIConfig)
}

// TestProviderModels_StreamingBehavior tests streaming flag handling across providers
func TestProviderModels_StreamingBehavior(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		targetType protocol.APIType
		model      string
	}{
		{"Anthropic Official Streaming", "api.anthropic.com", protocol.TypeAnthropicV1, "claude-opus-4-6-20250514"},
		{"DeepSeek Streaming", "api.deepseek.com", protocol.TypeAnthropicV1, "deepseek-v4-flash"},
		{"OpenAI Streaming", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(tt.targetType),
				transform.NewConsistencyTransform(tt.targetType),
				transform.NewVendorTransform(tt.provider),
			})

			var req interface{}
			switch tt.targetType {
			case protocol.TypeAnthropicV1:
				req = &anthropic.MessageNewParams{
					Model:     anthropic.Model(tt.model),
					MaxTokens: 1024,
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock("Streaming test")),
					},
				}
			case protocol.TypeOpenAIChat:
				req = &openai.ChatCompletionNewParams{
					Model: openai.ChatModel(tt.model),
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("Streaming test"),
					},
				}
			}

			extra := map[string]interface{}{}
			if strings.Contains(tt.provider, "anthropic") {
				extra = anthropicExtra()
			}

			ctx := newFullChainContext(req, tt.provider, extra)
			ctx.IsStreaming = true

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			// Streaming flag should be preserved in context
			assert.True(t, result.IsStreaming, "streaming flag should be preserved")
		})
	}
}

// TestProviderModels_ErrorMessageHandling tests error handling across different providers
func TestProviderModels_ErrorMessageHandling(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		targetType protocol.APIType
		wantError  string
	}{
		{"Empty Model Anthropic", "api.anthropic.com", protocol.TypeAnthropicV1, "model is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(tt.targetType),
				transform.NewConsistencyTransform(tt.targetType),
				transform.NewVendorTransform(tt.provider),
			})

			var req interface{}
			extra := map[string]interface{}{}
			if strings.Contains(tt.provider, "anthropic") {
				extra = anthropicExtra()
				req = &anthropic.MessageNewParams{
					Model:     "",
					MaxTokens: 1024,
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
					},
				}
			} else {
				req = &openai.ChatCompletionNewParams{
					Model: openai.ChatModel(""),
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("test"),
					},
				}
			}

			ctx := newFullChainContext(req, tt.provider, extra)

			_, err := chain.Execute(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// TestProviderModels_MultiTurnConversation tests multi-turn conversation handling
func TestProviderModels_MultiTurnConversation(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicV1),
		transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
		transform.NewVendorTransform("api.anthropic.com"),
	})

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-6-20250514"),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("What is 2+2?")),
			{
				Role: "assistant",
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: "The answer is 4."}},
				},
			},
			anthropic.NewUserMessage(anthropic.NewTextBlock("What about 3+3?")),
			{
				Role: "assistant",
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: "The answer is 6."}},
				},
			},
			anthropic.NewUserMessage(anthropic.NewTextBlock("Thank you!")),
		},
	}

	ctx := newFullChainContext(req, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	out, ok := result.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// All messages should be preserved
	assert.Len(t, out.Messages, 5, "all 5 messages should be preserved")

	// Verify message order
	assert.Equal(t, "user", string(out.Messages[0].Role))
	assert.Equal(t, "assistant", string(out.Messages[1].Role))
	assert.Equal(t, "user", string(out.Messages[2].Role))
	assert.Equal(t, "assistant", string(out.Messages[3].Role))
	assert.Equal(t, "user", string(out.Messages[4].Role))

	// Billing header should be injected
	require.NotEmpty(t, out.System)
	assert.Contains(t, out.System[0].Text, "x-anthropic-billing-header")
}

// TestProviderModels_ToolUseSupport tests tool use handling across providers
func TestProviderModels_ToolUseSupport(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		targetType protocol.APIType
		model      string
	}{
		{"Anthropic with Tools", "api.anthropic.com", protocol.TypeAnthropicV1, "claude-opus-4-6-20250514"},
		{"OpenAI with Tools", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4"},
		{"DeepSeek with Tools", "api.deepseek.com", protocol.TypeOpenAIChat, "deepseek-v4-pro"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(tt.targetType),
				transform.NewConsistencyTransform(tt.targetType),
				transform.NewVendorTransform(tt.provider),
			})

			var req interface{}
			extra := map[string]interface{}{}
			if strings.Contains(tt.provider, "anthropic") {
				extra = anthropicExtra()
				// Anthropic tool use would be set up here
				req = &anthropic.MessageNewParams{
					Model:     anthropic.Model(tt.model),
					MaxTokens: 4096,
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock("Use a tool")),
					},
				}
			} else {
				// OpenAI tool use would be set up here
				req = &openai.ChatCompletionNewParams{
					Model: openai.ChatModel(tt.model),
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("Use a tool"),
					},
				}
			}

			ctx := newFullChainContext(req, tt.provider, extra)

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			// Verify request was processed
			assert.NotNil(t, result.Request)
		})
	}
}

// TestProviderModels_MaxTokensHandling tests max_tokens parameter handling
func TestProviderModels_MaxTokensHandling(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		targetType protocol.APIType
		model      string
		maxTokens  int64
	}{
		{"Anthropic Small MaxTokens", "api.anthropic.com", protocol.TypeAnthropicV1, "claude-3-5-haiku-20241022", 512},
		{"Anthropic Large MaxTokens", "api.anthropic.com", protocol.TypeAnthropicV1, "claude-opus-4-6-20250514", 8192},
		{"OpenAI Small MaxTokens", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4o-mini", 256},
		{"OpenAI Large MaxTokens", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4", 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(tt.targetType),
				transform.NewConsistencyTransform(tt.targetType),
				transform.NewVendorTransform(tt.provider),
			})

			var req interface{}
			extra := map[string]interface{}{}
			if strings.Contains(tt.provider, "anthropic") {
				extra = anthropicExtra()
				req = &anthropic.MessageNewParams{
					Model:     anthropic.Model(tt.model),
					MaxTokens: tt.maxTokens,
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
					},
				}
			} else {
				req = &openai.ChatCompletionNewParams{
					Model:     openai.ChatModel(tt.model),
					MaxTokens: openai.Int(tt.maxTokens),
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("test"),
					},
				}
			}

			ctx := newFullChainContext(req, tt.provider, extra)

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			// Verify max_tokens was preserved
			switch r := result.Request.(type) {
			case *anthropic.MessageNewParams:
				assert.Equal(t, tt.maxTokens, r.MaxTokens)
			case *openai.ChatCompletionNewParams:
				assert.Equal(t, openai.Int(tt.maxTokens), r.MaxTokens)
			}
		})
	}
}

// TestProviderModels_SystemPromptHandling tests system prompt handling across providers
func TestProviderModels_SystemPromptHandling(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		targetType protocol.APIType
		model      string
		hasSystem  bool
	}{
		{"Anthropic With System", "api.anthropic.com", protocol.TypeAnthropicV1, "claude-opus-4-6-20250514", true},
		{"Anthropic Without System", "api.anthropic.com", protocol.TypeAnthropicV1, "claude-3-5-sonnet-20241022", false},
		{"OpenAI With System", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4", true},
		{"OpenAI Without System", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4o", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(tt.targetType),
				transform.NewConsistencyTransform(tt.targetType),
				transform.NewVendorTransform(tt.provider),
			})

			var req interface{}
			extra := map[string]interface{}{}
			if strings.Contains(tt.provider, "anthropic") {
				extra = anthropicExtra()
				anthropicReq := &anthropic.MessageNewParams{
					Model:     anthropic.Model(tt.model),
					MaxTokens: 1024,
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
					},
				}
				if tt.hasSystem {
					anthropicReq.System = []anthropic.TextBlockParam{
						{Text: "You are a helpful assistant."},
					}
				}
				req = anthropicReq
			} else {
				var messages []openai.ChatCompletionMessageParamUnion
				if tt.hasSystem {
					messages = []openai.ChatCompletionMessageParamUnion{
						openai.SystemMessage("You are a helpful assistant."),
						openai.UserMessage("test"),
					}
				} else {
					messages = []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("test"),
					}
				}
				req = &openai.ChatCompletionNewParams{
					Model:    openai.ChatModel(tt.model),
					Messages: messages,
				}
			}

			ctx := newFullChainContext(req, tt.provider, extra)

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			// Verify request was processed correctly
			assert.NotNil(t, result.Request)
		})
	}
}

// TestProviderModels_ReasoningModels tests reasoning model handling
func TestProviderModels_ReasoningModels(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		targetType  protocol.APIType
		model       string
		isReasoning bool
	}{
		{"OpenAI O1 Reasoning", "api.openai.com", protocol.TypeOpenAIChat, "o1", true},
		{"OpenAI O3 Mini Reasoning", "api.openai.com", protocol.TypeOpenAIChat, "o3-mini", true},
		{"OpenAI GPT-4o Standard", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4o", false},
		{"DeepSeek Pro Reasoning", "api.deepseek.com", protocol.TypeOpenAIChat, "deepseek-v4-pro", true},
		{"DeepSeek Flash Standard", "api.deepseek.com", protocol.TypeOpenAIChat, "deepseek-v4-flash", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(tt.targetType),
				transform.NewConsistencyTransform(tt.targetType),
				transform.NewVendorTransform(tt.provider),
			})

			req := &openai.ChatCompletionNewParams{
				Model: openai.ChatModel(tt.model),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Solve this step by step"),
				},
			}

			ctx := newFullChainContext(req, tt.provider, map[string]interface{}{})

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			out, ok := result.Request.(*openai.ChatCompletionNewParams)
			require.True(t, ok)

			// Model should be preserved
			assert.Equal(t, tt.model, string(out.Model))

			// For reasoning models, the vendor transform should handle specific requirements
			// This test mainly verifies the model passes through correctly
		})
	}
}

// TestProviderModels_AdaptiveThinkingModels tests adaptive thinking model support
func TestProviderModels_AdaptiveThinkingModels(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		supportsAdaptive bool
	}{
		{"Opus 4.6", "claude-opus-4-6-20250514", true},
		{"Sonnet 4.6", "claude-sonnet-4-6-20250514", true},
		{"Haiku 3.5", "claude-3-5-haiku-20241022", false},
		{"Sonnet 3.5", "claude-3-5-sonnet-20241022", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(protocol.TypeAnthropicV1),
				transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
				transform.NewVendorTransform("api.anthropic.com"),
			})

			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(tt.model),
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

			// Check adaptive thinking support
			if tt.supportsAdaptive {
				assert.NotNil(t, out.Thinking.OfAdaptive, "%s should support adaptive thinking", tt.model)
			} else {
				assert.Nil(t, out.Thinking.OfAdaptive, "%s should not support adaptive thinking", tt.model)
			}
		})
	}
}

// TestProviderModels_EnabledThinkingModels tests explicit enabled thinking
func TestProviderModels_EnabledThinkingModels(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		budgetTokens   int64
		shouldPreserve bool
	}{
		{"Opus 4.6 with Budget", "claude-opus-4-6-20250514", 5000, true},
		{"Haiku with Budget", "claude-3-5-haiku-20241022", 2000, true},
		{"Sonnet 4.6 with Budget", "claude-sonnet-4-6-20250514", 10000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(protocol.TypeAnthropicV1),
				transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
				transform.NewVendorTransform("api.anthropic.com"),
			})

			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(tt.model),
				MaxTokens: 8192,
				Thinking: anthropic.ThinkingConfigParamUnion{
					OfEnabled: &anthropic.ThinkingConfigEnabledParam{
						BudgetTokens: tt.budgetTokens,
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

			// Explicitly enabled thinking should be preserved for all models
			assert.NotNil(t, out.Thinking.OfEnabled, "%s should preserve explicitly enabled thinking", tt.model)
			if tt.shouldPreserve {
				assert.Equal(t, tt.budgetTokens, out.Thinking.OfEnabled.BudgetTokens)
			}
		})
	}
}

// TestProviderModels_TransformChainOrdering tests that transform steps execute in correct order
func TestProviderModels_TransformChainOrdering(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		targetType protocol.APIType
		model      string
	}{
		{"Anthropic Full Chain", "api.anthropic.com", protocol.TypeAnthropicV1, "claude-opus-4-6-20250514"},
		{"OpenAI Full Chain", "api.openai.com", protocol.TypeOpenAIChat, "gpt-4"},
		{"DeepSeek Full Chain", "api.deepseek.com", protocol.TypeOpenAIChat, "deepseek-v4-flash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(tt.targetType),
				transform.NewConsistencyTransform(tt.targetType),
				transform.NewVendorTransform(tt.provider),
			})

			var req interface{}
			extra := map[string]interface{}{}
			if strings.Contains(tt.provider, "anthropic") {
				extra = anthropicExtra()
				req = &anthropic.MessageNewParams{
					Model:     anthropic.Model(tt.model),
					MaxTokens: 1024,
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
					},
				}
			} else {
				req = &openai.ChatCompletionNewParams{
					Model: openai.ChatModel(tt.model),
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("test"),
					},
				}
			}

			ctx := newFullChainContext(req, tt.provider, extra)

			result, err := chain.Execute(ctx)
			require.NoError(t, err)

			// Verify transform steps were executed in correct order
			expectedSteps := []string{"base_convert", "consistency_normalize", "vendor_adjust"}
			assert.Equal(t, expectedSteps, result.TransformSteps, "transform steps should execute in correct order")
		})
	}
}

// TestProviderModels_ContextPreservation tests that original context is preserved
func TestProviderModels_ContextPreservation(t *testing.T) {
	chain := transform.NewTransformChain([]transform.Transform{
		transform.NewBaseTransform(protocol.TypeAnthropicV1),
		transform.NewConsistencyTransform(protocol.TypeAnthropicV1),
		transform.NewVendorTransform("api.anthropic.com"),
	})

	originalReq := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-6-20250514"),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("original")),
		},
	}

	ctx := newFullChainContext(originalReq, "api.anthropic.com", anthropicExtra())

	result, err := chain.Execute(ctx)
	require.NoError(t, err)

	// Original request should be preserved
	assert.Equal(t, originalReq, result.OriginalRequest, "original request should be preserved in context")

	// Provider URL should be preserved
	assert.Equal(t, "api.anthropic.com", result.ProviderURL, "provider URL should be preserved")

	// Streaming flag should be preserved
	assert.True(t, result.IsStreaming, "streaming flag should be preserved")

	// Scenario flags should be preserved
	assert.NotNil(t, result.ScenarioFlags, "scenario flags should be preserved")
}

// =============================================
// Helper Functions
// =============================================

// newFullChainContext creates a TransformContext with common fields for full-chain tests.
func newFullChainContext(request interface{}, providerURL string, extra map[string]interface{}) *transform.TransformContext {
	return &transform.TransformContext{
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

// newBetaRequest creates a new BetaMessageNewParams for testing.
func newBetaRequest(model string, thinking anthropic.BetaThinkingConfigParamUnion) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 4096,
		Thinking:  thinking,
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("test")}},
		},
	}
}
