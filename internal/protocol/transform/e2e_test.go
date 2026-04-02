package transform

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestE2E_ProtocolConversions tests end-to-end protocol conversions
// This ensures that converting from any source protocol to any target protocol works correctly
func TestE2E_ProtocolConversions(t *testing.T) {
	// Define test requests for each protocol type
	testRequests := map[string]struct {
		name    string
		request interface{}
	}{
		"anthropic_v1": {
			name: "Anthropic v1",
			request: &anthropic.MessageNewParams{
				Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
				MaxTokens: int64(1024),
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
				},
			},
		},
		"anthropic_beta": {
			name: "Anthropic Beta",
			request: &anthropic.BetaMessageNewParams{
				Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
				MaxTokens: int64(1024),
				Messages: []anthropic.BetaMessageParam{
					{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Hello")}},
				},
			},
		},
		"openai_chat": {
			name: "OpenAI Chat",
			request: &openai.ChatCompletionNewParams{
				Model: openai.ChatModel("gpt-4"),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Hello"),
				},
			},
		},
		"openai_responses": {
			name: "OpenAI Responses",
			request: &responses.ResponseNewParams{
				Model: "gpt-4o",
			},
		},
	}

	// Define all target API styles
	targetStyles := map[string]protocol.APIType{
		"anthropic_v1":     protocol.TypeAnthropicV1,
		"anthropic_beta":   protocol.TypeAnthropicBeta,
		"openai_chat":      protocol.TypeOpenAIChat,
		"openai_responses": protocol.TypeOpenAIResponses,
		"google":           protocol.TypeGoogle,
	}

	// Test all combinations: source -> target
	for sourceKey, sourceReq := range testRequests {
		for targetKey, targetStyle := range targetStyles {
			t.Run(sourceKey+"_to_"+targetKey, func(t *testing.T) {
				// Create transform chain with BaseTransform only
				// We're testing protocol conversion, not full chain with consistency/vendor
				chain := NewTransformChain([]Transform{
					NewBaseTransform(targetStyle),
				})

				ctx := &TransformContext{
					Request:     sourceReq.request,
					ProviderURL: "api.example.com",
					IsStreaming: false,
				}

				// Execute the transform
				finalCtx, err := chain.Execute(ctx)
				if err != nil {
					// Check error type and log appropriately
					errMsg := err.Error()
					switch {
					case strings.Contains(errMsg, "not yet implemented"):
						t.Log("⚠️  NOT SUPPORTED (not yet implemented)")
					case strings.Contains(errMsg, "cannot convert"):
						t.Log("⚠️  NOT SUPPORTED (cannot convert)")
					case strings.Contains(errMsg, "unsupported request type"):
						t.Log("⚠️  NOT SUPPORTED (unsupported request type)")
					default:
						t.Log("❌ FAILED:", err)
						t.Fail()
					}
					return
				}

				// Verify the result
				require.NotNil(t, finalCtx)
				require.NotNil(t, finalCtx.Request)

				// Verify the request was transformed to the correct type
				var correctType bool
				switch targetStyle {
				case protocol.TypeAnthropicV1:
					_, correctType = finalCtx.Request.(*anthropic.MessageNewParams)
				case protocol.TypeAnthropicBeta:
					_, correctType = finalCtx.Request.(*anthropic.BetaMessageNewParams)
				case protocol.TypeOpenAIChat:
					_, correctType = finalCtx.Request.(*openai.ChatCompletionNewParams)
				case protocol.TypeOpenAIResponses:
					_, correctType = finalCtx.Request.(*responses.ResponseNewParams)
				case protocol.TypeGoogle:
					_, correctType = finalCtx.Request.(*protocol.GoogleRequest)
				}

				if correctType {
					t.Log("✅ SUPPORTED")
				} else {
					t.Log("❌ FAILED - wrong type returned")
					t.Fail()
				}
			})
		}
	}
}

// TestE2E_FullChain tests the complete transform chain (Base -> Consistency -> Vendor)
func TestE2E_FullChain(t *testing.T) {
	// Test Anthropic v1 -> OpenAI Chat with full chain
	t.Run("anthropic_v1_to_openai_chat_full_chain", func(t *testing.T) {
		sourceReq := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
			MaxTokens: int64(1024),
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
			},
		}

		chain := NewTransformChain([]Transform{
			NewBaseTransform(protocol.TypeOpenAIChat),
			NewConsistencyTransform(protocol.TypeOpenAIChat),
			NewVendorTransform("api.openai.com"),
		})

		ctx := &TransformContext{
			Request:     sourceReq,
			ProviderURL: "api.openai.com",
			IsStreaming: false,
		}

		finalCtx, err := chain.Execute(ctx)
		require.NoError(t, err)
		require.NotNil(t, finalCtx)

		// Verify final request type
		finalReq, ok := finalCtx.Request.(*openai.ChatCompletionNewParams)
		require.True(t, ok, "expected *openai.ChatCompletionNewParams")

		// Verify the model was converted
		assert.NotEmpty(t, finalReq.Model)

		// Verify Config has OpenAIConfig set by BaseTransform
		config := finalCtx.Config.OpenAIConfig
		require.NotNil(t, config, "expected OpenAIConfig in Config")
	})

	// Test Anthropic v1 -> Google with full chain
	t.Run("anthropic_v1_to_google_full_chain", func(t *testing.T) {
		sourceReq := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
			MaxTokens: int64(1024),
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
			},
		}

		chain := NewTransformChain([]Transform{
			NewBaseTransform(protocol.TypeGoogle),
		})

		ctx := &TransformContext{
			Request:     sourceReq,
			ProviderURL: "generativelanguage.googleapis.com",
			IsStreaming: false,
		}

		finalCtx, err := chain.Execute(ctx)
		require.NoError(t, err)
		require.NotNil(t, finalCtx)

		// Verify final request type
		finalReq, ok := finalCtx.Request.(*protocol.GoogleRequest)
		require.True(t, ok, "expected *GoogleRequest")

		// Verify the model was converted
		assert.NotEmpty(t, finalReq.Model)

		// Verify contents were converted
		assert.NotNil(t, finalReq.Contents)
		assert.NotEmpty(t, finalReq.Contents)

		// Verify config was set
		assert.NotNil(t, finalReq.Config)
	})

	// Test Anthropic Beta -> OpenAI Responses with full chain
	t.Run("anthropic_beta_to_openai_responses_full_chain", func(t *testing.T) {
		sourceReq := &anthropic.BetaMessageNewParams{
			Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
			MaxTokens: int64(1024),
			Messages: []anthropic.BetaMessageParam{
				{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Hello")}},
			},
		}

		chain := NewTransformChain([]Transform{
			NewBaseTransform(protocol.TypeOpenAIResponses),
			NewConsistencyTransform(protocol.TypeOpenAIResponses),
			NewVendorTransform("api.openai.com"),
		})

		ctx := &TransformContext{
			Request:     sourceReq,
			ProviderURL: "api.openai.com",
			IsStreaming: false,
		}

		finalCtx, err := chain.Execute(ctx)
		require.NoError(t, err)
		require.NotNil(t, finalCtx)

		// Verify final request type
		finalReq, ok := finalCtx.Request.(*responses.ResponseNewParams)
		require.True(t, ok, "expected *responses.ResponseNewParams")

		// Verify the model was converted
		assert.NotEmpty(t, string(finalReq.Model))
	})
}

// TestE2E_PointerTypeValidation tests that non-pointer requests are rejected by transforms.
// With the generic NewTransformContext, only pointer types satisfying RequestUnionConstraint
// can be used — this test verifies the BaseTransform also rejects value types when the
// context is constructed directly (e.g., in internal tests).
func TestE2E_PointerTypeValidation(t *testing.T) {
	valueReq := anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: int64(1024),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeAnthropicV1),
	})

	ctx := &TransformContext{
		Request:     valueReq, // Value type, not pointer
		ProviderURL: "api.anthropic.com",
		IsStreaming: false,
	}

	_, err := chain.Execute(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported request type")
}

// TestE2E_TransformStepsRecorded tests that transform steps are properly recorded
func TestE2E_TransformStepsRecorded(t *testing.T) {
	sourceReq := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: int64(1024),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	chain := NewTransformChain([]Transform{
		NewBaseTransform(protocol.TypeOpenAIChat),
		NewConsistencyTransform(protocol.TypeOpenAIChat),
		NewVendorTransform("api.openai.com"),
	})

	ctx := &TransformContext{
		Request:     sourceReq,
		ProviderURL: "api.openai.com",
		IsStreaming: false,
	}

	finalCtx, err := chain.Execute(ctx)
	require.NoError(t, err)

	// Verify all transform steps were recorded
	expectedSteps := []string{"base_convert", "consistency_normalize", "vendor_adjust"}
	assert.Equal(t, expectedSteps, finalCtx.TransformSteps)
}
