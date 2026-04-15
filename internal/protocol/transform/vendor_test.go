package transform

import (
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

func TestNewVendorTransform(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")
	assert.Equal(t, "vendor_adjust", vt.Name())
	assert.Equal(t, "api.openai.com", vt.ProviderURL)
}

func TestVendorTransform_Apply_Success(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     newOpenAIRequest("gpt-4", 1024),
		ProviderURL: "api.openai.com",
		Extra: map[string]interface{}{
			"openaiConfig": &protocol.OpenAIConfig{HasThinking: false, ReasoningEffort: "none"},
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)
	_, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	assert.True(t, ok)
}

func TestVendorTransform_Apply_MissingOpenAIConfig(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     newOpenAIRequest("gpt-4", 1024),
		ProviderURL: "api.openai.com",
		Extra:       map[string]interface{}{},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err) // Should use default config
}

func TestVendorTransform_Apply_InvalidRequestType(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     "invalid",
		ProviderURL: "api.openai.com",
		Extra:       map[string]interface{}{},
	}

	err := vt.Apply(ctx)
	require.Error(t, err)

	validationErr, ok := err.(*ValidationError)
	require.True(t, ok)
	assert.Equal(t, "request", validationErr.Field)
}

func TestVendorTransform_Apply_NilRequest(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     nil,
		ProviderURL: "api.openai.com",
		Extra:       map[string]interface{}{},
	}

	err := vt.Apply(ctx)
	require.Error(t, err)
}

func TestVendorTransform_Protocols(t *testing.T) {
	tests := []struct {
		name        string
		providerURL string
		model       string
		maxTokens   int64
	}{
		{"OpenAI", "api.openai.com", "gpt-4", 1024},
		{"DeepSeek", "api.deepseek.com", "deepseek-chat", 4096},
		{"Moonshot", "api.moonshot.cn", "moonshot-v1-8k", 8192},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform(tt.providerURL)

			ctx := &TransformContext{
				Request:     newOpenAIRequest(tt.model, tt.maxTokens),
				ProviderURL: tt.providerURL,
				Extra:       map[string]interface{}{"openaiConfig": &protocol.OpenAIConfig{}},
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)
		})
	}
}

func TestNormalizeProviderURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.openai.com/v1/chat/completions", "api.openai.com"},
		{"http://api.deepseek.com:8080/v1", "api.deepseek.com"},
		{"api.moonshot.cn", "api.moonshot.cn"},
		{"api.openai.com:443", "api.openai.com"},
		{"https://api.anthropic.com", "api.anthropic.com"},
		{"https://api.openai.com/", "api.openai.com"},
		{"", ""},
		{"  https://api.openai.com  ", "api.openai.com"},
		{"HTTPS://API.OPENAI.COM", "api.openai.com"},
		{"https://api.example.com:8443/v1/models/gpt-4", "api.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeProviderURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVendorTransform_TransformerIntegration(t *testing.T) {
	// Test integration with transformer package
	providerURL := "api.openai.com"
	req := newOpenAIRequest("gpt-4", 1024)
	config := &protocol.OpenAIConfig{HasThinking: false, ReasoningEffort: "none"}

	// Direct transformer call
	transformed := ops.ApplyProviderTransforms(req, providerURL, "gpt-4", config)
	assert.NotNil(t, transformed)

	// Through VendorTransform
	vt := NewVendorTransform(providerURL)
	ctx := &TransformContext{
		Request:     req,
		ProviderURL: providerURL,
		Extra:       map[string]interface{}{"openaiConfig": config},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)
	assert.NotNil(t, ctx.Request)
}

// =============================================
// Anthropic Vendor Transform Tests
// =============================================

// assertV1Metadata checks that metadata user_id is set with expected device and account.
func assertV1Metadata(t *testing.T, result *anthropic.MessageNewParams, device, account string) {
	t.Helper()
	assert.True(t, result.Metadata.UserID.Valid(), "user_id metadata should be set")
	uid := result.Metadata.UserID.String()
	if device != "" {
		assert.Contains(t, uid, device, "user_id should contain device_id")
	}
	if account != "" {
		assert.Contains(t, uid, account, "user_id should contain account_uuid")
	}
	assert.Contains(t, uid, "session_id", "user_id should contain session_id")
}

// assertBetaMetadata checks that metadata user_id is set with expected device and account.
func assertBetaMetadata(t *testing.T, result *anthropic.BetaMessageNewParams, device, account string) {
	t.Helper()
	assert.True(t, result.Metadata.UserID.Valid(), "user_id metadata should be set")
	uid := result.Metadata.UserID.String()
	if device != "" {
		assert.Contains(t, uid, device, "user_id should contain device_id")
	}
	if account != "" {
		assert.Contains(t, uid, account, "user_id should contain account_uuid")
	}
	assert.Contains(t, uid, "session_id", "user_id should contain session_id")
}

func TestVendorTransform_AnthropicV1_ProviderURLMatching(t *testing.T) {
	tests := []struct {
		name        string
		providerURL string
		shouldApply bool
	}{
		{"Anthropic API", "https://api.anthropic.com/v1/messages", true},
		{"Claude AI", "https://claude.ai/api", true},
		{"Anthropic bare", "api.anthropic.com", true},
		{"Claude bare", "claude.ai", true},
		{"OpenAI", "https://api.openai.com/v1/chat/completions", false},
		{"DeepSeek", "https://api.deepseek.com/v1", false},
		{"Empty URL", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform(tt.providerURL)
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

			// device is required for metadata transform (panics without it)
			extra := map[string]interface{}{"device": "test-device", "user_id": "test-account"}

			ctx := &TransformContext{
				Request:     req,
				ProviderURL: tt.providerURL,
				Extra:       extra,
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)

			result, ok := ctx.Request.(*anthropic.MessageNewParams)
			require.True(t, ok)

			if tt.shouldApply {
				// For Anthropic provider, adaptive thinking should be disabled for Haiku
				assert.Nil(t, result.Thinking.OfAdaptive,
					"adaptive thinking should be disabled for Haiku on Anthropic provider %s", tt.providerURL)
			} else {
				// Non-Anthropic provider: no transform applied, adaptive should remain
				assert.NotNil(t, result.Thinking.OfAdaptive,
					"adaptive thinking should be preserved for non-Anthropic provider %s", tt.providerURL)
			}
		})
	}
}

func TestVendorTransform_AnthropicV1_EmptyModel(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")
	req := &anthropic.MessageNewParams{
		Model:     "",
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra:       map[string]interface{}{"device": "test-device", "user_id": "test-account"},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	// Request should be unchanged (empty model short-circuits)
	result, ok := ctx.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)
	assert.Empty(t, result.Model)
}

func TestVendorTransform_AnthropicV1_SupportedModel_AdaptivePreserved(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"Opus 4.6", "claude-opus-4-6"},
		{"Sonnet 4.6", "claude-sonnet-4-6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform("api.anthropic.com")
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

			ctx := &TransformContext{
				Request:     req,
				ProviderURL: "api.anthropic.com",
				Extra: map[string]interface{}{
					"device":  "test-device-id",
					"user_id": "test-account-uuid",
				},
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)

			result, ok := ctx.Request.(*anthropic.MessageNewParams)
			require.True(t, ok)

			// Adaptive thinking should be preserved for supported models
			assert.NotNil(t, result.Thinking.OfAdaptive,
				"adaptive thinking should be preserved for %s", tt.model)

			// Billing header should be injected
			require.NotEmpty(t, result.System, "system should have billing header")
			assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")

			// Metadata should contain user_id
			assert.True(t, result.Metadata.UserID.Valid(), "user_id metadata should be set")
			uid := result.Metadata.UserID.String()
			assert.Contains(t, uid, "test-device-id")
			assert.Contains(t, uid, "test-account-uuid")
		})
	}
}

func TestVendorTransform_AnthropicV1_UnsupportedModel_AdaptiveDisabled(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"Haiku", "claude-3-5-haiku-20241022"},
		{"Sonnet 3.5", "claude-3-5-sonnet-20241022"},
		{"Opus 3.7", "claude-3-7-opus-20250214"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform("api.anthropic.com")
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

			ctx := &TransformContext{
				Request:     req,
				ProviderURL: "api.anthropic.com",
				Extra: map[string]interface{}{
					"device":  "test-device-id",
					"user_id": "test-account-uuid",
				},
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)

			result, ok := ctx.Request.(*anthropic.MessageNewParams)
			require.True(t, ok)

			// Adaptive thinking should be disabled for unsupported models
			assert.Nil(t, result.Thinking.OfAdaptive,
				"adaptive thinking should be disabled for %s", tt.model)
			assert.Nil(t, result.Thinking.OfEnabled,
				"enabled thinking should also be nil for %s", tt.model)

			// Billing header should still be injected
			require.NotEmpty(t, result.System, "system should have billing header")
			assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")

			// Metadata should contain user_id
			assert.True(t, result.Metadata.UserID.Valid())
			t.Log(result.Metadata.UserID)
		})
	}
}

func TestVendorTransform_AnthropicV1_BillingHeaderPrepend(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	// Request with existing system prompt (no billing header)
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: "You are a helpful assistant."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Billing header should be prepended before existing system prompt
	require.Len(t, result.System, 2, "should have billing header + existing system")
	assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header",
		"first system block should be billing header")
	assert.Equal(t, "You are a helpful assistant.", result.System[1].Text,
		"second system block should be original prompt")
}

func TestVendorTransform_AnthropicV1_BillingHeaderReplace(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	// Request with existing billing header in system prompt
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: "x-anthropic-billing-header: cc_version=old; cc_entrypoint=cli; cch=aaaaa;"},
			{Text: "You are a helpful assistant."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Old billing header should be replaced, not duplicated
	require.Len(t, result.System, 2, "should still have 2 system blocks")
	assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")
	assert.NotContains(t, result.System[0].Text, "old", "old billing header should be replaced")
	assert.Equal(t, "You are a helpful assistant.", result.System[1].Text)
}

func TestVendorTransform_AnthropicV1_NoSystemPrompt(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		// No System field
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Billing header should be added to empty system slice
	require.NotEmpty(t, result.System, "system should have billing header")
	assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")
}

func TestVendorTransform_AnthropicV1_EnabledThinkingPreserved(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	// Enabled thinking should be preserved even on unsupported models
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	// Enabled thinking should be preserved
	assert.NotNil(t, result.Thinking.OfEnabled,
		"explicitly enabled thinking should be preserved even on Haiku")
}

// =============================================
// Anthropic Beta Vendor Transform Tests
// =============================================

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

func TestVendorTransform_AnthropicBeta_ProviderURLMatching(t *testing.T) {
	tests := []struct {
		name        string
		providerURL string
		shouldApply bool
	}{
		{"Anthropic API", "https://api.anthropic.com/v1/messages", true},
		{"Claude AI", "https://claude.ai/api", true},
		{"OpenAI", "https://api.openai.com/v1/chat/completions", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform(tt.providerURL)
			req := newBetaRequest("claude-3-5-haiku-20241022", anthropic.BetaThinkingConfigParamUnion{
				OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
			})

			// device is required for metadata transform (panics without it)
			extra := map[string]interface{}{"device": "test-device", "user_id": "test-account"}

			ctx := &TransformContext{
				Request:     req,
				ProviderURL: tt.providerURL,
				Extra:       extra,
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)

			result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
			require.True(t, ok)

			if tt.shouldApply {
				assert.Nil(t, result.Thinking.OfAdaptive,
					"adaptive thinking should be disabled for Haiku on Anthropic provider %s", tt.providerURL)
			} else {
				assert.NotNil(t, result.Thinking.OfAdaptive,
					"adaptive thinking should be preserved for non-Anthropic provider %s", tt.providerURL)
			}
		})
	}
}

func TestVendorTransform_AnthropicBeta_EmptyModel(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")
	req := &anthropic.BetaMessageNewParams{
		Model:     "",
		MaxTokens: 4096,
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("test")}},
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra:       map[string]interface{}{"device": "test-device", "user_id": "test-account"},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)
	assert.Empty(t, result.Model)
}

func TestVendorTransform_AnthropicBeta_SupportedModel_AdaptivePreserved(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"Opus 4.6", "claude-opus-4-6"},
		{"Sonnet 4.6", "claude-sonnet-4-6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform("api.anthropic.com")
			req := newBetaRequest(tt.model, anthropic.BetaThinkingConfigParamUnion{
				OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
			})

			ctx := &TransformContext{
				Request:     req,
				ProviderURL: "api.anthropic.com",
				Extra: map[string]interface{}{
					"device":  "test-device-id",
					"user_id": "test-account-uuid",
				},
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)

			result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
			require.True(t, ok)

			assert.NotNil(t, result.Thinking.OfAdaptive,
				"adaptive thinking should be preserved for %s", tt.model)

			// Billing header injected
			require.NotEmpty(t, result.System)
			assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")

			// Metadata user_id injected
			assert.True(t, result.Metadata.UserID.Valid())
			uid := result.Metadata.UserID.String()
			assert.Contains(t, uid, "test-device-id")
			assert.Contains(t, uid, "test-account-uuid")
		})
	}
}

func TestVendorTransform_AnthropicBeta_UnsupportedModel_AdaptiveDisabled(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"Haiku", "claude-3-5-haiku-20241022"},
		{"Sonnet 3.5", "claude-3-5-sonnet-20241022"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform("api.anthropic.com")
			req := newBetaRequest(tt.model, anthropic.BetaThinkingConfigParamUnion{
				OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
			})

			ctx := &TransformContext{
				Request:     req,
				ProviderURL: "api.anthropic.com",
				Extra: map[string]interface{}{
					"device":  "test-device-id",
					"user_id": "test-account-uuid",
				},
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)

			result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
			require.True(t, ok)

			assert.Nil(t, result.Thinking.OfAdaptive,
				"adaptive thinking should be disabled for %s", tt.model)

			// Metadata still injected
			assert.True(t, result.Metadata.UserID.Valid())
		})
	}
}

func TestVendorTransform_AnthropicBeta_BillingHeaderPrepend(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		System: []anthropic.BetaTextBlockParam{
			{Text: "You are a helpful assistant."},
		},
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("test")}},
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)

	require.Len(t, result.System, 2)
	assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")
	assert.Equal(t, "You are a helpful assistant.", result.System[1].Text)
}

func TestVendorTransform_AnthropicBeta_BillingHeaderReplace(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		System: []anthropic.BetaTextBlockParam{
			{Text: "x-anthropic-billing-header: cc_version=old; cc_entrypoint=cli; cch=aaaaa;"},
			{Text: "Original system prompt."},
		},
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("test")}},
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)

	require.Len(t, result.System, 2)
	assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")
	assert.NotContains(t, result.System[0].Text, "old")
	assert.Equal(t, "Original system prompt.", result.System[1].Text)
}

func TestVendorTransform_AnthropicBeta_NoSystemPrompt(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("test")}},
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)

	require.NotEmpty(t, result.System)
	assert.Contains(t, result.System[0].Text, "x-anthropic-billing-header")
}

func TestVendorTransform_AnthropicBeta_EnabledThinkingPreserved(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	req := newBetaRequest("claude-3-5-haiku-20241022", anthropic.BetaThinkingConfigParamUnion{
		OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{},
	})

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)

	assert.NotNil(t, result.Thinking.OfEnabled,
		"explicitly enabled thinking should be preserved even on Haiku")
}

func TestVendorTransform_AnthropicBeta_ThinkingBlocksFiltered(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	// Assistant message with thinking + text blocks
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("hello")}},
			{
				Role: "assistant",
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfThinking: &anthropic.BetaThinkingBlockParam{Thinking: "let me think...", Signature: "sig123"}},
					{OfText: &anthropic.BetaTextBlockParam{Text: "Here is my answer."}},
				},
			},
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("thanks")}},
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)

	// Assistant message should have thinking block removed, text block preserved
	require.Len(t, result.Messages, 3, "all 3 messages should be preserved")

	assistantMsg := result.Messages[1]
	assert.Equal(t, "assistant", string(assistantMsg.Role))

	// Should only have text block, no thinking
	for _, block := range assistantMsg.Content {
		assert.Nil(t, block.OfThinking, "thinking blocks should be filtered out")
	}

	// Text block should still be present
	foundText := false
	for _, block := range assistantMsg.Content {
		if block.OfText != nil && block.OfText.Text == "Here is my answer." {
			foundText = true
			break
		}
	}
	assert.True(t, foundText, "text block should be preserved after filtering thinking blocks")
}

func TestVendorTransform_AnthropicBeta_FullChain(t *testing.T) {
	// End-to-end test: model transform + metadata transform together
	vt := NewVendorTransform("api.anthropic.com")

	req := newBetaRequest("claude-3-5-haiku-20241022", anthropic.BetaThinkingConfigParamUnion{
		OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
	})

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":   "my-device-abc",
			"user_id":  "my-account-uuid",
			"some_key": "ignored_value",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.True(t, ok)

	// 1. Model transform: adaptive thinking disabled for Haiku
	assert.Nil(t, result.Thinking.OfAdaptive, "adaptive thinking should be disabled")

	// 2. Billing header injected
	require.NotEmpty(t, result.System)
	billingHeader := result.System[0].Text
	assert.True(t, strings.HasPrefix(billingHeader, "x-anthropic-billing-header:"))
	assert.Contains(t, billingHeader, "cc_version=")
	assert.Contains(t, billingHeader, "cc_entrypoint=cli")
	assert.Contains(t, billingHeader, "cch=")

	// 3. Metadata user_id injected with device and account
	assert.True(t, result.Metadata.UserID.Valid())
	uid := result.Metadata.UserID.String()
	assert.Contains(t, uid, "my-device-abc")
	assert.Contains(t, uid, "my-account-uuid")
	assert.Contains(t, uid, "session_id")
}

func TestVendorTransform_AnthropicV1_ThinkingBlocksFiltered(t *testing.T) {
	vt := NewVendorTransform("api.anthropic.com")

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			{Role: "user", Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("hello")}},
			{
				Role: "assistant",
				Content: []anthropic.ContentBlockParamUnion{
					{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "let me think...", Signature: "sig123"}},
					{OfText: &anthropic.TextBlockParam{Text: "Here is my answer."}},
				},
			},
			{Role: "user", Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("thanks")}},
		},
	}

	ctx := &TransformContext{
		Request:     req,
		ProviderURL: "api.anthropic.com",
		Extra: map[string]interface{}{
			"device":  "device-id",
			"user_id": "account-uuid",
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)

	result, ok := ctx.Request.(*anthropic.MessageNewParams)
	require.True(t, ok)

	require.Len(t, result.Messages, 3)

	assistantMsg := result.Messages[1]
	assert.Equal(t, "assistant", string(assistantMsg.Role))

	for _, block := range assistantMsg.Content {
		assert.Nil(t, block.OfThinking, "thinking blocks should be filtered out")
	}

	foundText := false
	for _, block := range assistantMsg.Content {
		if block.OfText != nil && block.OfText.Text == "Here is my answer." {
			foundText = true
			break
		}
	}
	assert.True(t, foundText, "text block should be preserved")
}
